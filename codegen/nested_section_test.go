package codegen_test

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	m "github.com/arran4/gogenconf"
	"github.com/arran4/gogenconf/codegen"
)

func TestGeneratedRecursiveSectionModelAndBinding(t *testing.T) {
	s := m.Schema{Version: 1, Sections: []m.SectionDefinition{{
		Name: "project", GoName: "Projects", GoType: "ProjectConfig", Repeated: true,
		Fields: []m.Field{{Key: "module_path", GoName: "ModulePath", GoType: "string"}},
		Children: []m.SectionDefinition{{
			Name: "build", GoName: "Build", GoType: "BuildConfig",
			Fields: []m.Field{{Key: "target", GoName: "Target", GoType: "string", Required: true}, {Key: "enabled", GoName: "Enabled", GoType: "bool", Default: m.Literal{Value: "true"}}},
		}},
	}}}
	generated, err := codegen.GenerateAppAPI(s, codegen.Options{Package: "appconfig", RuntimeImport: "example.test/nested/nativeconfig"})
	if err != nil {
		t.Fatal(err)
	}
	src := string(generated)
	for _, want := range []string{
		"type ProjectConfig struct", "BuildConfig", "type BuildConfig struct", "Target", "SectionBindingValue(nestedProjectsBuild", "missing required field project.build.target",
	} {
		if !strings.Contains(src, want) {
			t.Fatalf("generated recursive binding missing %q\n%s", want, src)
		}
	}
	if strings.Contains(src, "reflect.") {
		t.Fatal("recursive generated binding uses reflection")
	}
	runtime, err := codegen.GenerateRuntime(codegen.RuntimeOptions{Package: "nativeconfig", Capabilities: codegen.FullRuntimeCapabilities()})
	if err != nil {
		t.Fatal(err)
	}
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "go.mod"), []byte("module example.test/nested\n\ngo 1.25.0\n"), 0644); err != nil {
		t.Fatal(err)
	}
	if err := runtime.WriteToDir(filepath.Join(dir, "nativeconfig")); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(dir, "appconfig"), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "appconfig", "config_generated.go"), generated, 0644); err != nil {
		t.Fatal(err)
	}
	const main = `package main
import("context";"example.test/nested/appconfig")
func main(){
 input:=[]byte("config for generated version 1\n\nproject lxa {\n module_path github.com/arran4/lxa\n build {\n  target ./cmd/lxa\n  enabled false\n }\n}\n")
 cfg,_,err:=appconfig.LoadBytes(context.Background(),input)
 if err!=nil||len(cfg.Projects)!=1||cfg.Projects[0].Name!="lxa"||cfg.Projects[0].ModulePath!="github.com/arran4/lxa"||cfg.Projects[0].Build.Target!="./cmd/lxa"||cfg.Projects[0].Build.Enabled { panic("nested canonical decode failed") }
}`
	if err := os.WriteFile(filepath.Join(dir, "main.go"), []byte(main), 0644); err != nil {
		t.Fatal(err)
	}
	cmd := exec.Command("go", "run", ".")
	cmd.Dir = dir
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("generated recursive consumer failed: %v\n%s", err, out)
	}
}

func TestValidateRejectsRecursiveSchemaCollisions(t *testing.T) {
	base := m.Schema{Version: 1, Sections: []m.SectionDefinition{{
		Name: "project", GoName: "Project", GoType: "ProjectConfig",
		Fields:   []m.Field{{Key: "name", GoName: "Name", GoType: "string"}},
		Children: []m.SectionDefinition{{Name: "build", GoName: "Build", GoType: "BuildConfig", Fields: []m.Field{{Key: "target", GoName: "Target", GoType: "string"}}}},
	}}}
	if err := codegen.Validate(base); err != nil {
		t.Fatalf("valid nested schema rejected: %v", err)
	}
	cases := []m.Schema{
		func() m.Schema {
			s := base
			s.Sections[0].Children = append(s.Sections[0].Children, m.SectionDefinition{Name: "build", GoName: "Other", GoType: "OtherConfig"})
			return s
		}(),
		func() m.Schema { s := base; s.Sections[0].Children[0].GoName = "Name"; return s }(),
		func() m.Schema {
			s := base
			s.Sections[0].Children = append(s.Sections[0].Children, m.SectionDefinition{Name: "other", GoName: "Other", GoType: "BuildConfig"})
			return s
		}(),
		func() m.Schema {
			s := base
			s.Sections[0].Children[0].Fields = append(s.Sections[0].Children[0].Fields, m.Field{Key: "target", GoName: "Other", GoType: "string"})
			return s
		}(),
	}
	for i, s := range cases {
		if err := codegen.Validate(s); err == nil {
			t.Fatalf("recursive invalid schema %d accepted", i)
		}
	}
}

func TestGeneratedRecursiveStrictAndNonStrictValidation(t *testing.T) {
	s := m.Schema{
		Identity: "buildtool-schema",
		Version:  1,
		RootFields: []m.Field{
			{Key: "workspace", GoName: "Workspace", GoType: "string", Default: m.Literal{Value: "/work"}},
			{Key: "legacy_root", InputOnly: true, Deprecated: true},
			{Key: "runtime_root", GoName: "RuntimeRoot", GoType: "string", RuntimeOnly: true},
		},
		Sections: []m.SectionDefinition{
			{
				Name:   "daemon",
				GoName: "Daemon",
				GoType: "DaemonConfig",
				Fields: []m.Field{
					{Key: "listen", GoName: "Listen", GoType: "string", Default: m.Literal{Value: "127.0.0.1:9000"}},
					{Key: "legacy_daemon", InputOnly: true, Deprecated: true},
					{Key: "runtime_state", GoName: "RuntimeState", GoType: "string", RuntimeOnly: true},
				},
			},
			{
				Name:     "project",
				GoName:   "Projects",
				GoType:   "ProjectConfig",
				Repeated: true,
				Fields: []m.Field{
					{Key: "module_path", GoName: "ModulePath", GoType: "string", Required: true},
					{Key: "legacy_project", InputOnly: true, Deprecated: true},
				},
				Children: []m.SectionDefinition{
					{
						Name:   "build",
						GoName: "Build",
						GoType: "BuildConfig",
						Fields: []m.Field{
							{Key: "target", GoName: "Target", GoType: "string", Required: true},
							{Key: "cgo", GoName: "CGO", GoType: "bool", Default: m.Literal{Value: "false"}},
							{Key: "legacy_build", InputOnly: true, Deprecated: true},
							{Key: "runtime_cache", GoName: "RuntimeCache", GoType: "string", RuntimeOnly: true},
						},
					},
					{
						Name:   "release",
						GoName: "Release",
						GoType: "ReleaseConfig",
						Fields: []m.Field{
							{Key: "channel", GoName: "Channel", GoType: "string", Default: m.Literal{Value: "stable"}},
						},
					},
					{
						Name:     "artifact",
						GoName:   "Artifacts",
						GoType:   "ArtifactConfig",
						Repeated: true,
						Fields: []m.Field{
							{Key: "output", GoName: "Output", GoType: "string", Required: true},
						},
					},
				},
			},
		},
	}

	runtime, err := codegen.GenerateRuntime(codegen.RuntimeOptions{
		Package:      "nativeconfig",
		Capabilities: codegen.FullRuntimeCapabilities(),
	})
	if err != nil {
		t.Fatalf("GenerateRuntime failed: %v", err)
	}
	generated, err := codegen.GenerateAppAPI(s, codegen.Options{
		Package:       "appconfig",
		RuntimeImport: "example.test/validator/nativeconfig",
	})
	if err != nil {
		t.Fatalf("GenerateAppAPI failed: %v", err)
	}

	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "go.mod"), []byte("module example.test/validator\n\ngo 1.25.0\n"), 0644); err != nil {
		t.Fatal(err)
	}
	if err := runtime.WriteToDir(filepath.Join(dir, "nativeconfig")); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(dir, "appconfig"), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "appconfig", "config_generated.go"), generated, 0644); err != nil {
		t.Fatal(err)
	}

	const mainSrc = `package main

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"example.test/validator/appconfig"
	"example.test/validator/nativeconfig"
)

func mustParse(name, src string) *nativeconfig.Document {
	doc, err := nativeconfig.ParseNamed(strings.NewReader(src), name)
	if err != nil {
		panic(fmt.Sprintf("%s: unexpected parse error: %v", name, err))
	}
	return doc
}

func mustFailConfigError(caseName, src string, strict bool, wantStage nativeconfig.Stage, wantPathSub, wantErrSub string, wantLine int) {
	doc := mustParse(caseName+".conf", src)
	_, err := appconfig.Load(context.Background(), doc, appconfig.WithStrict(strict))
	if err == nil {
		panic(fmt.Sprintf("%s: expected error (%s), got nil", caseName, wantErrSub))
	}
	var cfgErr *nativeconfig.ConfigError
	if !errors.As(err, &cfgErr) {
		panic(fmt.Sprintf("%s: expected *nativeconfig.ConfigError, got %T: %v", caseName, err, err))
	}
	if wantStage != "" && cfgErr.Stage != wantStage {
		panic(fmt.Sprintf("%s: expected stage %q, got %q (%v)", caseName, wantStage, cfgErr.Stage, err))
	}
	if wantPathSub != "" && !strings.Contains(cfgErr.Path, wantPathSub) {
		panic(fmt.Sprintf("%s: expected path containing %q, got %q (%v)", caseName, wantPathSub, cfgErr.Path, err))
	}
	if wantErrSub != "" && !strings.Contains(err.Error(), wantErrSub) {
		panic(fmt.Sprintf("%s: expected error containing %q, got %q", caseName, wantErrSub, err.Error()))
	}
	if wantLine > 0 && cfgErr.Source.Line != wantLine {
		panic(fmt.Sprintf("%s: expected source line %d, got %+v (%v)", caseName, wantLine, cfgErr.Source, err))
	}
	if wantLine > 0 && cfgErr.Source.Source != caseName+".conf" {
		panic(fmt.Sprintf("%s: expected source file %q, got %+v", caseName, caseName+".conf", cfgErr.Source))
	}
}

func main() {
	ctx := context.Background()

	// 1. Valid legacy repeated section & 10. Valid optional nested section omitted
	legacyValid := "config for buildtool version 1\n\nsection project lxa\n    module_path github.com/arran4/lxa\n    section build\n        target ./cmd/lxa\n    end\nend\n"
	cfg1, _, err := appconfig.LoadBytes(ctx, []byte(legacyValid), appconfig.WithStrict(true))
	if err != nil || len(cfg1.Projects) != 1 || cfg1.Projects[0].Name != "lxa" || cfg1.Projects[0].Build.Target != "./cmd/lxa" {
		panic(fmt.Sprintf("case 1 failed: %v / %+v", err, cfg1))
	}

	// 2. Equivalent valid canonical named object & 3. Canonical named object with nested build section
	canonValid := "config for buildtool version 1\n\nproject lxa {\n    module_path github.com/arran4/lxa\n    build {\n        target ./cmd/lxa\n        cgo true\n    }\n}\n"
	cfg2, _, err := appconfig.LoadBytes(ctx, []byte(canonValid), appconfig.WithStrict(true))
	if err != nil || len(cfg2.Projects) != 1 || cfg2.Projects[0].Name != "lxa" || cfg2.Projects[0].ModulePath != "github.com/arran4/lxa" || cfg2.Projects[0].Build.Target != "./cmd/lxa" || !cfg2.Projects[0].Build.CGO {
		panic(fmt.Sprintf("case 2/3 failed: %v / %+v", err, cfg2))
	}

	// 4. Unknown nested field
	mustFailConfigError("case4_unknown_nested_field",
		"config for buildtool version 1\n\nproject lxa {\n    module_path github.com/arran4/lxa\n    build {\n        target ./cmd/lxa\n        bogus_flag 123\n    }\n}\n",
		true, nativeconfig.StageValidate, "project.lxa.build.bogus_flag", "unknown field", 7)

	// 5. Unknown nested object (named and anonymous)
	mustFailConfigError("case5a_unknown_nested_object",
		"config for buildtool version 1\n\nproject lxa {\n    module_path github.com/arran4/lxa\n    build {\n        target ./cmd/lxa\n    }\n    mystery {\n        foo bar\n    }\n}\n",
		true, nativeconfig.StageValidate, "project.lxa.mystery", "unknown section", 8)
	mustFailConfigError("case5b_anonymous_nested_object",
		"config for buildtool version 1\n\nproject lxa {\n    module_path github.com/arran4/lxa\n    build {\n        target ./cmd/lxa\n        {\n            foo bar\n        }\n    }\n}\n",
		true, nativeconfig.StageValidate, "project.lxa.build", "unknown section", 7)

	// 6. Duplicate nested scalar keys
	mustFailConfigError("case6_dup_nested_key",
		"config for buildtool version 1\n\nproject lxa {\n    module_path github.com/arran4/lxa\n    build {\n        target ./cmd/one\n        target ./cmd/two\n    }\n}\n",
		true, nativeconfig.StageBind, "project.lxa.build.target", "duplicate unresolved field", 7)

	// 7. Duplicate named child identities (top-level canonical, top-level mixed, and nested repeated child)
	mustFailConfigError("case7a_dup_canon_identity",
		"config for buildtool version 1\n\nproject lxa {\n    module_path a\n    build {\n        target ./a\n    }\n}\nproject lxa {\n    module_path b\n    build {\n        target ./b\n    }\n}\n",
		true, nativeconfig.StageBind, "project.lxa", "duplicate", 9)
	mustFailConfigError("case7b_dup_mixed_identity",
		"config for buildtool version 1\n\nsection project lxa\n    module_path a\n    section build\n        target ./a\n    end\nend\nproject lxa {\n    module_path b\n    build {\n        target ./b\n    }\n}\n",
		true, nativeconfig.StageBind, "project.lxa", "duplicate", 9)
	mustFailConfigError("case7c_dup_nested_repeated_identity",
		"config for buildtool version 1\n\nproject lxa {\n    module_path a\n    build {\n        target ./a\n    }\n    artifact cli {\n        output bin/a\n    }\n    artifact cli {\n        output bin/b\n    }\n}\n",
		true, nativeconfig.StageBind, "project.lxa.artifact.cli", "duplicate", 11)

	// 8. Missing required nested field
	mustFailConfigError("case8_missing_req_nested_field",
		"config for buildtool version 1\n\nproject lxa {\n    module_path github.com/arran4/lxa\n    build {\n        cgo true\n    }\n}\n",
		false, nativeconfig.StageValidate, "project.lxa.build.target", "missing required field", 5)

	// 9. Missing required nested section
	mustFailConfigError("case9_missing_req_nested_section",
		"config for buildtool version 1\n\nproject lxa {\n    module_path github.com/arran4/lxa\n}\n",
		false, nativeconfig.StageValidate, "project.lxa.build.target", "missing required section", 3)

	// 11. Multiple valid repeated children (top-level and nested)
	multiValid := "config for buildtool version 1\n\nproject lxa {\n    module_path github.com/arran4/lxa\n    build {\n        target ./cmd/lxa\n    }\n    artifact bin1 {\n        output /out/lxa\n    }\n    section artifact bin2\n        output /out/lxa-helper\n    end\n}\nsection project gogenconf\n    module_path github.com/arran4/gogenconf\n    section build\n        target ./cmd/gogenconf\n    end\nend\n"
	cfg11, _, err := appconfig.LoadBytes(ctx, []byte(multiValid), appconfig.WithStrict(true))
	if err != nil {
		panic(fmt.Sprintf("case 11 failed: %v", err))
	}
	var lxaProj, gogenProj *appconfig.ProjectConfig
	for i := range cfg11.Projects {
		if cfg11.Projects[i].Name == "lxa" {
			lxaProj = &cfg11.Projects[i]
		} else if cfg11.Projects[i].Name == "gogenconf" {
			gogenProj = &cfg11.Projects[i]
		}
	}
	if lxaProj == nil || gogenProj == nil || len(lxaProj.Artifacts) != 2 {
		panic(fmt.Sprintf("case 11 failed: %+v", cfg11))
	}
	hasBin1, hasBin2 := false, false
	for _, art := range lxaProj.Artifacts {
		if art.Name == "bin1" && art.Output == "/out/lxa" {
			hasBin1 = true
		}
		if art.Name == "bin2" && art.Output == "/out/lxa-helper" {
			hasBin2 = true
		}
	}
	if !hasBin1 || !hasBin2 {
		panic(fmt.Sprintf("case 11 missing artifacts: %+v", lxaProj.Artifacts))
	}

	// 12. Malformed repeated identities
	mustFailConfigError("case12a_legacy_missing_id",
		"config for buildtool version 1\n\nsection project\n    module_path a\nend\n",
		false, nativeconfig.StageValidate, "project", "repeated section", 3)
	mustFailConfigError("case12b_legacy_multi_token_id",
		"config for buildtool version 1\n\nsection project a b\n    module_path a\nend\n",
		false, nativeconfig.StageValidate, "project", "malformed repeated", 3)
	mustFailConfigError("case12c_brace_no_instance",
		"config for buildtool version 1\n\nproject {\n    module_path a\n}\n",
		false, nativeconfig.StageValidate, "project", "repeated section", 4)
	mustFailConfigError("case12d_brace_empty_container",
		"config for buildtool version 1\n\nproject {\n}\n",
		false, nativeconfig.StageValidate, "project", "repeated section", 3)
	mustFailConfigError("case12e_brace_empty_quoted_id",
		"config for buildtool version 1\n\nproject \"\" {\n    module_path a\n}\n",
		false, nativeconfig.StageValidate, "project", "malformed", 3)

	// 13. InputOnly migration fields (root, top-level, nested) accepted in strict mode
	inputOnlyValid := "config for buildtool version 1\n\nlegacy_root old_root\n\ndaemon {\n    legacy_daemon old_daemon\n}\n\nproject lxa {\n    module_path github.com/arran4/lxa\n    legacy_project old_proj\n    build {\n        target ./cmd/lxa\n        legacy_build old_build\n    }\n}\n"
	cfg13, _, err := appconfig.LoadBytes(ctx, []byte(inputOnlyValid), appconfig.WithStrict(true))
	if err != nil || cfg13.Workspace != "/work" || cfg13.Projects[0].Build.Target != "./cmd/lxa" {
		panic(fmt.Sprintf("case 13 failed: %v / %+v", err, cfg13))
	}

	// 14. Source-supplied RuntimeOnly fields rejected (root, top-level, nested)
	mustFailConfigError("case14a_runtime_only_root",
		"config for buildtool version 1\n\nruntime_root injected\n",
		false, nativeconfig.StageValidate, "runtime_root", "runtime-only", 3)
	mustFailConfigError("case14b_runtime_only_section",
		"config for buildtool version 1\n\ndaemon {\n    runtime_state injected\n}\n",
		false, nativeconfig.StageValidate, "daemon.runtime_state", "runtime-only", 4)
	mustFailConfigError("case14c_runtime_only_nested",
		"config for buildtool version 1\n\nproject lxa {\n    module_path x\n    build {\n        target ./cmd/x\n        runtime_cache injected\n    }\n}\n",
		false, nativeconfig.StageValidate, "project.lxa.build.runtime_cache", "runtime-only", 7)

	// 15. Wrong identity and future version
	mustFailConfigError("case15a_wrong_identity",
		"config for othertool version 1\n",
		false, nativeconfig.StageValidate, "", "expected config identity", 1)
	mustFailConfigError("case15b_future_version",
		"config for buildtool version 99\n",
		false, nativeconfig.StageValidate, "", "newer than supported version", 1)

	// 17. Strict rejection with original AST preserved
	strictRejectSrc := "config for buildtool version 1\n\n# user comment at root\nproject lxa {\n    # user comment in project\n    module_path github.com/arran4/lxa\n    build {\n        target ./cmd/lxa\n        unknown_nested_key \"keep-me\"\n    }\n    unknown_nested_block {\n        preserved_key \"also-keep-me\"\n    }\n}\n"
	doc17 := mustParse("case17.conf", strictRejectSrc)
	if _, err := appconfig.Load(ctx, doc17, appconfig.WithStrict(true)); err == nil {
		panic("case 17: expected strict rejection")
	}
	formatted17 := nativeconfig.Format(doc17)
	for _, want := range []string{"# user comment at root", "# user comment in project", "unknown_nested_key", "keep-me", "unknown_nested_block", "also-keep-me"} {
		if !strings.Contains(formatted17, want) {
			panic(fmt.Sprintf("case 17: AST lost %q after strict rejection:\n%s", want, formatted17))
		}
	}

	// 18. Non-strict loading with unknown AST content retained
	doc18 := mustParse("case18.conf", strictRejectSrc)
	cfg18, err := appconfig.Load(ctx, doc18, appconfig.WithStrict(false))
	if err != nil || len(cfg18.Projects) != 1 || cfg18.Projects[0].Build.Target != "./cmd/lxa" {
		panic(fmt.Sprintf("case 18: non-strict load failed: %v / %+v", err, cfg18))
	}
	formatted18 := nativeconfig.Format(doc18)
	for _, want := range []string{"# user comment at root", "# user comment in project", "unknown_nested_key", "keep-me", "unknown_nested_block", "also-keep-me"} {
		if !strings.Contains(formatted18, want) {
			panic(fmt.Sprintf("case 18: AST lost %q after non-strict load:\n%s", want, formatted18))
		}
	}
}
`
	if err := os.WriteFile(filepath.Join(dir, "main.go"), []byte(mainSrc), 0644); err != nil {
		t.Fatal(err)
	}
	cmd := exec.Command("go", "run", ".")
	cmd.Dir = dir
	cmd.Env = append(os.Environ(), "GOWORK=off")
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("recursive strict/non-strict validation consumer failed: %v\n%s", err, out)
	}
}

func TestGeneratedRecursiveEnrichmentAndInspection(t *testing.T) {
	s := m.Schema{
		Identity: "testapp",
		Version:  1,
		RootFields: []m.Field{
			{Key: "workspace", GoName: "Workspace", GoType: "string", Default: m.Literal{Value: "/default/work"}},
			{Key: "root_secret", GoName: "RootSecret", GoType: "string", Sensitive: true, Required: true},
		},
		Sections: []m.SectionDefinition{
			{
				Name: "service", GoName: "Service", GoType: "ServiceConfig",
				Fields: []m.Field{
					{Key: "name", GoName: "Name", GoType: "string", Default: m.Literal{Value: "myservice"}},
				},
				Children: []m.SectionDefinition{
					{
						Name: "build", GoName: "Build", GoType: "ServiceBuildConfig",
						Fields: []m.Field{
							{Key: "target", GoName: "Target", GoType: "string", Default: m.Literal{Value: "./service/main"}},
							{Key: "token", GoName: "Token", GoType: "string", Sensitive: true},
						},
					},
				},
			},
			{
				Name: "project", GoName: "Projects", GoType: "ProjectConfig", Repeated: true,
				ExampleNames: []string{"lxa"},
				Fields: []m.Field{
					{Key: "module_path", GoName: "ModulePath", GoType: "string"},
				},
				Children: []m.SectionDefinition{
					{
						Name: "build", GoName: "Build", GoType: "ProjectBuildConfig",
						Fields: []m.Field{
							{Key: "target", GoName: "Target", GoType: "string", Required: true},
							{Key: "cgo", GoName: "Cgo", GoType: "bool", Default: m.Literal{Value: "true"}},
							{Key: "token", GoName: "Token", GoType: "string", Sensitive: true},
						},
					},
				},
			},
		},
	}

	runtime, err := codegen.GenerateRuntime(codegen.RuntimeOptions{
		Package:      "nativeconfig",
		Capabilities: codegen.FullRuntimeCapabilities(),
	})
	if err != nil {
		t.Fatalf("GenerateRuntime failed: %v", err)
	}
	generated, err := codegen.GenerateAppAPI(s, codegen.Options{
		Package:       "appconfig",
		RuntimeImport: "example.test/enrichment/nativeconfig",
	})
	if err != nil {
		t.Fatalf("GenerateAppAPI failed: %v", err)
	}

	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "go.mod"), []byte("module example.test/enrichment\n\ngo 1.25.0\n"), 0644); err != nil {
		t.Fatal(err)
	}
	if err := runtime.WriteToDir(filepath.Join(dir, "nativeconfig")); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(dir, "appconfig"), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "appconfig", "config_generated.go"), generated, 0644); err != nil {
		t.Fatal(err)
	}

	const mainSrc = `package main

import (
	"context"
	"fmt"
	"strings"

	"example.test/enrichment/appconfig"
	"example.test/enrichment/nativeconfig"
)

func main() {
	// 1. Metadata queries & sibling section disambiguation
	if !appconfig.IsSensitive("", "root_secret") {
		panic("root_secret should be sensitive")
	}
	if !appconfig.IsSensitive("service.build", "token") {
		panic("service.build.token should be sensitive")
	}
	if !appconfig.IsSensitive("project.build", "token") {
		panic("project.build.token should be sensitive")
	}
	if !appconfig.IsSensitive("project.lxa.build", "token") {
		panic("project.lxa.build.token should be sensitive")
	}

	metaServiceBuild, ok := appconfig.Field("service.build", "target")
	if !ok || metaServiceBuild.Section != "service.build" {
		panic(fmt.Sprintf("service.build.target meta failed: ok=%v, meta=%+v", ok, metaServiceBuild))
	}
	metaProjBuild, ok := appconfig.Field("project.build", "target")
	if !ok || metaProjBuild.Section != "project.build" || !metaProjBuild.Required {
		panic(fmt.Sprintf("project.build.target meta failed: ok=%v, meta=%+v", ok, metaProjBuild))
	}
	metaProjLxaBuild, ok := appconfig.Field("project.lxa.build", "cgo")
	if !ok || metaProjLxaBuild.Section != "project.build" {
		panic(fmt.Sprintf("project.lxa.build.cgo meta failed: ok=%v, meta=%+v", ok, metaProjLxaBuild))
	}

	// 2. Recursive Inspection & sensitive redaction
	docSrc := "config for testapp version 1\n\nroot_secret \"super-secret-root\"\n\nservice {\n    build {\n        token \"secret-service-token\"\n    }\n}\n\nproject lxa {\n    build {\n        target ./cmd/lxa\n        token \"secret-project-token\"\n    }\n}\n"
	doc, err := nativeconfig.Parse(strings.NewReader(docSrc))
	if err != nil {
		panic(err)
	}

	inspected := appconfig.Inspect(doc)
	_ = inspected
	inspectedStr := appconfig.InspectString(doc)

	if strings.Contains(inspectedStr, "super-secret-root") || strings.Contains(inspectedStr, "secret-service-token") || strings.Contains(inspectedStr, "secret-project-token") {
		panic(fmt.Sprintf("InspectString leaked sensitive value:\n%s", inspectedStr))
	}
	if !strings.Contains(inspectedStr, "[REDACTED]") {
		panic(fmt.Sprintf("InspectString missing [REDACTED]:\n%s", inspectedStr))
	}

	// Check that original document was NOT mutated (safe copy)
	origStr := nativeconfig.Format(doc)
	if !strings.Contains(origStr, "super-secret-root") || !strings.Contains(origStr, "secret-service-token") || !strings.Contains(origStr, "secret-project-token") {
		panic("Inspect mutated original document!")
	}

	// 3. Seed and Sample
	seed := appconfig.Seed()
	if seed == nil || seed.Version != 1 {
		panic("invalid seed")
	}
	sample := appconfig.Sample()
	sampleStr := nativeconfig.Format(sample)
	if !strings.Contains(sampleStr, "service") || !strings.Contains(sampleStr, "/default/work") {
		panic(fmt.Sprintf("sample missing expected defaults:\n%s", sampleStr))
	}

	// 4. Recursive Enrichment and Idempotency
	enrichInput := "config for testapp version 1\n\nroot_secret my-secret\n\nproject lxa {\n    module_path github.com/arran4/lxa\n    build {\n        target ./cmd/lxa\n    }\n}\n"
	docEnrich, err := nativeconfig.Parse(strings.NewReader(enrichInput))
	if err != nil {
		panic(err)
	}

	if err := appconfig.Enrich(docEnrich); err != nil {
		panic(fmt.Sprintf("Enrich failed: %v", err))
	}

	firstEnrich := nativeconfig.Format(docEnrich)
	if !strings.Contains(firstEnrich, "workspace /default/work") && !strings.Contains(firstEnrich, "workspace \"/default/work\"") {
		panic(fmt.Sprintf("Enrich missed root default workspace:\n%s", firstEnrich))
	}
	if !strings.Contains(firstEnrich, "cgo true") {
		panic(fmt.Sprintf("Enrich missed nested child default cgo:\n%s", firstEnrich))
	}

	// Enrich second time (idempotency test)
	if err := appconfig.Enrich(docEnrich); err != nil {
		panic(fmt.Sprintf("second Enrich failed: %v", err))
	}
	secondEnrich := nativeconfig.Format(docEnrich)
	if firstEnrich != secondEnrich {
		panic(fmt.Sprintf("Enrich not idempotent:\nFIRST:\n%s\nSECOND:\n%s", firstEnrich, secondEnrich))
	}

	// 5. Load typed config from enriched doc
	cfg, err := appconfig.Load(context.Background(), docEnrich)
	if err != nil {
		panic(fmt.Sprintf("Load failed: %v", err))
	}
	if cfg.Workspace != "/default/work" || len(cfg.Projects) != 1 || cfg.Projects[0].Build.Target != "./cmd/lxa" || !cfg.Projects[0].Build.Cgo {
		panic(fmt.Sprintf("Loaded config mismatch: %+v", cfg))
	}
}
`
	if err := os.WriteFile(filepath.Join(dir, "main.go"), []byte(mainSrc), 0644); err != nil {
		t.Fatal(err)
	}
	cmd := exec.Command("go", "run", ".")
	cmd.Dir = dir
	cmd.Env = append(os.Environ(), "GOWORK=off")
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("recursive enrichment and inspection consumer failed: %v\n%s", err, out)
	}
}

func TestSchemaFrontendAndDocGeneration(t *testing.T) {
	s := m.Schema{
		Identity: "apptool-schema",
		Version:  2,
		RootFields: []m.Field{
			{Key: "workspace", GoName: "Workspace", GoType: "string", Default: m.Literal{Value: "/work"}, Required: true, Documentation: []string{"Root workspace."}},
		},
		Sections: []m.SectionDefinition{
			{
				Name: "daemon", GoName: "Daemon", GoType: "DaemonConfig", Documentation: []string{"Daemon configuration."},
				Fields: []m.Field{
					{Key: "port", GoName: "Port", GoType: "int", Default: m.Literal{Value: "8080"}},
				},
				Children: []m.SectionDefinition{
					{
						Name: "telemetry", GoName: "Telemetry", GoType: "TelemetryConfig", Documentation: []string{"Telemetry settings."},
						Fields: []m.Field{
							{Key: "endpoint", GoName: "Endpoint", GoType: "string", Default: m.Literal{Value: "http://telemetry.local"}},
						},
					},
				},
			},
			{
				Name: "worker", GoName: "Workers", GoType: "WorkerConfig", Repeated: true, ExampleNames: []string{"worker-1", "worker-2"},
				Documentation: []string{"Worker instances."},
				Fields: []m.Field{
					{Key: "concurrency", GoName: "Concurrency", GoType: "int", Default: m.Literal{Value: "4"}},
				},
			},
		},
	}

	// 1. FormatSchema and ParseSchema round-trip
	formatted := m.FormatSchema("apptool", &s)
	parsed, err := m.ParseSchema(strings.NewReader(formatted))
	if err != nil {
		t.Fatalf("ParseSchema failed: %v\n%s", err, formatted)
	}
	if len(parsed.RootFields) != 1 || parsed.RootFields[0].Key != "workspace" {
		t.Fatalf("parsed root fields mismatch: %+v", parsed.RootFields)
	}
	if len(parsed.Sections) != 2 || len(parsed.Sections[0].Children) != 1 {
		t.Fatalf("parsed sections mismatch: %+v", parsed.Sections)
	}
	reformatted := m.FormatSchema("apptool", parsed)
	if reformatted != formatted {
		t.Fatalf("schema format not idempotent:\nORIGINAL:\n%s\nREFORMATTED:\n%s", formatted, reformatted)
	}

	// 2. GenerateMarkdown documentation
	md, err := codegen.GenerateMarkdown(&s, codegen.DocOptions{AppName: "apptool"})
	if err != nil {
		t.Fatalf("GenerateMarkdown failed: %v", err)
	}
	mdStr := string(md)
	for _, want := range []string{"Root Fields", "workspace", "daemon", "telemetry", "worker"} {
		if !strings.Contains(mdStr, want) {
			t.Errorf("markdown missing %q:\n%s", want, mdStr)
		}
	}

	// 3. GenerateManPage documentation
	man, err := codegen.GenerateManPage(&s, codegen.ManOptions{AppName: "apptool"})
	if err != nil {
		t.Fatalf("GenerateManPage failed: %v", err)
	}
	manStr := string(man)
	for _, want := range []string{"Root Fields", "workspace", "daemon", "daemon.telemetry", "worker"} {
		if !strings.Contains(manStr, want) {
			t.Errorf("manpage missing %q:\n%s", want, manStr)
		}
	}

	// 4. InspectSchema and FormatText
	info := codegen.InspectSchema(&s)
	if len(info.RootFields) != 1 || len(info.Sections) != 2 || len(info.Sections[0].Children) != 1 {
		t.Fatalf("InspectSchema info mismatch: %+v", info)
	}
	txt := info.FormatText()
	for _, want := range []string{"Root Fields", "workspace", "daemon", "telemetry", "worker"} {
		if !strings.Contains(txt, want) {
			t.Errorf("FormatText missing %q:\n%s", want, txt)
		}
	}
}

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

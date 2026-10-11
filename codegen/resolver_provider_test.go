package codegen_test

import (
	"bytes"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	m "github.com/arran4/gogenconf"
	"github.com/arran4/gogenconf/codegen"
)

func runCmd(t *testing.T, dir string, env []string, name string, args ...string) string {
	t.Helper()
	cmd := exec.Command(name, args...)
	cmd.Dir = dir
	cmd.Env = append(os.Environ(), env...)
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("%s %v failed in %s: %v\nOutput:\n%s", name, args, dir, err, out)
	}
	return string(out)
}

func TestFeatureSelectionAbsentUnused(t *testing.T) {
	// 1. Literal-only schema: should have no env, file, json, or provider machinery
	literalSchema := m.Schema{
		Version: 1,
		Sections: []m.SectionDefinition{
			{
				Name:   "service",
				GoName: "Service",
				GoType: "ServiceConfig",
				Fields: []m.Field{
					{
						Key:     "endpoint",
						GoName:  "Endpoint",
						GoType:  "string",
						Policy:  m.Eager,
						Default: m.Literal{Value: "https://default.test"},
					},
				},
			},
		},
	}

	planLiteral, err := codegen.PlanFromSchema(literalSchema, codegen.RuntimeOptions{Package: "rt"})
	if err != nil {
		t.Fatalf("PlanFromSchema failed: %v", err)
	}

	if planLiteral.HasCapability(codegen.CapabilityProvider) {
		t.Errorf("literal-only schema should not have Provider capability")
	}
	if planLiteral.HasDeclarer("from_json_file") {
		t.Errorf("literal-only schema should not have from_json_file declarer")
	}
	if planLiteral.HasDeclarer("from_file") {
		t.Errorf("literal-only schema should not have from_file declarer")
	}

	filesLiteral, err := planLiteral.Generate()
	if err != nil {
		t.Fatalf("Generate failed: %v", err)
	}

	for _, f := range filesLiteral {
		if strings.Contains(f.Name, "provider") {
			t.Errorf("unexpected provider file in literal-only runtime: %s", f.Name)
		}
		if bytes.Contains(f.Content, []byte("encoding/json")) {
			t.Errorf("unexpected encoding/json in literal-only file: %s", f.Name)
		}
		if bytes.Contains(f.Content, []byte("from_json_file")) {
			t.Errorf("unexpected from_json_file in literal-only file: %s", f.Name)
		}
	}

	// 2. Env + File nesting schema: should have from_env and from_file, but no json and no provider
	envFileSchema := m.Schema{
		Version: 1,
		Sections: []m.SectionDefinition{
			{
				Name:   "service",
				GoName: "Service",
				GoType: "ServiceConfig",
				Fields: []m.Field{
					{
						Key:    "credential",
						GoName: "Credential",
						GoType: "[]byte",
						Policy: m.Eager,
						Default: m.Call{
							Name: "from_file",
							Args: []m.Expr{
								m.Call{
									Name: "from_env",
									Args: []m.Expr{m.Literal{Value: "CREDENTIAL_FILE"}},
								},
							},
						},
					},
				},
			},
		},
	}

	planEnvFile, err := codegen.PlanFromSchema(envFileSchema, codegen.RuntimeOptions{Package: "rt"})
	if err != nil {
		t.Fatalf("PlanFromSchema failed: %v", err)
	}
	if !planEnvFile.HasDeclarer("from_env") {
		t.Errorf("env-file schema should have from_env declarer")
	}
	if !planEnvFile.HasDeclarer("from_file") {
		t.Errorf("env-file schema should have from_file declarer")
	}
	if planEnvFile.HasDeclarer("from_json_file") {
		t.Errorf("env-file schema should not have from_json_file declarer")
	}
	if planEnvFile.HasCapability(codegen.CapabilityProvider) {
		t.Errorf("eager env-file schema should not have Provider capability")
	}

	filesEnvFile, err := planEnvFile.Generate()
	if err != nil {
		t.Fatalf("Generate failed: %v", err)
	}
	for _, f := range filesEnvFile {
		if strings.Contains(f.Name, "provider") {
			t.Errorf("unexpected provider file in eager schema: %s", f.Name)
		}
		if bytes.Contains(f.Content, []byte("encoding/json")) {
			t.Errorf("unexpected encoding/json in env-file schema: %s", f.Name)
		}
	}

	// 3. Provider-backed schema: should have Provider capability
	providerSchema := m.Schema{
		Version: 1,
		Sections: []m.SectionDefinition{
			{
				Name:   "service",
				GoName: "Service",
				GoType: "ServiceConfig",
				Fields: []m.Field{
					{
						Key:    "content",
						GoName: "Content",
						GoType: "[]byte",
						Policy: m.ProviderBacked,
					},
				},
			},
		},
	}

	planProvider, err := codegen.PlanFromSchema(providerSchema, codegen.RuntimeOptions{Package: "rt"})
	if err != nil {
		t.Fatalf("PlanFromSchema failed: %v", err)
	}
	if !planProvider.HasCapability(codegen.CapabilityProvider) {
		t.Errorf("provider schema must have Provider capability")
	}
	filesProvider, err := planProvider.Generate()
	if err != nil {
		t.Fatalf("Generate failed: %v", err)
	}
	hasProviderFile := false
	for _, f := range filesProvider {
		if strings.Contains(f.Name, "provider") {
			hasProviderFile = true
		}
	}
	if !hasProviderFile {
		t.Errorf("expected provider file in provider schema output")
	}
}

func TestStandaloneDependencyFreeConsumer(t *testing.T) {
	// Schema with eager fields, provider-backed fields, repeated sections, and sensitive fields.
	schema := m.Schema{
		Version: 1,
		Sections: []m.SectionDefinition{
			{
				Name:   "service",
				GoName: "Service",
				GoType: "ServiceConfig",
				Fields: []m.Field{
					{
						Key:     "endpoint",
						GoName:  "Endpoint",
						GoType:  "string",
						Policy:  m.Eager,
						Default: m.Literal{Value: "https://default.test"},
					},
					{
						Key:       "token",
						GoName:    "Token",
						GoType:    "string",
						Policy:    m.Eager,
						Sensitive: true,
					},
					{
						Key:    "content",
						GoName: "Content",
						GoType: "[]byte",
						Policy: m.ProviderBacked,
					},
				},
			},
			{
				Name:     "upstream",
				GoName:   "Upstreams",
				GoType:   "UpstreamConfig",
				Repeated: true,
				Fields: []m.Field{
					{
						Key:    "url",
						GoName: "URL",
						GoType: "string",
						Policy: m.Eager,
					},
				},
			},
		},
	}

	dir := t.TempDir()
	const moduleName = "example.test/standaloneconsumer"

	// 1. Fresh go.mod with NO require or replace on gogenconf
	goMod := "module " + moduleName + "\n\ngo 1.25.0\n"
	if err := os.WriteFile(filepath.Join(dir, "go.mod"), []byte(goMod), 0644); err != nil {
		t.Fatal(err)
	}

	// 2. Generate application runtime
	rtOpts := codegen.RuntimeOptions{
		Package:      "myconfig",
		Capabilities: codegen.FullRuntimeCapabilities(),
	}
	rtFiles, err := codegen.GenerateRuntime(rtOpts)
	if err != nil {
		t.Fatalf("GenerateRuntime failed: %v", err)
	}
	if err := rtFiles.WriteToDir(filepath.Join(dir, "myconfig")); err != nil {
		t.Fatalf("WriteToDir failed: %v", err)
	}

	// 3. Generate application model using RuntimeImport
	modelBytes, err := codegen.GenerateModel(schema, codegen.Options{
		Package:       "appconfig",
		RuntimeImport: moduleName + "/myconfig",
	})
	if err != nil {
		t.Fatalf("GenerateModel failed: %v", err)
	}
	if err := os.MkdirAll(filepath.Join(dir, "appconfig"), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "appconfig", "config_generated.go"), modelBytes, 0644); err != nil {
		t.Fatal(err)
	}

	// 4. Generate application binder using RuntimeImport
	binderBytes, err := codegen.GenerateBinder(schema, codegen.Options{
		Package:       "binding",
		ConfigImport:  moduleName + "/appconfig",
		RuntimeImport: moduleName + "/myconfig",
	})
	if err != nil {
		t.Fatalf("GenerateBinder failed: %v", err)
	}
	if err := os.MkdirAll(filepath.Join(dir, "binding"), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "binding", "binding_generated.go"), binderBytes, 0644); err != nil {
		t.Fatal(err)
	}

	// 5. Verify NO gogenconf imports in any generated files
	for _, f := range rtFiles {
		if bytes.Contains(f.Content, []byte("github.com/arran4/gogenconf")) {
			t.Fatalf("runtime file %s imports gogenconf", f.Name)
		}
	}
	if bytes.Contains(modelBytes, []byte("github.com/arran4/gogenconf")) {
		t.Fatalf("generated model imports gogenconf")
	}
	if bytes.Contains(binderBytes, []byte("github.com/arran4/gogenconf")) {
		t.Fatalf("generated binder imports gogenconf")
	}

	// 6. Write consumer main.go proving parsing, resolution, custom declarer, provider reload, sensitive error redaction
	consumerMain := `package main

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"example.test/standaloneconsumer/appconfig"
	"example.test/standaloneconsumer/binding"
	"example.test/standaloneconsumer/myconfig"
)

func main() {
	ctx := context.Background()
	dir := os.TempDir()
	contentPath := filepath.Join(dir, "provider-test.txt")
	if err := os.WriteFile(contentPath, []byte("initial content"), 0600); err != nil {
		panic(err)
	}
	defer os.Remove(contentPath)

	configText := fmt.Sprintf(` + "`" + `
config_version 1

section service
	endpoint from_env(CUSTOM_ENDPOINT)
	token custom_upper(from_env(RAW_TOKEN))
	content from_file("%s")
end

section upstream primary
	url https://upstream-1.test
end

section upstream secondary
	url https://upstream-2.test
end
` + "`" + `, contentPath)

	doc, err := myconfig.Parse(strings.NewReader(configText))
	if err != nil {
		panic(fmt.Sprintf("parse error: %v", err))
	}

	registry, err := myconfig.NewStandardRegistry()
	if err != nil {
		panic(err)
	}

	// Register custom application declarer: custom_upper
	err = registry.Register("custom_upper", "", func(ctx context.Context, r *myconfig.Registry, args []myconfig.Expr) (any, error) {
		if len(args) != 1 {
			return nil, fmt.Errorf("custom_upper expects 1 argument")
		}
		val, err := myconfig.Resolve[string](ctx, r, args[0])
		if err != nil {
			return nil, err
		}
		return strings.ToUpper(val), nil
	})
	if err != nil {
		panic(err)
	}

	os.Setenv("CUSTOM_ENDPOINT", "https://api.mycompany.test")
	os.Setenv("RAW_TOKEN", "super-secret-token")

	cfg, err := binding.Resolve(ctx, doc, registry, nil)
	if err != nil {
		panic(fmt.Sprintf("resolve error: %v", err))
	}
	var _ appconfig.Config = cfg

	if cfg.Service.Endpoint != "https://api.mycompany.test" {
		panic("endpoint mismatch: " + cfg.Service.Endpoint)
	}
	if cfg.Service.Token != "SUPER-SECRET-TOKEN" {
		panic("token mismatch: " + cfg.Service.Token)
	}

	// Test provider resolution
	val, err := cfg.Service.Content.Resolve(ctx)
	if err != nil {
		panic(err)
	}
	if string(val) != "initial content" {
		panic("provider value mismatch: " + string(val))
	}

	// Test provider sees updated source file
	if err := os.WriteFile(contentPath, []byte("updated content"), 0600); err != nil {
		panic(err)
	}
	val2, err := cfg.Service.Content.Resolve(ctx)
	if err != nil {
		panic(err)
	}
	if string(val2) != "updated content" {
		panic("provider reload mismatch: " + string(val2))
	}

	// Test repeated sections
	if len(cfg.Upstreams) != 2 {
		panic(fmt.Sprintf("expected 2 upstreams, got %d", len(cfg.Upstreams)))
	}
	if cfg.Upstreams[0].Name != "primary" || cfg.Upstreams[0].URL != "https://upstream-1.test" {
		panic("upstream[0] mismatch")
	}
	if cfg.Upstreams[1].Name != "secondary" || cfg.Upstreams[1].URL != "https://upstream-2.test" {
		panic("upstream[1] mismatch")
	}

	// Test sensitive error redaction:
	badDoc, _ := myconfig.Parse(strings.NewReader("config_version 1\nsection service\n token from_file(/nonexistent/secret)\nend\n"))
	_, badErr := binding.Resolve(ctx, badDoc, registry, nil)
	if badErr == nil {
		panic("expected error for nonexistent sensitive source")
	}
	if !strings.Contains(badErr.Error(), "source resolution failed") {
		panic("sensitive error should be redacted: " + badErr.Error())
	}
	if strings.Contains(badErr.Error(), "/nonexistent/secret") {
		panic("sensitive path leaked in error: " + badErr.Error())
	}

	fmt.Println("STANDALONE_SUCCESS")
}
`
	if err := os.WriteFile(filepath.Join(dir, "main.go"), []byte(consumerMain), 0644); err != nil {
		t.Fatal(err)
	}

	// 7. Test dependency graph: go list -deps MUST NOT contain github.com/arran4/gogenconf
	deps := runCmd(t, dir, []string{"GOWORK=off"}, "go", "list", "-deps", "./...")
	if strings.Contains(deps, "github.com/arran4/gogenconf") {
		t.Fatalf("CRITICAL CONTRACT VIOLATION: standalone consumer imports gogenconf:\n%s", deps)
	}

	// 8. Build and run the consumer
	runCmd(t, dir, []string{"GOWORK=off"}, "go", "build", "./...")
	out := runCmd(t, dir, []string{"GOWORK=off"}, "go", "run", ".")
	if !strings.Contains(out, "STANDALONE_SUCCESS") {
		t.Fatalf("unexpected consumer output:\n%s", out)
	}
}

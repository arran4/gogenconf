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

func TestGeneratedDefaultsRejectInvalidEnvironmentAndRedactSensitiveValues(t *testing.T) {
	s := m.Schema{Version: 1, Sections: []m.SectionDefinition{{
		Name: "service", GoName: "Service", GoType: "ServiceConfig", Fields: []m.Field{
			{Key: "enabled", GoName: "Enabled", GoType: "bool", Default: m.Literal{Value: "true"}, Environment: "SERVICE_ENABLED"},
			{Key: "secret_enabled", GoName: "SecretEnabled", GoType: "bool", Sensitive: true, Environment: "SERVICE_SECRET_ENABLED"},
		},
	}}}
	runtime, err := codegen.GenerateRuntime(codegen.RuntimeOptions{Package: "nativeconfig", Capabilities: codegen.FullRuntimeCapabilities()})
	if err != nil {
		t.Fatal(err)
	}
	generated, err := codegen.GenerateAppAPI(s, codegen.Options{Package: "appconfig", RuntimeImport: "example.test/app/nativeconfig"})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(generated), "func Defaults(lookup nativeconfig.LookupEnv) (Config, error)") {
		t.Fatal("Defaults did not become fallible")
	}
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "go.mod"), []byte("module example.test/app\n\ngo 1.25.0\n"), 0644); err != nil {
		t.Fatal(err)
	}
	if err := runtime.WriteToDir(filepath.Join(dir, "nativeconfig")); err != nil {
		t.Fatal(err)
	}
	appDir := filepath.Join(dir, "appconfig")
	if err := os.MkdirAll(appDir, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(appDir, "config_generated.go"), generated, 0644); err != nil {
		t.Fatal(err)
	}
	main := `package main
import (
  "errors"
  "fmt"
  "strings"
  "example.test/app/appconfig"
)
func main() {
  _, err := appconfig.Defaults(func(k string) (string, bool) { return "not-a-bool", k == "SERVICE_ENABLED" })
  if err == nil || !strings.Contains(err.Error(), "service.enabled") { panic(fmt.Sprintf("unexpected ordinary error: %v", err)) }
  _, err = appconfig.Defaults(func(k string) (string, bool) { return "top-secret-not-a-bool", k == "SERVICE_SECRET_ENABLED" })
  if err == nil || !errors.Is(err, appconfig.ErrInvalidDefault) || strings.Contains(err.Error(), "top-secret") { panic(fmt.Sprintf("sensitive value leaked or sentinel missing: %v", err)) }
  fmt.Println("defaults rejection succeeded")
}`
	if err := os.WriteFile(filepath.Join(dir, "main.go"), []byte(main), 0644); err != nil {
		t.Fatal(err)
	}
	cmd := exec.Command("go", "run", ".")
	cmd.Dir = dir
	cmd.Env = append(os.Environ(), "GOWORK=off")
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("generated consumer failed: %v\n%s", err, out)
	}
	if !strings.Contains(string(out), "defaults rejection succeeded") {
		t.Fatalf("unexpected generated consumer output: %s", out)
	}
}

func TestGeneratedDefaultsRejectNarrowIntegerOverflowAtGeneration(t *testing.T) {
	s := m.Schema{Version: 1, Sections: []m.SectionDefinition{{
		Name: "service", GoName: "Service", GoType: "ServiceConfig", Fields: []m.Field{
			{Key: "count", GoName: "Count", GoType: "int8", Default: m.Literal{Value: "128"}},
		},
	}}}
	if _, err := codegen.Generate(s, codegen.Options{Package: "appconfig"}); err == nil || !strings.Contains(err.Error(), "value out of range") {
		t.Fatalf("narrow integer default was accepted: %v", err)
	}
}

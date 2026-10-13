package codegen_test

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	m "github.com/arran4/gogenconf"
	"github.com/arran4/gogenconf/codegen"
)

func TestClassifyType(t *testing.T) {
	tests := []struct {
		typeStr   string
		wantKind  codegen.TypeKind
		wantSlice bool
		wantMap   bool
		wantPtr   bool
		wantErr   bool
	}{
		{typeStr: "string", wantKind: codegen.TypeKindString},
		{typeStr: "bool", wantKind: codegen.TypeKindBool},
		{typeStr: "int", wantKind: codegen.TypeKindInt},
		{typeStr: "int8", wantKind: codegen.TypeKindInt},
		{typeStr: "int16", wantKind: codegen.TypeKindInt},
		{typeStr: "int32", wantKind: codegen.TypeKindInt},
		{typeStr: "int64", wantKind: codegen.TypeKindInt},
		{typeStr: "uint", wantKind: codegen.TypeKindUint},
		{typeStr: "uint8", wantKind: codegen.TypeKindUint},
		{typeStr: "uint16", wantKind: codegen.TypeKindUint},
		{typeStr: "uint32", wantKind: codegen.TypeKindUint},
		{typeStr: "uint64", wantKind: codegen.TypeKindUint},
		{typeStr: "float32", wantKind: codegen.TypeKindFloat},
		{typeStr: "float64", wantKind: codegen.TypeKindFloat},
		{typeStr: "time.Duration", wantKind: codegen.TypeKindDuration},
		{typeStr: "[]byte", wantKind: codegen.TypeKindBytes},
		{typeStr: "[]string", wantKind: codegen.TypeKindString, wantSlice: true},
		{typeStr: "[]int", wantKind: codegen.TypeKindInt, wantSlice: true},
		{typeStr: "[$]int", wantKind: codegen.TypeKindInt, wantMap: true},
		{typeStr: "map[string]int", wantKind: codegen.TypeKindInt, wantMap: true},
		{typeStr: "[]time.Duration", wantKind: codegen.TypeKindDuration, wantSlice: true},
		{typeStr: "*int", wantKind: codegen.TypeKindInt, wantPtr: true},
		{typeStr: "*string", wantKind: codegen.TypeKindString, wantPtr: true},
		{typeStr: "*time.Duration", wantKind: codegen.TypeKindDuration, wantPtr: true},
		{typeStr: "Port", wantKind: codegen.TypeKindCustom},
		{typeStr: "custom.MyType", wantKind: codegen.TypeKindCustom},
		{typeStr: "*custom.MyType", wantKind: codegen.TypeKindCustom, wantPtr: true},
		// Invalid / unsupported types
		{typeStr: "complex128", wantErr: true},
		{typeStr: "chan int", wantErr: true},
		{typeStr: "func()", wantErr: true},
		{typeStr: "[4]int", wantErr: true},
		{typeStr: "unexported_type", wantErr: true},
	}

	for _, tt := range tests {
		ti, err := codegen.ClassifyType(tt.typeStr)
		if tt.wantErr {
			if err == nil {
				t.Errorf("ClassifyType(%q) expected error, got nil", tt.typeStr)
			}
			continue
		}
		if err != nil {
			t.Errorf("ClassifyType(%q) unexpected error: %v", tt.typeStr, err)
			continue
		}
		if ti.Kind != tt.wantKind {
			t.Errorf("ClassifyType(%q) Kind = %v, want %v", tt.typeStr, ti.Kind, tt.wantKind)
		}
		if ti.IsSlice != tt.wantSlice {
			t.Errorf("ClassifyType(%q) IsSlice = %v, want %v", tt.typeStr, ti.IsSlice, tt.wantSlice)
		}
		if ti.IsMap != tt.wantMap {
			t.Errorf("ClassifyType(%q) IsMap = %v, want %v", tt.typeStr, ti.IsMap, tt.wantMap)
		}
		if ti.IsPtr != tt.wantPtr {
			t.Errorf("ClassifyType(%q) IsPtr = %v, want %v", tt.typeStr, ti.IsPtr, tt.wantPtr)
		}
	}
}

func TestValidateTypeDefault(t *testing.T) {
	tiInt, _ := codegen.ClassifyType("int")
	tiBool, _ := codegen.ClassifyType("bool")
	tiDur, _ := codegen.ClassifyType("time.Duration")
	tiSliceInt, _ := codegen.ClassifyType("[]int")

	if err := codegen.ValidateTypeDefault(tiInt, "123"); err != nil {
		t.Fatalf("unexpected error for int default: %v", err)
	}
	if err := codegen.ValidateTypeDefault(tiInt, "notanint"); err == nil {
		t.Fatal("expected error for non-int default")
	}

	if err := codegen.ValidateTypeDefault(tiBool, "true"); err != nil {
		t.Fatalf("unexpected error for bool default: %v", err)
	}
	if err := codegen.ValidateTypeDefault(tiBool, "yes"); err == nil {
		t.Fatal("expected error for non-bool default")
	}

	if err := codegen.ValidateTypeDefault(tiDur, "5s"); err != nil {
		t.Fatalf("unexpected error for duration default: %v", err)
	}
	if err := codegen.ValidateTypeDefault(tiDur, "five_seconds"); err == nil {
		t.Fatal("expected error for non-duration default")
	}

	if err := codegen.ValidateTypeDefault(tiSliceInt, "1, 2, 3"); err != nil {
		t.Fatalf("unexpected error for slice int default: %v", err)
	}
	if err := codegen.ValidateTypeDefault(tiSliceInt, "1, bad, 3"); err == nil {
		t.Fatal("expected error for invalid slice element default")
	}
}

func TestStandaloneModuleTypesAndLifecycle(t *testing.T) {
	dir := t.TempDir()

	// 1. Initialize fresh standalone consumer module with zero dependencies
	goMod := "module standalone.example.com/typesapp\n\ngo 1.24\n"
	if err := os.WriteFile(filepath.Join(dir, "go.mod"), []byte(goMod), 0644); err != nil {
		t.Fatal(err)
	}

	// 2. Generate application-owned native runtime into internal/nativeconfig
	runtimeFiles, err := codegen.GenerateRuntime(codegen.RuntimeOptions{
		Package:      "nativeconfig",
		Capabilities: []codegen.Capability{codegen.CapabilityParser, codegen.CapabilityFormatter, codegen.CapabilityResolver, codegen.CapabilityProvider},
	})
	if err != nil {
		t.Fatal(err)
	}
	runtimeDir := filepath.Join(dir, "internal", "nativeconfig")
	if err := runtimeFiles.WriteToDir(runtimeDir); err != nil {
		t.Fatal(err)
	}

	// 3. Define schema exercising rich Go types and custom hooks
	s := m.Schema{
		Version: 1,
		Sections: []m.SectionDefinition{
			{
				Name:   "app",
				GoName: "App",
				GoType: "AppConfig",
				Fields: []m.Field{
					{Key: "name", GoName: "Name", GoType: "string", Default: m.Literal{Value: "my-service"}},
					{Key: "port", GoName: "Port", GoType: "int", Default: m.Literal{Value: "8080"}},
					{Key: "tls_port", GoName: "TLSPort", GoType: "uint16", Default: m.Literal{Value: "8443"}},
					{Key: "rate_limit", GoName: "RateLimit", GoType: "float64", Default: m.Literal{Value: "100.5"}},
					{Key: "enabled", GoName: "Enabled", GoType: "bool", Default: m.Literal{Value: "true"}},
					{Key: "timeout", GoName: "Timeout", GoType: "time.Duration", Default: m.Literal{Value: "10s"}},
					{Key: "tags", GoName: "Tags", GoType: "[]string", Default: m.Literal{Value: "web,api"}},
					{Key: "allowed_ports", GoName: "AllowedPorts", GoType: "[]int", Default: m.Literal{Value: "80,443"}},
					{Key: "routes", GoName: "Routes", GoType: "[$]int", Default: m.Map{Entries: []m.MapEntry{{Key: "http", Value: m.Literal{Value: "80"}}, {Key: "https", Value: m.Literal{Value: "443"}}}}},
					{Key: "retries", GoName: "Retries", GoType: "*int", Default: m.Literal{Value: "3"}},
					{Key: "optional_flag", GoName: "OptionalFlag", GoType: "*bool"},
					{Key: "sanitized_path", GoName: "SanitizedPath", GoType: "SanitizedString", Default: m.Literal{Value: "/my//path/"}},
					{Key: "validated_range", GoName: "ValidatedRange", GoType: "RangeValue", Default: m.Literal{Value: "50"}},
					{Key: "secret_token", GoName: "SecretToken", GoType: "SecretToken", Sensitive: true, Default: m.Literal{Value: "valid-secret"}},
				},
			},
		},
	}

	// 4. Generate app API
	appAPICode, err := codegen.GenerateAppAPI(s, codegen.Options{
		Package:       "appconfig",
		RuntimeImport: "standalone.example.com/typesapp/internal/nativeconfig",
	})
	if err != nil {
		t.Fatalf("GenerateAppAPI failed: %v", err)
	}

	appConfigDir := filepath.Join(dir, "appconfig")
	if err := os.MkdirAll(appConfigDir, 0755); err != nil {
		t.Fatal(err)
	}

	// Add custom application types definition into appconfig package
	typesCode := `package appconfig

import (
	"fmt"
	"strconv"
	"strings"
)

// SanitizedString implements ConfigSanitizer
type SanitizedString string

func (s *SanitizedString) SanitizeConfig() error {
	*s = SanitizedString(strings.ReplaceAll(string(*s), "//", "/"))
	return nil
}

// RangeValue implements ConfigDecoder and ConfigValidator
type RangeValue int

func (r *RangeValue) DecodeConfig(val string) error {
	v, err := strconv.Atoi(val)
	if err != nil {
		return err
	}
	*r = RangeValue(v)
	return nil
}

func (r *RangeValue) ValidateConfig() error {
	if int(*r) < 0 || int(*r) > 100 {
		return fmt.Errorf("range value %d out of bounds [0, 100]", *r)
	}
	return nil
}

// SecretToken implements ConfigValidator on a sensitive field
type SecretToken string

func (s *SecretToken) ValidateConfig() error {
	if strings.HasPrefix(string(*s), "invalid") {
		return fmt.Errorf("secret token %q is invalid", string(*s))
	}
	return nil
}
`
	if err := os.WriteFile(filepath.Join(appConfigDir, "custom_types.go"), []byte(typesCode), 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(appConfigDir, "config_generated.go"), appAPICode, 0644); err != nil {
		t.Fatal(err)
	}

	// 5. Write a comprehensive test in the standalone module
	testCode := `package appconfig_test

import (
	"context"
	"strings"
	"testing"
	"time"

	"standalone.example.com/typesapp/appconfig"
)

func TestTypesAndLifecycle(t *testing.T) {
	ctx := context.Background()

	// 1. Defaults load
	cfg, err := appconfig.Load(ctx, appconfig.Seed())
	if err != nil {
		t.Fatalf("Load(Seed()) failed: %v", err)
	}

	if cfg.App.Name != "my-service" {
		t.Errorf("Name = %q, want my-service", cfg.App.Name)
	}
	if cfg.App.Port != 8080 {
		t.Errorf("Port = %d, want 8080", cfg.App.Port)
	}
	if cfg.App.TLSPort != 8443 {
		t.Errorf("TLSPort = %d, want 8443", cfg.App.TLSPort)
	}
	if cfg.App.RateLimit != 100.5 {
		t.Errorf("RateLimit = %f, want 100.5", cfg.App.RateLimit)
	}
	if !cfg.App.Enabled {
		t.Errorf("Enabled = false, want true")
	}
	if cfg.App.Timeout != 10*time.Second {
		t.Errorf("Timeout = %v, want 10s", cfg.App.Timeout)
	}
	if len(cfg.App.Tags) != 2 || cfg.App.Tags[0] != "web" || cfg.App.Tags[1] != "api" {
		t.Errorf("Tags = %v, want [web, api]", cfg.App.Tags)
	}
	if len(cfg.App.AllowedPorts) != 2 || cfg.App.AllowedPorts[0] != 80 || cfg.App.AllowedPorts[1] != 443 {
		t.Errorf("AllowedPorts = %v, want [80, 443]", cfg.App.AllowedPorts)
	}
	if cfg.App.Routes["http"] != 80 || cfg.App.Routes["https"] != 443 {
		t.Errorf("Routes = %v, want http=80 and https=443", cfg.App.Routes)
	}
	if cfg.App.Retries == nil || *cfg.App.Retries != 3 {
		t.Errorf("Retries = %v, want &3", cfg.App.Retries)
	}
	if cfg.App.OptionalFlag != nil {
		t.Errorf("OptionalFlag = %v, want nil (omitted)", cfg.App.OptionalFlag)
	}

	// Sanitizer verification: /my//path/ -> /my/path/
	if cfg.App.SanitizedPath != "/my/path/" {
		t.Errorf("SanitizedPath = %q, want /my/path/", cfg.App.SanitizedPath)
	}

	// 2. Custom document overriding values
	customDoc := ` + "`" + `
config_version 1

section app
	port 9000
	optional_flag false
	timeout 30s
	validated_range 75
	secret_token super-secret
end
` + "`" + `
	cfg2, _, err := appconfig.LoadBytes(ctx, []byte(customDoc))
	if err != nil {
		t.Fatalf("LoadBytes failed: %v", err)
	}
	if cfg2.App.Port != 9000 {
		t.Errorf("Port = %d, want 9000", cfg2.App.Port)
	}
	if cfg2.App.Timeout != 30*time.Second {
		t.Errorf("Timeout = %v, want 30s", cfg2.App.Timeout)
	}
	if cfg2.App.OptionalFlag == nil || *cfg2.App.OptionalFlag != false {
		t.Errorf("OptionalFlag = %v, want &false", cfg2.App.OptionalFlag)
	}
	if cfg2.App.ValidatedRange != 75 {
		t.Errorf("ValidatedRange = %d, want 75", cfg2.App.ValidatedRange)
	}

	// 3. Validation failure check
	invalidDoc := ` + "`" + `
config_version 1

section app
	validated_range 999
end
` + "`" + `
	_, _, err = appconfig.LoadBytes(ctx, []byte(invalidDoc))
	if err == nil {
		t.Fatal("expected validation error for range 999, got nil")
	}
	if !strings.Contains(err.Error(), "out of bounds") {
		t.Errorf("error = %v, want out of bounds", err)
	}

	// 4. Sensitive failure redaction check
	sensitiveFailDoc := ` + "`" + `
config_version 1

section app
	secret_token invalid-secret-credentials-12345
end
` + "`" + `
	_, _, err = appconfig.LoadBytes(ctx, []byte(sensitiveFailDoc))
	if err == nil {
		t.Fatal("expected error on invalid secret token")
	}
	if strings.Contains(err.Error(), "invalid-secret-credentials-12345") {
		t.Fatalf("leak in error message! Sensitive value disclosed: %v", err)
	}
	if !strings.Contains(err.Error(), "failed") {
		t.Fatalf("unexpected error message: %v", err)
	}
}
`
	if err := os.WriteFile(filepath.Join(appConfigDir, "types_test.go"), []byte(testCode), 0644); err != nil {
		t.Fatal(err)
	}

	// 6. Execute go test in the standalone module
	cmd := exec.Command("go", "test", "-v", "./...")
	cmd.Dir = dir
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("standalone module go test failed: %v\nOutput:\n%s", err, string(out))
	}

	// 7. Assert ZERO runtime dependency on github.com/arran4/gogenconf
	depsCmd := exec.Command("go", "list", "-deps", "./...")
	depsCmd.Dir = dir
	depsOut, err := depsCmd.CombinedOutput()
	if err != nil {
		t.Fatalf("go list -deps failed: %v\nOutput:\n%s", err, string(depsOut))
	}
	if strings.Contains(string(depsOut), "github.com/arran4/gogenconf") {
		t.Fatalf("standalone module illegally depends on github.com/arran4/gogenconf:\n%s", string(depsOut))
	}
}

func TestResolveBuiltinTypes(t *testing.T) {
	ctx := context.Background()
	r, err := m.NewStandardRegistry()
	if err != nil {
		t.Fatal(err)
	}

	// int
	intVal, err := m.Resolve[int](ctx, r, m.Literal{Value: "42"})
	if err != nil || intVal != 42 {
		t.Fatalf("Resolve[int] = %d, %v", intVal, err)
	}

	// bool
	boolVal, err := m.Resolve[bool](ctx, r, m.Literal{Value: "true"})
	if err != nil || !boolVal {
		t.Fatalf("Resolve[bool] = %v, %v", boolVal, err)
	}

	// time.Duration
	durVal, err := m.Resolve[time.Duration](ctx, r, m.Literal{Value: "5s"})
	if err != nil || durVal != 5*time.Second {
		t.Fatalf("Resolve[time.Duration] = %v, %v", durVal, err)
	}

	// float64
	floatVal, err := m.Resolve[float64](ctx, r, m.Literal{Value: "3.14"})
	if err != nil || floatVal != 3.14 {
		t.Fatalf("Resolve[float64] = %v, %v", floatVal, err)
	}

	// []int
	sliceVal, err := m.Resolve[[]int](ctx, r, m.Literal{Value: "1, 2, 3"})
	if err != nil || len(sliceVal) != 3 || sliceVal[0] != 1 || sliceVal[1] != 2 || sliceVal[2] != 3 {
		t.Fatalf("Resolve[[]int] = %v, %v", sliceVal, err)
	}

	// *int
	ptrVal, err := m.Resolve[*int](ctx, r, m.Literal{Value: "100"})
	if err != nil || ptrVal == nil || *ptrVal != 100 {
		t.Fatalf("Resolve[*int] = %v, %v", ptrVal, err)
	}
}

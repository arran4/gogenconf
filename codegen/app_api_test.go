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

func sampleSchema() m.Schema {
	return m.Schema{
		Version: 2,
		Sections: []m.SectionDefinition{
			{
				Name:   "service",
				GoName: "Service",
				GoType: "ServiceConfig",
				Documentation: []string{
					"Service configuration block.",
				},
				Fields: []m.Field{
					{
						Key:           "endpoint",
						GoName:        "Endpoint",
						GoType:        "string",
						Default:       m.Literal{Value: "https://default.example.test"},
						Documentation: []string{"Endpoint URL for service connection."},
					},
					{
						Key:           "token",
						GoName:        "Token",
						GoType:        "string",
						Required:      true,
						Sensitive:     true,
						Documentation: []string{"Authentication token for API requests."},
					},
					{
						Key:           "legacy_flag",
						InputOnly:     true,
						Deprecated:    true,
						Documentation: []string{"Deprecated compatibility flag."},
					},
				},
			},
			{
				Name:     "worker",
				GoName:   "Workers",
				GoType:   "WorkerConfig",
				Repeated: true,
				ExampleNames: []string{
					"worker-1",
				},
				Documentation: []string{
					"Worker instances.",
				},
				Fields: []m.Field{
					{
						Key:     "concurrency",
						GoName:  "Concurrency",
						GoType:  "string",
						Default: m.Literal{Value: "4"},
					},
				},
			},
		},
	}
}

func TestGenerateAppAPIStructure(t *testing.T) {
	s := sampleSchema()
	apiCode, err := codegen.GenerateAppAPI(s, codegen.Options{
		Package:       "myconfig",
		LibraryImport: "github.com/arran4/gogenconf",
	})
	if err != nil {
		t.Fatalf("GenerateAppAPI failed: %v", err)
	}

	src := string(apiCode)
	expectedFragments := []string{
		"const SchemaVersion = 2",
		"func Load(ctx context.Context, d *gogenconf.Document",
		"func LoadReader(ctx context.Context, r io.Reader",
		"func LoadFile(ctx context.Context, path string",
		"func LoadBytes(ctx context.Context, data []byte",
		"func Save(w io.Writer, d *gogenconf.Document",
		"func SaveFile(path string, d *gogenconf.Document",
		"func Seed() *gogenconf.Document",
		"func Sample() *gogenconf.Document",
		"func Enrich(d *gogenconf.Document",
		"func Validate(d *gogenconf.Document, strict bool",
		"func Inspect(d *gogenconf.Document",
		"func InspectString(d *gogenconf.Document",
		"func Field(section, key string) (FieldMeta, bool)",
		"func IsSensitive(section, key string) bool",
		"func IsRequired(section, key string) bool",
		"func IsInputOnly(section, key string) bool",
		"func IsWritable(section, key string) bool",
		"type Migrations struct",
		"func NewMigrations() *Migrations",
	}

	for _, frag := range expectedFragments {
		if !strings.Contains(src, frag) {
			t.Errorf("missing expected fragment in generated API: %s", frag)
		}
	}
}

func TestGenerateAppAPICompilationAndRuntime(t *testing.T) {
	s := sampleSchema()

	// Generate standalone native runtime into runtime pkg
	rtFiles, err := codegen.GenerateRuntime(codegen.RuntimeOptions{
		Package:      "nativeconfig",
		Capabilities: codegen.FullRuntimeCapabilities(),
	})
	if err != nil {
		t.Fatalf("GenerateRuntime failed: %v", err)
	}

	// Generate application API importing standalone runtime pkg
	apiCode, err := codegen.GenerateAppAPI(s, codegen.Options{
		Package:       "appconfig",
		RuntimeImport: "example.test/app/nativeconfig",
	})
	if err != nil {
		t.Fatalf("GenerateAppAPI failed: %v", err)
	}

	// Verify no gogenconf import in the generated appconfig package
	if strings.Contains(string(apiCode), "github.com/arran4/gogenconf") {
		t.Fatal("generated appconfig illegally imports github.com/arran4/gogenconf")
	}

	dir := t.TempDir()

	// Write go.mod with NO gogenconf module dependency
	mod := "module example.test/app\n\ngo 1.25.0\n"
	if err := os.WriteFile(filepath.Join(dir, "go.mod"), []byte(mod), 0644); err != nil {
		t.Fatal(err)
	}

	// Write nativeconfig files
	rtDir := filepath.Join(dir, "nativeconfig")
	if err := rtFiles.WriteToDir(rtDir); err != nil {
		t.Fatal(err)
	}

	// Write appconfig file
	appDir := filepath.Join(dir, "appconfig")
	if err := os.MkdirAll(appDir, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(appDir, "config_generated.go"), apiCode, 0644); err != nil {
		t.Fatal(err)
	}

	// Write test application demonstrating the entire lifecycle
	mainContent := `package main

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"example.test/app/appconfig"
	"example.test/app/nativeconfig"
)

func main() {
	ctx := context.Background()

	// 1. Metadata queries
	if !appconfig.IsSensitive("service", "token") {
		panic("expected service.token to be sensitive")
	}
	if appconfig.IsSensitive("service", "endpoint") {
		panic("expected service.endpoint to not be sensitive")
	}
	if !appconfig.IsRequired("service", "token") {
		panic("expected service.token to be required")
	}
	if !appconfig.IsInputOnly("service", "legacy_flag") {
		panic("expected service.legacy_flag to be input-only")
	}
	if appconfig.IsWritable("service", "legacy_flag") {
		panic("expected input-only field to not be writable")
	}

	// 2. Seed document
	seeded := appconfig.Seed()
	if seeded.Version != appconfig.SchemaVersion {
		panic("seeded version mismatch")
	}
	if seeded.Section("service") == nil {
		panic("missing service section in seed")
	}

	// 3. Sample document
	sampled := appconfig.Sample()
	if sampled.Section("service") == nil {
		panic("missing service section in sample")
	}

	// 4. Migrations scaffolding
	migrations := appconfig.NewMigrations()
	migrations.Register(1, func(d *nativeconfig.Document) error {
		// Migration from v1 to v2: rename old_token to token
		sec := d.Section("service")
		if sec != nil {
			if entry, _, ok := sec.Entry("old_token"); ok {
				sec.Delete("old_token")
				sec.Set("token", entry.Value)
			}
		}
		return nil
	})

	// Input v1 document needing migration
	v1Input := "config_version 1\nsection service\n    old_token \"secret-api-key\"\nend\n"
	cfg, doc, err := appconfig.LoadBytes(ctx, []byte(v1Input),
		appconfig.WithMigrations(migrations),
	)
	if err != nil {
		panic(fmt.Sprintf("LoadBytes failed: %v", err))
	}
	if cfg.Service.Token != "secret-api-key" {
		panic("migration failed to resolve migrated token")
	}
	if cfg.Service.Endpoint != "https://default.example.test" {
		panic("default endpoint was not enriched")
	}

	// 5. Inspect and redaction
	inspected := appconfig.InspectString(doc)
	if strings.Contains(inspected, "secret-api-key") {
		panic("inspected string leaked sensitive token value")
	}
	if !strings.Contains(inspected, "[REDACTED]") {
		panic("expected [REDACTED] in inspected output")
	}

	// 6. Strict validation rejects unknown fields
	doc.Section("service").Set("unknown_setting", nativeconfig.Literal{Value: "val"})
	if err := appconfig.Validate(doc, true); err == nil {
		panic("expected strict validation to reject unknown_setting")
	}
	// Normal non-strict validation accepts unknown fields (forward compatibility)
	if err := appconfig.Validate(doc, false); err != nil {
		panic(fmt.Sprintf("non-strict validation failed: %v", err))
	}
	doc.Section("service").Delete("unknown_setting")

	// 7. Save and reload from file
	tmpFile := filepath.Join(os.TempDir(), "appconfig_test.conf")
	defer os.Remove(tmpFile)
	if err := appconfig.SaveFile(tmpFile, doc); err != nil {
		panic(err)
	}

	reloaded, reloadedDoc, err := appconfig.LoadFile(ctx, tmpFile)
	if err != nil {
		panic(err)
	}
	if reloaded.Service.Token != "secret-api-key" {
		panic("reloaded token mismatch")
	}
	if reloadedDoc.Version != appconfig.SchemaVersion {
		panic("reloaded document version mismatch")
	}

	fmt.Println("app config lifecycle succeeded")
}
`
	if err := os.WriteFile(filepath.Join(dir, "main.go"), []byte(mainContent), 0644); err != nil {
		t.Fatal(err)
	}

	// Verify dependencies using go list -deps
	listCmd := exec.Command("go", "list", "-deps", "./...")
	listCmd.Dir = dir
	listCmd.Env = append(os.Environ(), "GOWORK=off")
	deps, err := listCmd.CombinedOutput()
	if err != nil {
		t.Fatalf("go list failed: %v\n%s", err, deps)
	}
	if strings.Contains(string(deps), "github.com/arran4/gogenconf") {
		t.Fatalf("go list -deps illegally contains gogenconf:\n%s", deps)
	}

	// Build and run the consumer application
	runCmd := exec.Command("go", "run", ".")
	runCmd.Dir = dir
	runCmd.Env = append(os.Environ(), "GOWORK=off")
	out, err := runCmd.CombinedOutput()
	if err != nil {
		t.Fatalf("go run failed: %v\n%s", err, out)
	}
	if !strings.Contains(string(out), "app config lifecycle succeeded") {
		t.Fatalf("unexpected output: %s", out)
	}
}

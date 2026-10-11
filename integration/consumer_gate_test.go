package integration

import (
	"embed"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	m "github.com/arran4/gogenconf"
	"github.com/arran4/gogenconf/codegen"
	"github.com/arran4/gogenconf/internal/testutil"
	"golang.org/x/tools/txtar"
)

//go:embed testdata/consumer-gate/*.txtar
var consumerGateCases embed.FS

type gateOptions struct {
	Scenario    string `json:"scenario"`
	Description string `json:"description"`
	ExpectError bool   `json:"expect_error"`
}

type schemaModel struct {
	Version  int            `json:"version"`
	Sections []sectionModel `json:"sections"`
}

type sectionModel struct {
	Name     string       `json:"name"`
	GoName   string       `json:"go_name"`
	GoType   string       `json:"go_type"`
	Repeated bool         `json:"repeated"`
	Fields   []fieldModel `json:"fields"`
}

type fieldModel struct {
	Key         string `json:"key"`
	GoName      string `json:"go_name"`
	GoType      string `json:"go_type"`
	Default     string `json:"default"`
	DefaultExpr string `json:"default_expr"`
	Required    bool   `json:"required"`
	Sensitive   bool   `json:"sensitive"`
	InputOnly   bool   `json:"input_only"`
	RuntimeOnly bool   `json:"runtime_only"`
	Policy      string `json:"policy"`
	Environment string `json:"environment"`
}

func parseSchema(data []byte) (m.Schema, error) {
	var sm schemaModel
	if err := json.Unmarshal(data, &sm); err != nil {
		return m.Schema{}, err
	}
	s := m.Schema{Version: sm.Version}
	for _, sec := range sm.Sections {
		sd := m.SectionDefinition{
			Name:     sec.Name,
			GoName:   sec.GoName,
			GoType:   sec.GoType,
			Repeated: sec.Repeated,
		}
		for _, f := range sec.Fields {
			fd := m.Field{
				Key:         f.Key,
				GoName:      f.GoName,
				GoType:      f.GoType,
				Required:    f.Required,
				Sensitive:   f.Sensitive,
				InputOnly:   f.InputOnly,
				RuntimeOnly: f.RuntimeOnly,
				Environment: f.Environment,
			}
			if f.Default != "" {
				fd.Default = m.Literal{Value: f.Default}
			} else if f.DefaultExpr != "" {
				parsed, err := m.ParseExpr(f.DefaultExpr)
				if err != nil {
					return m.Schema{}, err
				}
				fd.Default = parsed
			}
			if f.Policy == "provider" {
				fd.Policy = m.ProviderBacked
			}
			sd.Fields = append(sd.Fields, fd)
		}
		s.Sections = append(s.Sections, sd)
	}
	return s, nil
}

func fileData(ar *txtar.Archive, name string) []byte {
	for _, f := range ar.Files {
		if f.Name == name {
			return f.Data
		}
	}
	return nil
}

// TestConsumerGate executes the mandatory Issue #10 verification gate:
// proving zero gogenconf runtime dependency across the full consumer feature matrix.
func TestConsumerGate(t *testing.T) {
	cases, err := testutil.LoadCases(consumerGateCases, "testdata/consumer-gate")
	if err != nil {
		t.Fatal(err)
	}
	if len(cases) < 9 {
		t.Fatalf("expected at least 9 consumer gate scenarios, discovered %d", len(cases))
	}

	for _, tc := range cases {
		name := strings.TrimSuffix(filepath.Base(tc.Path), ".txtar")
		t.Run(name, func(t *testing.T) {
			optsData := fileData(tc.Archive, "options.json")
			if optsData == nil {
				t.Fatal("missing options.json in archive")
			}
			var opts gateOptions
			if err := json.Unmarshal(optsData, &opts); err != nil {
				t.Fatalf("unmarshal options.json: %v", err)
			}

			schemaData := fileData(tc.Archive, "schema.json")
			if schemaData == nil {
				t.Fatal("missing schema.json in archive")
			}
			schema, err := parseSchema(schemaData)
			if err != nil {
				t.Fatalf("parse schema: %v", err)
			}

			// Negative test case handling
			if opts.ExpectError {
				errExpected := strings.TrimSpace(string(fileData(tc.Archive, "expected/error.txt")))
				_, err := codegen.GenerateAppAPI(schema, codegen.Options{
					Package:       "appconfig",
					RuntimeImport: "consumer.test/app/internal/nativeconfig",
				})
				if err == nil {
					t.Fatalf("scenario %s expected generation error, got nil", opts.Scenario)
				}
				if !strings.Contains(err.Error(), errExpected) {
					t.Fatalf("scenario %s error mismatch: got %q, want substring %q", opts.Scenario, err.Error(), errExpected)
				}
				return
			}

			// 1. Setup isolated consumer workspace
			dir := t.TempDir()
			goMod := "module consumer.test/app\n\ngo 1.24\n"
			if err := os.WriteFile(filepath.Join(dir, "go.mod"), []byte(goMod), 0644); err != nil {
				t.Fatal(err)
			}

			// 2. Generate native runtime
			runtimeFiles, err := codegen.GenerateRuntime(codegen.RuntimeOptions{
				Package: "nativeconfig",
				Capabilities: []codegen.Capability{
					codegen.CapabilityDocument,
					codegen.CapabilityExpr,
					codegen.CapabilityParser,
					codegen.CapabilityFormatter,
					codegen.CapabilityResolver,
					codegen.CapabilityProvider,
				},
			})
			if err != nil {
				t.Fatalf("GenerateRuntime failed: %v", err)
			}
			runtimeDir := filepath.Join(dir, "internal", "nativeconfig")
			if err := runtimeFiles.WriteToDir(runtimeDir); err != nil {
				t.Fatalf("WriteToDir runtime failed: %v", err)
			}

			// 3. Generate schema-specialized app configuration API
			appAPICode, err := codegen.GenerateAppAPI(schema, codegen.Options{
				Package:       "appconfig",
				RuntimeImport: "consumer.test/app/internal/nativeconfig",
			})
			if err != nil {
				t.Fatalf("GenerateAppAPI failed: %v", err)
			}
			appConfigDir := filepath.Join(dir, "appconfig")
			if err := os.MkdirAll(appConfigDir, 0755); err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(filepath.Join(appConfigDir, "config_generated.go"), appAPICode, 0644); err != nil {
				t.Fatal(err)
			}

			// 4. Materialize input tree from txtar
			inputTree, err := testutil.Tree(tc.Archive, "input")
			if err != nil {
				t.Fatalf("Tree(input) failed: %v", err)
			}
			for path, file := range inputTree {
				dest := filepath.Join(dir, filepath.FromSlash(path))
				if err := os.MkdirAll(filepath.Dir(dest), 0755); err != nil {
					t.Fatal(err)
				}
				if err := os.WriteFile(dest, file.Data, 0644); err != nil {
					t.Fatal(err)
				}
			}

			// 5. Assert generated runtime & appconfig contain ZERO gogenconf imports
			for _, rf := range runtimeFiles {
				if strings.Contains(string(rf.Content), "github.com/arran4/gogenconf") {
					t.Fatalf("runtime file %s illegally imports gogenconf", rf.Name)
				}
			}
			if strings.Contains(string(appAPICode), "github.com/arran4/gogenconf") {
				t.Fatalf("generated appconfig illegally imports gogenconf")
			}

			// 6. Assert go list -deps ./... contains NO github.com/arran4/gogenconf
			deps := run(t, dir, []string{"GOWORK=off"}, "", "go", "list", "-deps", "./...")
			if strings.Contains(deps, "github.com/arran4/gogenconf") {
				t.Fatalf("consumer module dependency graph illegally contains github.com/arran4/gogenconf:\n%s", deps)
			}

			// 7. Build and run consumer application
			run(t, dir, []string{"GOWORK=off"}, "", "go", "build", "./...")
			stdout := run(t, dir, []string{"GOWORK=off"}, "", "go", "run", ".")

			// 8. Compare stdout with expected
			expectedStdout := string(fileData(tc.Archive, "expected/stdout.txt"))
			if strings.TrimSpace(stdout) != strings.TrimSpace(expectedStdout) {
				t.Fatalf("stdout mismatch:\nGot:\n%s\nWant:\n%s", stdout, expectedStdout)
			}
		})
	}
}

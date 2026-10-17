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

// F-01: Generated Inspect and InspectString redact secrets when repeated section
// instance names contain dots ("foo.bar") or collide with child section names ("build").
func TestAudit_F01_GeneratedInspectRedaction(t *testing.T) {
	s := m.Schema{
		Identity: "testapp-schema",
		Version:  1,
		Sections: []m.SectionDefinition{{
			Name:     "project",
			GoName:   "Projects",
			GoType:   "ProjectConfig",
			Repeated: true,
			Fields: []m.Field{
				{Key: "api_secret", GoName: "APISecret", GoType: "string", Sensitive: true},
			},
			Children: []m.SectionDefinition{{
				Name:   "build",
				GoName: "Build",
				GoType: "BuildConfig",
				Fields: []m.Field{
					{Key: "signing_key", GoName: "SigningKey", GoType: "string", Sensitive: true},
				},
			}},
		}},
	}

	runtime, err := codegen.GenerateRuntime(codegen.RuntimeOptions{
		Package:      "nativeconfig",
		Capabilities: codegen.FullRuntimeCapabilities(),
	})
	if err != nil {
		t.Fatal(err)
	}
	generated, err := codegen.GenerateAppAPI(s, codegen.Options{
		Package:       "appconfig",
		RuntimeImport: "example.test/audit_f01/nativeconfig",
	})
	if err != nil {
		t.Fatal(err)
	}

	dir := t.TempDir()
	_ = os.WriteFile(filepath.Join(dir, "go.mod"), []byte("module example.test/audit_f01\n\ngo 1.25.0\n"), 0644)
	if err := runtime.WriteToDir(filepath.Join(dir, "nativeconfig")); err != nil {
		t.Fatal(err)
	}
	_ = os.MkdirAll(filepath.Join(dir, "appconfig"), 0755)
	_ = os.WriteFile(filepath.Join(dir, "appconfig", "config_generated.go"), generated, 0644)

	const mainSrc = `package main

import (
	"context"
	"fmt"
	"strings"
	"example.test/audit_f01/appconfig"
	"example.test/audit_f01/nativeconfig"
)

func main() {
	input := "config for testapp version 1\n\n" +
		"project \"foo.bar\" {\n" +
		"  api_secret \"SECRET_DOTTED_API_123\"\n" +
		"  build {\n" +
		"    signing_key \"SECRET_DOTTED_SIGN_456\"\n" +
		"  }\n" +
		"}\n" +
		"project build {\n" +
		"  api_secret \"SECRET_COLLIDE_API_789\"\n" +
		"  build {\n" +
		"    signing_key \"SECRET_COLLIDE_SIGN_000\"\n" +
		"  }\n" +
		"}\n"

	doc, err := nativeconfig.Parse(strings.NewReader(input))
	if err != nil {
		panic(err)
	}
	if _, err := appconfig.Load(context.Background(), doc, appconfig.WithStrict(true)); err != nil {
		panic(err)
	}
	inspected := appconfig.InspectString(doc)
	for _, secret := range []string{
		"SECRET_DOTTED_API_123",
		"SECRET_DOTTED_SIGN_456",
		"SECRET_COLLIDE_API_789",
		"SECRET_COLLIDE_SIGN_000",
	} {
		if strings.Contains(inspected, secret) {
			panic(fmt.Sprintf("LEAK: %s was not redacted by InspectString:\n%s", secret, inspected))
		}
	}
}
`
	_ = os.WriteFile(filepath.Join(dir, "main.go"), []byte(mainSrc), 0644)
	cmd := exec.Command("go", "run", ".")
	cmd.Dir = dir
	cmd.Env = append(os.Environ(), "GOWORK=off")
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("Audit F-01 regression failed: %v\n%s", err, out)
	}
}

// F-02: Multiple canonical brace container blocks (e.g., project { alpha { ... } }
// followed by project { beta { ... } }) enrich and resolve both instances instead of dropping the second.
func TestAudit_F02_MultipleBraceContainerBlocks(t *testing.T) {
	s := m.Schema{
		Identity: "buildtool-schema",
		Version:  1,
		Sections: []m.SectionDefinition{{
			Name:     "project",
			GoName:   "Projects",
			GoType:   "ProjectConfig",
			Repeated: true,
			Fields: []m.Field{
				{Key: "module_path", GoName: "ModulePath", GoType: "string", Default: m.Literal{Value: "default/mod"}},
			},
			Children: []m.SectionDefinition{{
				Name:   "build",
				GoName: "Build",
				GoType: "BuildConfig",
				Fields: []m.Field{
					{Key: "target", GoName: "Target", GoType: "string", Required: true},
				},
			}},
		}},
	}

	runtime, err := codegen.GenerateRuntime(codegen.RuntimeOptions{
		Package:      "nativeconfig",
		Capabilities: codegen.FullRuntimeCapabilities(),
	})
	if err != nil {
		t.Fatal(err)
	}
	generated, err := codegen.GenerateAppAPI(s, codegen.Options{
		Package:       "appconfig",
		RuntimeImport: "example.test/audit_f02/nativeconfig",
	})
	if err != nil {
		t.Fatal(err)
	}

	dir := t.TempDir()
	_ = os.WriteFile(filepath.Join(dir, "go.mod"), []byte("module example.test/audit_f02\n\ngo 1.25.0\n"), 0644)
	if err := runtime.WriteToDir(filepath.Join(dir, "nativeconfig")); err != nil {
		t.Fatal(err)
	}
	_ = os.MkdirAll(filepath.Join(dir, "appconfig"), 0755)
	_ = os.WriteFile(filepath.Join(dir, "appconfig", "config_generated.go"), generated, 0644)

	const mainSrc = `package main

import (
	"context"
	"fmt"
	"example.test/audit_f02/appconfig"
)

func main() {
	input := []byte("config for buildtool version 1\n\n" +
		"project {\n  alpha {\n    build {\n      target \"./cmd/alpha\"\n    }\n  }\n}\n" +
		"project {\n  beta {\n    build {\n      target \"./cmd/beta\"\n    }\n  }\n}\n")

	cfg, _, err := appconfig.LoadBytes(context.Background(), input, appconfig.WithStrict(true))
	if err != nil {
		panic(err)
	}
	if len(cfg.Projects) != 2 {
		panic(fmt.Sprintf("expected 2 projects (alpha, beta), got %d: %+v", len(cfg.Projects), cfg.Projects))
	}
	if cfg.Projects[0].Name != "alpha" || cfg.Projects[0].Build.Target != "./cmd/alpha" {
		panic(fmt.Sprintf("project 0 mismatch: %+v", cfg.Projects[0]))
	}
	if cfg.Projects[1].Name != "beta" || cfg.Projects[1].Build.Target != "./cmd/beta" {
		panic(fmt.Sprintf("project 1 mismatch: %+v", cfg.Projects[1]))
	}
}
`
	_ = os.WriteFile(filepath.Join(dir, "main.go"), []byte(mainSrc), 0644)
	cmd := exec.Command("go", "run", ".")
	cmd.Dir = dir
	cmd.Env = append(os.Environ(), "GOWORK=off")
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("Audit F-02 regression failed: %v\n%s", err, out)
	}
}

// F-03 (Part B): Map defaults, pointer-slice defaults compile cleanly and execute correctly.
func TestAudit_F03_MapDefaultsAndPointerSliceCompilation(t *testing.T) {
	s := m.Schema{
		Version: 1,
		RootFields: []m.Field{
			{Key: "labels", GoName: "Labels", GoType: "map[string]string", Default: m.Map{
				Entries: []m.MapEntry{
					{Key: "env", Value: m.Literal{Value: "production"}},
					{Key: "region", Value: m.Literal{Value: "us-east-1"}},
				},
			}},
			{Key: "routes", GoName: "Routes", GoType: "[$]int", Default: m.Map{
				Entries: []m.MapEntry{
					{Key: "http", Value: m.Literal{Value: "80"}},
					{Key: "https", Value: m.Literal{Value: "443"}},
				},
			}},
			{Key: "counts", GoName: "Counts", GoType: "[]*int", Default: m.Literal{Value: "1,2,3"}},
			{Key: "tags", GoName: "Tags", GoType: "[]*string", Default: m.Literal{Value: "alpha,beta"}},
			{Key: "clusters", GoName: "Clusters", GoType: "map[string]map[string]string", Default: m.Map{
				Entries: []m.MapEntry{
					{Key: "prod", Value: m.Map{
						Entries: []m.MapEntry{
							{Key: "region", Value: m.Literal{Value: "us-east-1"}},
							{Key: "zone", Value: m.Literal{Value: "us-east-1a"}},
						},
					}},
				},
			}},
		},
		Sections: []m.SectionDefinition{{
			Name:   "service",
			GoName: "Service",
			GoType: "ServiceConfig",
			Fields: []m.Field{
				{Key: "metadata", GoName: "Metadata", GoType: "map[string]string", Default: m.Map{
					Entries: []m.MapEntry{
						{Key: "tier", Value: m.Literal{Value: "backend"}},
					},
				}},
				{Key: "overrides", GoName: "Overrides", GoType: "map[string]map[string]string", Default: m.Map{
					Entries: []m.MapEntry{
						{Key: "stage", Value: m.Map{
							Entries: []m.MapEntry{
								{Key: "canary", Value: m.Literal{Value: "true"}},
							},
						}},
					},
				}},
			},
		}},
	}
	runtime, err := codegen.GenerateRuntime(codegen.RuntimeOptions{
		Package:      "nativeconfig",
		Capabilities: codegen.FullRuntimeCapabilities(),
	})
	if err != nil {
		t.Fatal(err)
	}
	generated, err := codegen.GenerateAppAPI(s, codegen.Options{
		Package:       "appconfig",
		RuntimeImport: "example.test/audit_f03/nativeconfig",
	})
	if err != nil {
		t.Fatalf("GenerateAppAPI failed: %v", err)
	}
	dir := t.TempDir()
	_ = os.WriteFile(filepath.Join(dir, "go.mod"), []byte("module example.test/audit_f03\n\ngo 1.25.0\n"), 0644)
	if err := runtime.WriteToDir(filepath.Join(dir, "nativeconfig")); err != nil {
		t.Fatal(err)
	}
	_ = os.MkdirAll(filepath.Join(dir, "appconfig"), 0755)
	_ = os.WriteFile(filepath.Join(dir, "appconfig", "config_generated.go"), generated, 0644)

	const mainSrc = `package main

import (
	"context"
	"fmt"
	"example.test/audit_f03/appconfig"
)

func main() {
	defaults, err := appconfig.Defaults(nil)
	if err != nil {
		panic(err)
	}
	// Verify pointer-slice int defaults
	if len(defaults.Counts) != 3 || defaults.Counts[0] == nil || *defaults.Counts[0] != 1 || defaults.Counts[2] == nil || *defaults.Counts[2] != 3 {
		panic(fmt.Sprintf("unexpected counts defaults: %+v", defaults.Counts))
	}
	if defaults.Counts[0] == defaults.Counts[1] || defaults.Counts[1] == defaults.Counts[2] {
		panic("pointer slice elements must not share identical pointer addresses")
	}

	// Verify pointer-slice string defaults
	if len(defaults.Tags) != 2 || defaults.Tags[0] == nil || *defaults.Tags[0] != "alpha" || defaults.Tags[1] == nil || *defaults.Tags[1] != "beta" {
		panic(fmt.Sprintf("unexpected tags defaults: %+v", defaults.Tags))
	}
	if defaults.Tags[0] == defaults.Tags[1] {
		panic("string pointer slice elements must not share identical pointer addresses")
	}

	// Verify map defaults in root and sections
	if defaults.Labels == nil || defaults.Labels["env"] != "production" || defaults.Labels["region"] != "us-east-1" {
		panic(fmt.Sprintf("unexpected labels map default: %+v", defaults.Labels))
	}
	if defaults.Routes == nil || defaults.Routes["http"] != 80 || defaults.Routes["https"] != 443 {
		panic(fmt.Sprintf("unexpected routes map default: %+v", defaults.Routes))
	}
	if defaults.Service.Metadata == nil || defaults.Service.Metadata["tier"] != "backend" {
		panic(fmt.Sprintf("unexpected service.metadata default: %+v", defaults.Service.Metadata))
	}

	// Verify nested map defaults in root and sections
	if defaults.Clusters == nil || defaults.Clusters["prod"] == nil || defaults.Clusters["prod"]["region"] != "us-east-1" || defaults.Clusters["prod"]["zone"] != "us-east-1a" {
		panic(fmt.Sprintf("unexpected clusters nested map default: %+v", defaults.Clusters))
	}
	if defaults.Service.Overrides == nil || defaults.Service.Overrides["stage"] == nil || defaults.Service.Overrides["stage"]["canary"] != "true" {
		panic(fmt.Sprintf("unexpected service.overrides nested map default: %+v", defaults.Service.Overrides))
	}

	// Verify parity with Load(Seed())
	loaded, err := appconfig.Load(context.Background(), appconfig.Seed())
	if err != nil {
		panic(err)
	}
	if loaded.Labels["env"] != "production" || loaded.Routes["http"] != 80 || loaded.Service.Metadata["tier"] != "backend" ||
		loaded.Clusters["prod"]["region"] != "us-east-1" || loaded.Clusters["prod"]["zone"] != "us-east-1a" ||
		loaded.Service.Overrides["stage"]["canary"] != "true" {
		panic("Seed/Load parity failure for map defaults")
	}
}
`
	_ = os.WriteFile(filepath.Join(dir, "main.go"), []byte(mainSrc), 0644)
	cmd := exec.Command("go", "run", ".")
	cmd.Dir = dir
	cmd.Env = append(os.Environ(), "GOWORK=off")
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("Audit F-03 regression failed: %v\n%s", err, out)
	}
}

// F-03: Unsupported map environment fallbacks and literal defaults must fail early and explicitly during validation.
func TestAudit_F03_MapEnvironmentFallbackRejected(t *testing.T) {
	testCases := []struct {
		name    string
		schema  m.Schema
		wantErr string
	}{
		{
			name: "root map[string]string with environment",
			schema: m.Schema{
				Version: 1,
				RootFields: []m.Field{
					{Key: "labels", GoName: "Labels", GoType: "map[string]string", Environment: "APP_LABELS"},
				},
			},
			wantErr: "root.labels: environment fallback is unsupported for map fields",
		},
		{
			name: "root [$]string with environment",
			schema: m.Schema{
				Version: 1,
				RootFields: []m.Field{
					{Key: "routes", GoName: "Routes", GoType: "[$]string", Environment: "APP_ROUTES"},
				},
			},
			wantErr: "root.routes: environment fallback is unsupported for map fields",
		},
		{
			name: "root map with environment_non_empty",
			schema: m.Schema{
				Version: 1,
				RootFields: []m.Field{
					{Key: "labels", GoName: "Labels", GoType: "map[string]string", EnvironmentNonEmpty: true},
				},
			},
			wantErr: "root.labels: environment fallback is unsupported for map fields",
		},
		{
			name: "section map[string]string with environment",
			schema: m.Schema{
				Version: 1,
				Sections: []m.SectionDefinition{{
					Name:   "service",
					GoName: "Service",
					GoType: "ServiceConfig",
					Fields: []m.Field{
						{Key: "tags", GoName: "Tags", GoType: "map[string]string", Environment: "SVC_TAGS"},
					},
				}},
			},
			wantErr: "service.tags: environment fallback is unsupported for map fields",
		},
		{
			name: "section [$]int with environment",
			schema: m.Schema{
				Version: 1,
				Sections: []m.SectionDefinition{{
					Name:   "service",
					GoName: "Service",
					GoType: "ServiceConfig",
					Fields: []m.Field{
						{Key: "ports", GoName: "Ports", GoType: "[$]int", Environment: "SVC_PORTS"},
					},
				}},
			},
			wantErr: "service.ports: environment fallback is unsupported for map fields",
		},
		{
			name: "nested child section map with environment",
			schema: m.Schema{
				Version: 1,
				Sections: []m.SectionDefinition{{
					Name:   "service",
					GoName: "Service",
					GoType: "ServiceConfig",
					Children: []m.SectionDefinition{{
						Name:   "cluster",
						GoName: "Cluster",
						GoType: "ClusterConfig",
						Fields: []m.Field{
							{Key: "nodes", GoName: "Nodes", GoType: "map[string]string", Environment: "CLUSTER_NODES"},
						},
					}},
				}},
			},
			wantErr: "service.cluster.nodes: environment fallback is unsupported for map fields",
		},
		{
			name: "root map with literal default",
			schema: m.Schema{
				Version: 1,
				RootFields: []m.Field{
					{Key: "labels", GoName: "Labels", GoType: "map[string]string", Default: m.Literal{Value: "foo=bar"}},
				},
			},
			wantErr: "root.labels: literal or non-map default is unsupported for map fields",
		},
		{
			name: "section map with literal default",
			schema: m.Schema{
				Version: 1,
				Sections: []m.SectionDefinition{{
					Name:   "service",
					GoName: "Service",
					GoType: "ServiceConfig",
					Fields: []m.Field{
						{Key: "labels", GoName: "Labels", GoType: "map[string]string", Default: m.Literal{Value: "foo=bar"}},
					},
				}},
			},
			wantErr: "service.labels: literal or non-map default is unsupported for map fields",
		},
		{
			name: "root nested map with literal default entry",
			schema: m.Schema{
				Version: 1,
				RootFields: []m.Field{
					{Key: "clusters", GoName: "Clusters", GoType: "map[string]map[string]string", Default: m.Map{
						Entries: []m.MapEntry{
							{Key: "prod", Value: m.Literal{Value: "not-a-map"}},
						},
					}},
				},
			},
			wantErr: "root.clusters.prod: literal or non-map default is unsupported for map fields",
		},
		{
			name: "root nested map with call default entry",
			schema: m.Schema{
				Version: 1,
				RootFields: []m.Field{
					{Key: "clusters", GoName: "Clusters", GoType: "map[string]map[string]string", Default: m.Map{
						Entries: []m.MapEntry{
							{Key: "prod", Value: m.Map{
								Entries: []m.MapEntry{
									{Key: "region", Value: m.Call{Name: "env", Args: []m.Expr{m.Literal{Value: "REGION"}}}},
								},
							}},
						},
					}},
				},
			},
			wantErr: "root.clusters.prod.region: unsupported expression in map default",
		},
		{
			name: "root map with invalid integer default entry",
			schema: m.Schema{
				Version: 1,
				RootFields: []m.Field{
					{Key: "limits", GoName: "Limits", GoType: "map[string]int", Default: m.Map{
						Entries: []m.MapEntry{
							{Key: "max", Value: m.Literal{Value: "not-an-int"}},
						},
					}},
				},
			},
			wantErr: "root.limits.max: invalid integer default",
		},
		{
			name: "root map default with empty key",
			schema: m.Schema{
				Version: 1,
				RootFields: []m.Field{
					{Key: "labels", GoName: "Labels", GoType: "map[string]string", Default: m.Map{
						Entries: []m.MapEntry{
							{Key: "", Value: m.Literal{Value: "val"}},
						},
					}},
				},
			},
			wantErr: "root.labels: empty key in map default",
		},
		{
			name: "root map default with nil value",
			schema: m.Schema{
				Version: 1,
				RootFields: []m.Field{
					{Key: "labels", GoName: "Labels", GoType: "map[string]string", Default: m.Map{
						Entries: []m.MapEntry{
							{Key: "foo", Value: nil},
						},
					}},
				},
			},
			wantErr: "root.labels.foo: nil entry value in map default",
		},
		{
			name: "section nested map with literal default entry",
			schema: m.Schema{
				Version: 1,
				Sections: []m.SectionDefinition{{
					Name:   "service",
					GoName: "Service",
					GoType: "ServiceConfig",
					Fields: []m.Field{
						{Key: "overrides", GoName: "Overrides", GoType: "map[string]map[string]string", Default: m.Map{
							Entries: []m.MapEntry{
								{Key: "stage", Value: m.Literal{Value: "not-a-map"}},
							},
						}},
					},
				}},
			},
			wantErr: "service.overrides.stage: literal or non-map default is unsupported for map fields",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			_, err := codegen.GenerateAppAPI(tc.schema, codegen.Options{
				Package:       "appconfig",
				RuntimeImport: "example.test/reject/nativeconfig",
			})
			if err == nil {
				t.Fatalf("expected error containing %q, got nil", tc.wantErr)
			}
			if !strings.Contains(err.Error(), tc.wantErr) {
				t.Fatalf("expected error containing %q, got %v", tc.wantErr, err)
			}
		})
	}
}

// F-04: emitSectionValueResolve executes field-level SanitizeConfig() and ValidateConfig() hooks
// on decoded field values in canonical brace repeated sections and nested child sections.
func TestAudit_F04_HooksInBraceAndNestedSections(t *testing.T) {
	s := m.Schema{
		Identity: "hookapp-schema",
		Version:  1,
		Sections: []m.SectionDefinition{{
			Name:     "project",
			GoName:   "Projects",
			GoType:   "ProjectConfig",
			Repeated: true,
			Fields: []m.Field{
				{Key: "tag", GoName: "Tag", GoType: "ValidatedTag"},
			},
			Children: []m.SectionDefinition{{
				Name:   "build",
				GoName: "Build",
				GoType: "BuildConfig",
				Fields: []m.Field{
					{Key: "nested_tag", GoName: "NestedTag", GoType: "ValidatedTag"},
				},
			}},
		}},
	}
	runtime, err := codegen.GenerateRuntime(codegen.RuntimeOptions{
		Package:      "nativeconfig",
		Capabilities: codegen.FullRuntimeCapabilities(),
	})
	if err != nil {
		t.Fatal(err)
	}
	generated, err := codegen.GenerateAppAPI(s, codegen.Options{
		Package:       "appconfig",
		RuntimeImport: "example.test/audit_f04/nativeconfig",
	})
	if err != nil {
		t.Fatal(err)
	}
	dir := t.TempDir()
	_ = os.WriteFile(filepath.Join(dir, "go.mod"), []byte("module example.test/audit_f04\n\ngo 1.25.0\n"), 0644)
	if err := runtime.WriteToDir(filepath.Join(dir, "nativeconfig")); err != nil {
		t.Fatal(err)
	}
	_ = os.MkdirAll(filepath.Join(dir, "appconfig"), 0755)
	_ = os.WriteFile(filepath.Join(dir, "appconfig", "config_generated.go"), generated, 0644)
	_ = os.WriteFile(filepath.Join(dir, "appconfig", "ext.go"), []byte(`package appconfig
import ("fmt"; "strings")
type ValidatedTag string
func (v *ValidatedTag) DecodeConfig(raw string) error {
	*v = ValidatedTag(raw)
	return nil
}
func (v *ValidatedTag) SanitizeConfig() error {
	*v = ValidatedTag(strings.TrimSpace(string(*v)))
	return nil
}
func (v *ValidatedTag) ValidateConfig() error {
	if string(*v) == "FORBIDDEN" {
		return fmt.Errorf("tag FORBIDDEN is not allowed")
	}
	return nil
}
`), 0644)

	const mainSrc = `package main

import (
	"context"
	"strings"
	"example.test/audit_f04/appconfig"
)

func main() {
	// Canonical brace section with forbidden tag: ValidateConfig must reject it!
	braceIn := []byte("config for hookapp version 1\n\nproject lxa {\n  tag \"  FORBIDDEN  \"\n}\n")
	if _, _, err := appconfig.LoadBytes(context.Background(), braceIn); err == nil {
		panic("expected ValidateConfig error in canonical brace section, got nil")
	} else if !strings.Contains(err.Error(), "tag FORBIDDEN is not allowed") {
		panic("unexpected error message: " + err.Error())
	}

	// Valid tag should be sanitized (trimmed)
	validIn := []byte("config for hookapp version 1\n\nproject lxa {\n  tag \"  allowed_tag  \"\n}\n")
	cfg, _, err := appconfig.LoadBytes(context.Background(), validIn)
	if err != nil {
		panic(err)
	}
	if cfg.Projects[0].Tag != "allowed_tag" {
		panic("expected sanitized tag 'allowed_tag', got: " + string(cfg.Projects[0].Tag))
	}
}
`
	_ = os.WriteFile(filepath.Join(dir, "main.go"), []byte(mainSrc), 0644)
	cmd := exec.Command("go", "run", ".")
	cmd.Dir = dir
	cmd.Env = append(os.Environ(), "GOWORK=off")
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("Audit F-04 regression failed: %v\n%s", err, out)
	}
}

// F-06 (Part C): ResolveField returns *ConfigError with StageResolve, Path, and Source span on failure.
func TestAudit_F06_StructuredResolveFieldConfigError(t *testing.T) {
	runtime, err := codegen.GenerateRuntime(codegen.RuntimeOptions{
		Package:      "nativeconfig",
		Capabilities: codegen.FullRuntimeCapabilities(),
	})
	if err != nil {
		t.Fatal(err)
	}
	dir := t.TempDir()
	_ = os.WriteFile(filepath.Join(dir, "go.mod"), []byte("module example.test/audit_f06\n\ngo 1.25.0\n"), 0644)
	if err := runtime.WriteToDir(filepath.Join(dir, "nativeconfig")); err != nil {
		t.Fatal(err)
	}

	const testSrc = `package main

import (
	"context"
	"errors"
	"fmt"
	"example.test/audit_f06/nativeconfig"
)

func main() {
	r := nativeconfig.NewRegistry()
	_ = r.Register("failing_provider", "", func(ctx context.Context, reg *nativeconfig.Registry, args []nativeconfig.Expr) (any, error) {
		return nil, errors.New("underlying network failure")
	})

	call := nativeconfig.Call{Name: "failing_provider", Args: nil, Span: nativeconfig.Span{Source: "test.conf", Line: 4, Column: 2}}
	
	// Test non-sensitive field resolution error
	_, err := nativeconfig.ResolveField[string](context.Background(), r, call, "service.token", false)
	if err == nil {
		panic("expected error, got nil")
	}
	var cfgErr *nativeconfig.ConfigError
	if !errors.As(err, &cfgErr) {
		panic(fmt.Sprintf("expected *ConfigError, got %T: %v", err, err))
	}
	if cfgErr.Stage != nativeconfig.StageResolve {
		panic(fmt.Sprintf("expected StageResolve, got %v", cfgErr.Stage))
	}
	if cfgErr.Path != "service.token" {
		panic(fmt.Sprintf("expected path service.token, got %s", cfgErr.Path))
	}
	if cfgErr.Source.Line != 4 {
		panic(fmt.Sprintf("expected source line 4, got %d", cfgErr.Source.Line))
	}

	// Test sensitive field resolution error (redacted cause)
	_, err = nativeconfig.ResolveField[string](context.Background(), r, call, "service.secret", true)
	if err == nil {
		panic("expected error, got nil")
	}
	if !errors.As(err, &cfgErr) {
		panic(fmt.Sprintf("expected *ConfigError for sensitive field, got %T: %v", err, err))
	}
	if cfgErr.Err.Error() != "source resolution failed" {
		panic(fmt.Sprintf("expected sensitive redaction 'source resolution failed', got: %v", cfgErr.Err))
	}
}
`
	_ = os.WriteFile(filepath.Join(dir, "main.go"), []byte(testSrc), 0644)
	cmd := exec.Command("go", "run", ".")
	cmd.Dir = dir
	cmd.Env = append(os.Environ(), "GOWORK=off")
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("Audit F-06 (Part C) regression failed: %v\n%s", err, out)
	}
}

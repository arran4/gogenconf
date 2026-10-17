package codegen_test

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	m "github.com/arran4/gogenconf"
	"github.com/arran4/gogenconf/codegen"
)

// historicalRepoPath returns the absolute path to the frozen historical checkout
// at revision d000175b840edf42166bf2f441b7dbce3153a7de.
func historicalRepoPath(t *testing.T) string {
	t.Helper()
	p, err := filepath.Abs("testdata/historical/repo")
	if err != nil {
		t.Fatalf("resolve historical repo path: %v", err)
	}
	if fi, err := os.Stat(p); err != nil || !fi.IsDir() {
		t.Fatalf("historical repo directory missing at %s: %v", p, err)
	}
	return p
}

// TestHistoricalBaselineParity_Basic verifies behavioral parity between the frozen
// historical implementation (d000175b840edf42166bf2f441b7dbce3153a7de) and the new
// static decoder on identical basic scalar schema and inputs.
func TestHistoricalBaselineParity_Basic(t *testing.T) {
	histRepo := historicalRepoPath(t)

	// Matching schema: service with endpoint (defaulted) and credential (required, sensitive)
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
						Default: m.Literal{Value: "https://example.test"},
					},
					{
						Key:       "credential",
						GoName:    "Credential",
						GoType:    "[]byte",
						Required:  true,
						Sensitive: true,
					},
				},
			},
		},
	}

	// Generate static app API and native runtime for the matching schema
	staticAppBytes, err := codegen.GenerateAppAPI(schema, codegen.Options{
		Package:       "staticapp",
		RuntimeImport: "example.test/diffbasic/nativeconfig",
	})
	if err != nil {
		t.Fatalf("GenerateAppAPI: %v", err)
	}

	runtimeFiles, err := codegen.GenerateRuntime(codegen.RuntimeOptions{
		Package:      "nativeconfig",
		Capabilities: codegen.FullRuntimeCapabilities(),
	})
	if err != nil {
		t.Fatalf("GenerateRuntime: %v", err)
	}

	dir := t.TempDir()

	// go.mod replacing github.com/arran4/gogenconf with the frozen historical checkout
	goMod := fmt.Sprintf("module example.test/diffbasic\n\ngo 1.25.0\n\nrequire github.com/arran4/gogenconf v0.0.0\n\nreplace github.com/arran4/gogenconf => %s\n", histRepo)
	if err := os.WriteFile(filepath.Join(dir, "go.mod"), []byte(goMod), 0644); err != nil {
		t.Fatal(err)
	}

	// Write static app API and native runtime
	if err := runtimeFiles.WriteToDir(filepath.Join(dir, "nativeconfig")); err != nil {
		t.Fatal(err)
	}
	staticDir := filepath.Join(dir, "staticapp")
	if err := os.MkdirAll(staticDir, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(staticDir, "config_generated.go"), staticAppBytes, 0644); err != nil {
		t.Fatal(err)
	}

	// Harness executing both frozen historical binder and new static decoder
	const harness = `package main

import (
	"bytes"
	"context"
	"fmt"
	"strings"

	"github.com/arran4/gogenconf"
	hist_bind "github.com/arran4/gogenconf/examples/generated/basic/binding"
	"example.test/diffbasic/nativeconfig"
	"example.test/diffbasic/staticapp"
)

func main() {
	ctx := context.Background()

	// Case 1: Populated fields parity
	{
		input := "config_version 1\nsection service\n  endpoint \"https://custom.service.test\"\n  credential \"secret-token-12345\"\nend\n"

		// Historical resolve
		docHist, err := gogenconf.Parse(strings.NewReader(input))
		if err != nil {
			panic(fmt.Sprintf("hist parse: %v", err))
		}
		regHist, err := gogenconf.NewStandardRegistry()
		if err != nil {
			panic(fmt.Sprintf("hist reg: %v", err))
		}
		cfgHist, err := hist_bind.Resolve(ctx, docHist, regHist, nil)
		if err != nil {
			panic(fmt.Sprintf("hist resolve: %v", err))
		}

		// Static decoder resolve
		docStatic, err := nativeconfig.Parse(strings.NewReader(input))
		if err != nil {
			panic(fmt.Sprintf("static parse: %v", err))
		}
		cfgStatic, err := staticapp.Load(ctx, docStatic)
		if err != nil {
			panic(fmt.Sprintf("static load: %v", err))
		}

		if cfgHist.Service.Endpoint != cfgStatic.Service.Endpoint {
			panic(fmt.Sprintf("case 1 endpoint mismatch: hist=%q, static=%q", cfgHist.Service.Endpoint, cfgStatic.Service.Endpoint))
		}
		if !bytes.Equal(cfgHist.Service.Credential, cfgStatic.Service.Credential) {
			panic(fmt.Sprintf("case 1 credential mismatch: hist=%q, static=%q", cfgHist.Service.Credential, cfgStatic.Service.Credential))
		}
		if cfgStatic.Service.Endpoint != "https://custom.service.test" {
			panic(fmt.Sprintf("case 1 unexpected endpoint: %q", cfgStatic.Service.Endpoint))
		}
		if string(cfgStatic.Service.Credential) != "secret-token-12345" {
			panic(fmt.Sprintf("case 1 unexpected credential: %q", cfgStatic.Service.Credential))
		}
	}

	// Case 2: Schema default parity when optional field omitted
	{
		input := "config_version 1\nsection service\n  credential \"secret-token-12345\"\nend\n"

		docHist, err := gogenconf.Parse(strings.NewReader(input))
		if err != nil {
			panic(err)
		}
		regHist, _ := gogenconf.NewStandardRegistry()
		cfgHist, err := hist_bind.Resolve(ctx, docHist, regHist, nil)
		if err != nil {
			panic(err)
		}

		docStatic, err := nativeconfig.Parse(strings.NewReader(input))
		if err != nil {
			panic(err)
		}
		cfgStatic, err := staticapp.Load(ctx, docStatic)
		if err != nil {
			panic(err)
		}

		if cfgHist.Service.Endpoint != cfgStatic.Service.Endpoint {
			panic(fmt.Sprintf("case 2 default endpoint mismatch: hist=%q, static=%q", cfgHist.Service.Endpoint, cfgStatic.Service.Endpoint))
		}
		if cfgStatic.Service.Endpoint != "https://example.test" {
			panic(fmt.Sprintf("case 2 unexpected default: %q", cfgStatic.Service.Endpoint))
		}
	}

	// Case 3: Missing required field failure parity
	{
		input := "config_version 1\nsection service\n  endpoint \"https://custom.service.test\"\nend\n"

		docHist, err := gogenconf.Parse(strings.NewReader(input))
		if err != nil {
			panic(err)
		}
		regHist, _ := gogenconf.NewStandardRegistry()
		_, errHist := hist_bind.Resolve(ctx, docHist, regHist, nil)
		if errHist == nil {
			panic("hist expected error on missing required credential, got nil")
		}

		docStatic, err := nativeconfig.Parse(strings.NewReader(input))
		if err != nil {
			panic(err)
		}
		_, errStatic := staticapp.Load(ctx, docStatic)
		if errStatic == nil {
			panic("static expected error on missing required credential, got nil")
		}

		if !strings.Contains(errHist.Error(), "service.credential") {
			panic(fmt.Sprintf("hist error %q does not mention service.credential", errHist.Error()))
		}
		if !strings.Contains(errStatic.Error(), "service.credential") {
			panic(fmt.Sprintf("static error %q does not mention service.credential", errStatic.Error()))
		}
	}

	// Case 4: Real sensitive conversion/resolution failure redaction
	{
		const secretPayload = "super-secret-token-value-9999"
		input := "config_version 1\nsection service\n  endpoint \"https://custom.service.test\"\n  credential fail_secret(\"" + secretPayload + "\")\nend\n"

		// 1. Historical resolve with failing declarer on sensitive field
		docHist, err := gogenconf.Parse(strings.NewReader(input))
		if err != nil {
			panic(fmt.Sprintf("hist parse: %v", err))
		}
		regHist, _ := gogenconf.NewStandardRegistry()
		_ = regHist.Register("fail_secret", []byte{}, func(ctx context.Context, r *gogenconf.Registry, args []gogenconf.Expr) (any, error) {
			return nil, fmt.Errorf("internal failure with secret content: %s", secretPayload)
		})
		_, errHist := hist_bind.Resolve(ctx, docHist, regHist, nil)
		if errHist == nil {
			panic("hist expected error on failing sensitive declarer, got nil")
		}
		if strings.Contains(errHist.Error(), secretPayload) {
			panic("hist leaked secretPayload in error message!")
		}
		if !strings.Contains(errHist.Error(), "service.credential") {
			panic(fmt.Sprintf("hist error %q does not identify service.credential", errHist.Error()))
		}
		if !strings.Contains(errHist.Error(), "source resolution failed") {
			panic(fmt.Sprintf("hist error %q does not satisfy documented redaction contract", errHist.Error()))
		}

		// 2. Static decoder resolve with failing declarer on sensitive field
		docStatic, err := nativeconfig.Parse(strings.NewReader(input))
		if err != nil {
			panic(fmt.Sprintf("static parse: %v", err))
		}
		regStatic := nativeconfig.NewRegistry()
		_ = regStatic.Register("fail_secret", []byte{}, func(ctx context.Context, r *nativeconfig.Registry, args []nativeconfig.Expr) (any, error) {
			return nil, fmt.Errorf("internal failure with secret content: %s", secretPayload)
		})
		_, errStatic := staticapp.Load(ctx, docStatic, staticapp.WithRegistry(regStatic))
		if errStatic == nil {
			panic("static expected error on failing sensitive declarer, got nil")
		}
		if strings.Contains(errStatic.Error(), secretPayload) {
			panic("static leaked secretPayload in error message!")
		}
		if !strings.Contains(errStatic.Error(), "service.credential") {
			panic(fmt.Sprintf("static error %q does not identify service.credential", errStatic.Error()))
		}
		if !strings.Contains(errStatic.Error(), "source resolution failed") {
			panic(fmt.Sprintf("static error %q does not satisfy documented redaction contract", errStatic.Error()))
		}
	}
}
`
	if err := os.WriteFile(filepath.Join(dir, "main.go"), []byte(harness), 0644); err != nil {
		t.Fatal(err)
	}

	cmd := exec.CommandContext(context.Background(), "go", "run", ".")
	cmd.Dir = dir
	if output, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("basic parity harness failed: %v\n%s", err, output)
	}
}

// TestHistoricalBaselineParity_Repeated verifies behavioral parity between the frozen
// historical implementation and the new static decoder on repeated section workloads.
func TestHistoricalBaselineParity_Repeated(t *testing.T) {
	histRepo := historicalRepoPath(t)

	// Matching schema: repeated worker section with required label
	schema := m.Schema{
		Version: 1,
		Sections: []m.SectionDefinition{
			{
				Name:     "worker",
				GoName:   "Workers",
				GoType:   "WorkerConfig",
				Repeated: true,
				Fields: []m.Field{
					{
						Key:      "label",
						GoName:   "Label",
						GoType:   "string",
						Required: true,
					},
				},
			},
		},
	}

	staticAppBytes, err := codegen.GenerateAppAPI(schema, codegen.Options{
		Package:       "staticapp",
		RuntimeImport: "example.test/diffrep/nativeconfig",
	})
	if err != nil {
		t.Fatalf("GenerateAppAPI: %v", err)
	}

	runtimeFiles, err := codegen.GenerateRuntime(codegen.RuntimeOptions{
		Package:      "nativeconfig",
		Capabilities: codegen.FullRuntimeCapabilities(),
	})
	if err != nil {
		t.Fatalf("GenerateRuntime: %v", err)
	}

	dir := t.TempDir()
	goMod := fmt.Sprintf("module example.test/diffrep\n\ngo 1.25.0\n\nrequire github.com/arran4/gogenconf v0.0.0\n\nreplace github.com/arran4/gogenconf => %s\n", histRepo)
	if err := os.WriteFile(filepath.Join(dir, "go.mod"), []byte(goMod), 0644); err != nil {
		t.Fatal(err)
	}

	if err := runtimeFiles.WriteToDir(filepath.Join(dir, "nativeconfig")); err != nil {
		t.Fatal(err)
	}
	staticDir := filepath.Join(dir, "staticapp")
	if err := os.MkdirAll(staticDir, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(staticDir, "config_generated.go"), staticAppBytes, 0644); err != nil {
		t.Fatal(err)
	}

	const harness = `package main

import (
	"context"
	"fmt"
	"strings"

	"github.com/arran4/gogenconf"
	hist_bind "github.com/arran4/gogenconf/examples/generated/repeated/binding"
	"example.test/diffrep/nativeconfig"
	"example.test/diffrep/staticapp"
)

func main() {
	ctx := context.Background()

	// Case 1: Multiple repeated workers parity
	{
		var sb strings.Builder
		sb.WriteString("config_version 1\n")
		for i := 0; i < 10; i++ {
			sb.WriteString(fmt.Sprintf("section worker worker-%02d\n  label \"worker-label-%02d\"\nend\n", i, i))
		}
		input := sb.String()

		docHist, err := gogenconf.Parse(strings.NewReader(input))
		if err != nil {
			panic(err)
		}
		regHist, _ := gogenconf.NewStandardRegistry()
		cfgHist, err := hist_bind.Resolve(ctx, docHist, regHist, nil)
		if err != nil {
			panic(err)
		}

		docStatic, err := nativeconfig.Parse(strings.NewReader(input))
		if err != nil {
			panic(err)
		}
		cfgStatic, err := staticapp.Load(ctx, docStatic)
		if err != nil {
			panic(err)
		}

		if len(cfgHist.Workers) != len(cfgStatic.Workers) {
			panic(fmt.Sprintf("worker count mismatch: hist=%d, static=%d", len(cfgHist.Workers), len(cfgStatic.Workers)))
		}
		if len(cfgStatic.Workers) != 10 {
			panic(fmt.Sprintf("expected 10 workers, got %d", len(cfgStatic.Workers)))
		}
		for i := range cfgHist.Workers {
			if cfgHist.Workers[i].Name != cfgStatic.Workers[i].Name {
				panic(fmt.Sprintf("worker[%d] name mismatch: hist=%q, static=%q", i, cfgHist.Workers[i].Name, cfgStatic.Workers[i].Name))
			}
			if cfgHist.Workers[i].Label != cfgStatic.Workers[i].Label {
				panic(fmt.Sprintf("worker[%d] label mismatch: hist=%q, static=%q", i, cfgHist.Workers[i].Label, cfgStatic.Workers[i].Label))
			}
		}
	}

	// Case 2: Missing required label error parity
	{
		input := "config_version 1\nsection worker worker-bad\nend\n"

		docHist, err := gogenconf.Parse(strings.NewReader(input))
		if err != nil {
			panic(err)
		}
		regHist, _ := gogenconf.NewStandardRegistry()
		_, errHist := hist_bind.Resolve(ctx, docHist, regHist, nil)
		if errHist == nil {
			panic("hist expected error on missing required worker label")
		}

		docStatic, err := nativeconfig.Parse(strings.NewReader(input))
		if err != nil {
			panic(err)
		}
		_, errStatic := staticapp.Load(ctx, docStatic)
		if errStatic == nil {
			panic("static expected error on missing required worker label")
		}

		if !strings.Contains(errHist.Error(), "label") {
			panic(fmt.Sprintf("hist error %q does not mention label", errHist.Error()))
		}
		if !strings.Contains(errStatic.Error(), "label") {
			panic(fmt.Sprintf("static error %q does not mention label", errStatic.Error()))
		}
	}
}
`
	if err := os.WriteFile(filepath.Join(dir, "main.go"), []byte(harness), 0644); err != nil {
		t.Fatal(err)
	}

	cmd := exec.CommandContext(context.Background(), "go", "run", ".")
	cmd.Dir = dir
	if output, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("repeated parity harness failed: %v\n%s", err, output)
	}
}

// TestHistoricalBaseline_IntentionalDivergences explicitly verifies:
// 1. Canonical brace container syntax supported in static decoder but not historical baseline.
// 2. Collection type support (lists and maps) introduced in #51/#52.
// 3. Standalone dependency-free runtime (zero dependencies on github.com/arran4/gogenconf).
func TestHistoricalBaseline_IntentionalDivergences(t *testing.T) {
	histRepo := historicalRepoPath(t)

	schema := m.Schema{
		Version: 2,
		Sections: []m.SectionDefinition{
			{
				Name:     "worker",
				GoName:   "Workers",
				GoType:   "WorkerConfig",
				Repeated: true,
				Fields: []m.Field{
					{Key: "label", GoName: "Label", GoType: "string", Required: true},
				},
			},
			{
				Name:   "service",
				GoName: "Service",
				GoType: "ServiceConfig",
				Fields: []m.Field{
					{Key: "tags", GoName: "Tags", GoType: "[]string"},
					{Key: "headers", GoName: "Headers", GoType: "map[string]string"},
				},
			},
		},
	}

	staticAppBytes, err := codegen.GenerateAppAPI(schema, codegen.Options{
		Package:       "staticapp",
		RuntimeImport: "example.test/diffdiv/nativeconfig",
	})
	if err != nil {
		t.Fatalf("GenerateAppAPI: %v", err)
	}

	runtimeFiles, err := codegen.GenerateRuntime(codegen.RuntimeOptions{
		Package:      "nativeconfig",
		Capabilities: codegen.FullRuntimeCapabilities(),
	})
	if err != nil {
		t.Fatalf("GenerateRuntime: %v", err)
	}

	dir := t.TempDir()
	goMod := fmt.Sprintf("module example.test/diffdiv\n\ngo 1.25.0\n\nrequire github.com/arran4/gogenconf v0.0.0\n\nreplace github.com/arran4/gogenconf => %s\n", histRepo)
	if err := os.WriteFile(filepath.Join(dir, "go.mod"), []byte(goMod), 0644); err != nil {
		t.Fatal(err)
	}

	if err := runtimeFiles.WriteToDir(filepath.Join(dir, "nativeconfig")); err != nil {
		t.Fatal(err)
	}
	staticDir := filepath.Join(dir, "staticapp")
	if err := os.MkdirAll(staticDir, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(staticDir, "config_generated.go"), staticAppBytes, 0644); err != nil {
		t.Fatal(err)
	}

	const harness = `package main

import (
	"context"
	"fmt"
	"strings"

	"github.com/arran4/gogenconf"
	hist_bind "github.com/arran4/gogenconf/examples/generated/repeated/binding"
	"example.test/diffdiv/nativeconfig"
	"example.test/diffdiv/staticapp"
)

func main() {
	ctx := context.Background()

	// 1. Canonical brace container syntax:
	// Historical baseline expects "section worker w1\n  label x\nend"
	// Canonical brace container is "worker w1 {\n  label x\n}"
	braceInput := "config_version 2\n\nworker w1 {\n  label \"canonical-brace-worker\"\n}\n"

	// Historical parser parses brace container into Document.Sections with structural container,
	// but historical generated binder (d000175) iterates d.Sections checking strings.HasPrefix(s.Name, "worker ")
	// which fails to find or bind repeated items authored in canonical brace container syntax.
	docHist, err := gogenconf.Parse(strings.NewReader(braceInput))
	if err == nil {
		regHist, _ := gogenconf.NewStandardRegistry()
		cfgHist, _ := hist_bind.Resolve(ctx, docHist, regHist, nil)
		if len(cfgHist.Workers) != 0 {
			panic(fmt.Sprintf("historical baseline unexpectedly bound canonical brace container: %d workers", len(cfgHist.Workers)))
		}
	}

	// New static decoder seamlessly decodes canonical brace containers into typed fields
	docStatic, err := nativeconfig.Parse(strings.NewReader(braceInput))
	if err != nil {
		panic(fmt.Sprintf("static parse failed on canonical brace container: %v", err))
	}
	cfgStatic, err := staticapp.Load(ctx, docStatic)
	if err != nil {
		panic(fmt.Sprintf("static load failed on canonical brace container: %v", err))
	}
	if len(cfgStatic.Workers) != 1 || cfgStatic.Workers[0].Name != "w1" || cfgStatic.Workers[0].Label != "canonical-brace-worker" {
		panic(fmt.Sprintf("static decoder failed to decode canonical brace worker: %+v", cfgStatic.Workers))
	}

	// 2. Collections (lists & maps)
	collInput := "config_version 2\n\nservice {\n  tags [ \"web\", \"primary\" ]\n  headers { Authorization: \"bearer_token\", X-Env: \"production\" }\n}\nworker w1 {\n  label \"primary\"\n}\n"
	docColl, err := nativeconfig.Parse(strings.NewReader(collInput))
	if err != nil {
		panic(fmt.Sprintf("static parse collections: %v", err))
	}
	cfgColl, err := staticapp.Load(ctx, docColl)
	if err != nil {
		panic(fmt.Sprintf("static load collections: %v", err))
	}
	if len(cfgColl.Service.Tags) != 2 || cfgColl.Service.Tags[0] != "web" || cfgColl.Service.Tags[1] != "primary" {
		panic(fmt.Sprintf("tags mismatch: %v", cfgColl.Service.Tags))
	}
	if len(cfgColl.Service.Headers) != 2 || cfgColl.Service.Headers["Authorization"] != "bearer_token" {
		panic(fmt.Sprintf("headers mismatch: %v", cfgColl.Service.Headers))
	}
}
`
	if err := os.WriteFile(filepath.Join(dir, "main.go"), []byte(harness), 0644); err != nil {
		t.Fatal(err)
	}

	cmd := exec.CommandContext(context.Background(), "go", "run", ".")
	cmd.Dir = dir
	if output, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("intentional divergences harness failed: %v\n%s", err, output)
	}

	// 3. Dependency independence verification:
	// Verify that the static consumer module has zero dependencies on github.com/arran4/gogenconf
	depsCmd := exec.CommandContext(context.Background(), "go", "list", "-deps", "./staticapp")
	depsCmd.Dir = dir
	depsOut, err := depsCmd.CombinedOutput()
	if err != nil {
		t.Fatalf("go list -deps failed: %v\n%s", err, depsOut)
	}
	for _, line := range strings.Split(string(depsOut), "\n") {
		line = strings.TrimSpace(line)
		if strings.Contains(line, "github.com/arran4/gogenconf") {
			t.Fatalf("staticapp must not depend on github.com/arran4/gogenconf, found dependency: %s", line)
		}
	}
}

// TestCurrentPathHistoricalBinderParity verifies that the current codegen's GenerateBinder
// and GenerateAppAPI produce identical bound struct fields across scalars, lists, maps,
// and repeated sections.
func TestCurrentPathHistoricalBinderParity(t *testing.T) {
	schema := m.Schema{
		Version: 1,
		Sections: []m.SectionDefinition{
			{
				Name:   "service",
				GoName: "Service",
				GoType: "ServiceConfig",
				Fields: []m.Field{
					{Key: "endpoint", GoName: "Endpoint", GoType: "string", Default: m.Literal{Value: "http://localhost:8080"}},
					{Key: "retries", GoName: "Retries", GoType: "int", Default: m.Literal{Value: "3"}},
					{Key: "tags", GoName: "Tags", GoType: "[]string"},
					{Key: "headers", GoName: "Headers", GoType: "map[string]string"},
				},
			},
			{
				Name:     "worker",
				GoName:   "Workers",
				GoType:   "WorkerConfig",
				Repeated: true,
				Fields: []m.Field{
					{Key: "concurrency", GoName: "Concurrency", GoType: "int", Default: m.Literal{Value: "4"}},
				},
			},
		},
	}

	// 1. Generate model and historical binder (gogenconf runtime import)
	modelBytes, err := codegen.GenerateModel(schema, codegen.Options{Package: "appconfig"})
	if err != nil {
		t.Fatalf("GenerateModel: %v", err)
	}
	binderBytes, err := codegen.GenerateBinder(schema, codegen.Options{
		Package:       "binding",
		RuntimeImport: "github.com/arran4/gogenconf",
		ConfigImport:  "example.test/diff/appconfig",
	})
	if err != nil {
		t.Fatalf("GenerateBinder: %v", err)
	}

	// 2. Generate static app API (standalone nativeconfig runtime)
	staticAppBytes, err := codegen.GenerateAppAPI(schema, codegen.Options{
		Package:       "staticapp",
		RuntimeImport: "example.test/diff/nativeconfig",
	})
	if err != nil {
		t.Fatalf("GenerateAppAPI: %v", err)
	}

	runtimeFiles, err := codegen.GenerateRuntime(codegen.RuntimeOptions{
		Package:      "nativeconfig",
		Capabilities: codegen.FullRuntimeCapabilities(),
	})
	if err != nil {
		t.Fatalf("GenerateRuntime: %v", err)
	}

	dir := t.TempDir()
	// Create go.mod pointing to local workspace root for the current binder
	repoRoot, err := filepath.Abs("..")
	if err != nil {
		t.Fatal(err)
	}
	goMod := "module example.test/diff\n\ngo 1.25.0\n\nrequire github.com/arran4/gogenconf v0.0.0\n\nreplace github.com/arran4/gogenconf => " + repoRoot + "\n"
	if err := os.WriteFile(filepath.Join(dir, "go.mod"), []byte(goMod), 0644); err != nil {
		t.Fatal(err)
	}

	// Write historical model and binder
	appDir := filepath.Join(dir, "appconfig")
	if err := os.MkdirAll(appDir, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(appDir, "config_generated.go"), modelBytes, 0644); err != nil {
		t.Fatal(err)
	}

	bindingDir := filepath.Join(dir, "binding")
	if err := os.MkdirAll(bindingDir, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(bindingDir, "binding_generated.go"), binderBytes, 0644); err != nil {
		t.Fatal(err)
	}

	// Write static app API and native runtime
	if err := runtimeFiles.WriteToDir(filepath.Join(dir, "nativeconfig")); err != nil {
		t.Fatal(err)
	}

	staticDir := filepath.Join(dir, "staticapp")
	if err := os.MkdirAll(staticDir, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(staticDir, "config_generated.go"), staticAppBytes, 0644); err != nil {
		t.Fatal(err)
	}

	const harness = `package main

import (
	"context"
	"fmt"
	"reflect"
	"strings"

	"github.com/arran4/gogenconf"
	"example.test/diff/binding"
	"example.test/diff/nativeconfig"
	"example.test/diff/staticapp"
)

func main() {
	input := "config_version 1\n" +
		"section service\n" +
		"    endpoint https://api.prod.example.com\n" +
		"    retries 5\n" +
		"    tags [ web, primary ]\n" +
		"    headers { Authorization: bearer_token, X-Tier: gold }\n" +
		"end\n" +
		"section worker w1\n" +
		"    concurrency 16\n" +
		"end\n" +
		"section worker w2\n" +
		"    concurrency 32\n" +
		"end\n"

	// 1. Resolve with historical binder
	docHist, err := gogenconf.Parse(strings.NewReader(input))
	if err != nil {
		panic(err)
	}
	regHist, err := gogenconf.NewStandardRegistry()
	if err != nil {
		panic(err)
	}
	cfgHist, err := binding.Resolve(context.Background(), docHist, regHist, nil)
	if err != nil {
		panic(fmt.Sprintf("historical resolve failed: %v", err))
	}

	// 2. Resolve with static decoder
	docStatic, err := nativeconfig.Parse(strings.NewReader(input))
	if err != nil {
		panic(err)
	}
	cfgStatic, err := staticapp.Load(context.Background(), docStatic)
	if err != nil {
		panic(fmt.Sprintf("static load failed: %v", err))
	}

	// 3. Differential assertions: verify all fields match
	if cfgHist.Service.Endpoint != cfgStatic.Service.Endpoint {
		panic(fmt.Sprintf("Endpoint mismatch: hist=%q, static=%q", cfgHist.Service.Endpoint, cfgStatic.Service.Endpoint))
	}
	if cfgHist.Service.Retries != cfgStatic.Service.Retries {
		panic(fmt.Sprintf("Retries mismatch: hist=%d, static=%d", cfgHist.Service.Retries, cfgStatic.Service.Retries))
	}
	if !reflect.DeepEqual(cfgHist.Service.Tags, cfgStatic.Service.Tags) {
		panic(fmt.Sprintf("Tags mismatch: hist=%v, static=%v", cfgHist.Service.Tags, cfgStatic.Service.Tags))
	}
	if !reflect.DeepEqual(cfgHist.Service.Headers, cfgStatic.Service.Headers) {
		panic(fmt.Sprintf("Headers mismatch: hist=%v, static=%v", cfgHist.Service.Headers, cfgStatic.Service.Headers))
	}
	if len(cfgHist.Workers) != len(cfgStatic.Workers) {
		panic(fmt.Sprintf("Workers count mismatch: hist=%d, static=%d", len(cfgHist.Workers), len(cfgStatic.Workers)))
	}
	for i := range cfgHist.Workers {
		if cfgHist.Workers[i].Name != cfgStatic.Workers[i].Name {
			panic(fmt.Sprintf("Worker[%d].Name mismatch: hist=%q, static=%q", i, cfgHist.Workers[i].Name, cfgStatic.Workers[i].Name))
		}
		if cfgHist.Workers[i].Concurrency != cfgStatic.Workers[i].Concurrency {
			panic(fmt.Sprintf("Worker[%d].Concurrency mismatch: hist=%d, static=%d", i, cfgHist.Workers[i].Concurrency, cfgStatic.Workers[i].Concurrency))
		}
	}
}
`
	if err := os.WriteFile(filepath.Join(dir, "main.go"), []byte(harness), 0644); err != nil {
		t.Fatal(err)
	}

	cmd := exec.CommandContext(context.Background(), "go", "run", ".")
	cmd.Dir = dir
	if output, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("differential harness execution failed: %v\n%s", err, output)
	}
}

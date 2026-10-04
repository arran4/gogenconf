package integration

import (
	"embed"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/arran4/gogenconf/codegen"
	"github.com/arran4/gogenconf/internal/testutil"
)

//go:embed testdata/runtime-consumer.txtar
var runtimeConsumerFixture embed.FS

func TestRuntimeConsumerHasNoGogenconfDependency(t *testing.T) {
	// 1. Generate runtime source using the public codegen API
	opts := codegen.RuntimeOptions{
		Package: "myconfig",
	}
	files, err := codegen.GenerateRuntime(opts)
	if err != nil {
		t.Fatalf("GenerateRuntime failed: %v", err)
	}

	// 2. Inspect generated files: none must import gogenconf
	for _, f := range files {
		if strings.Contains(string(f.Content), "github.com/arran4/gogenconf") {
			t.Fatalf("generated file %s imports github.com/arran4/gogenconf", f.Name)
		}
	}

	dir := t.TempDir()
	fixture, err := testutil.LoadCases(runtimeConsumerFixture, "testdata")
	if err != nil || len(fixture) != 1 {
		t.Fatalf("runtime fixture = %v, %v", fixture, err)
	}
	input, err := testutil.Tree(fixture[0].Archive, "input")
	if err != nil {
		t.Fatal(err)
	}
	for name, file := range input {
		path := filepath.Join(dir, filepath.FromSlash(name))
		if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, file.Data, 0644); err != nil {
			t.Fatal(err)
		}
	}

	// 3. The fixture's go.mod has NO require or replace for gogenconf.
	configPkgDir := filepath.Join(dir, "myconfig")
	if err := files.WriteToDir(configPkgDir); err != nil {
		t.Fatalf("failed to write generated runtime: %v", err)
	}

	// 4. Verify dependencies using go list -deps ./...
	// gogenconf MUST NOT appear in the runtime build/dependency graph.
	deps := run(t, dir, []string{"GOWORK=off"}, "", "go", "list", "-deps", "./...")
	if strings.Contains(deps, "github.com/arran4/gogenconf") {
		t.Fatalf("go list -deps contains github.com/arran4/gogenconf:\n%s", deps)
	}

	// 5. Build the consumer application
	run(t, dir, []string{"GOWORK=off"}, "", "go", "build", "./...")

	// 6. Run the consumer application and verify output
	out := run(t, dir, []string{"GOWORK=off"}, "", "go", "run", ".")
	expected, err := testutil.Tree(fixture[0].Archive, "expected")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out, string(expected["stdout.txt"].Data)) {
		t.Fatalf("unexpected consumer output: %s", out)
	}
}

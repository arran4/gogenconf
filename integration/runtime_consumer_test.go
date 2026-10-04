package integration

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/arran4/gogenconf/codegen"
)

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

	// 3. Create a temporary external module with NO gogenconf dependency in go.mod
	dir := t.TempDir()
	configPkgDir := filepath.Join(dir, "myconfig")
	if err := files.WriteToDir(configPkgDir); err != nil {
		t.Fatalf("failed to write generated runtime: %v", err)
	}

	// go.mod has NO require or replace for github.com/arran4/gogenconf
	goMod := "module example.test/runtimeconsumer\n\ngo 1.25.0\n"
	if err := os.WriteFile(filepath.Join(dir, "go.mod"), []byte(goMod), 0644); err != nil {
		t.Fatal(err)
	}

	// 4. Create an application main.go that uses the generated package
	consumerMain := `package main

import (
	"fmt"
	"strings"

	"example.test/runtimeconsumer/myconfig"
)

func main() {
	input := "# Config file\nconfig_version 1\nsection server\n    port 8080\n    host from_env(HOST)\nend\n"
	doc, err := myconfig.Parse(strings.NewReader(input))
	if err != nil {
		panic(err)
	}
	if doc.Version != 1 {
		panic("expected version 1")
	}
	sec := doc.Section("server")
	if sec == nil {
		panic("missing section server")
	}
	entry, _, ok := sec.Entry("port")
	if !ok {
		panic("missing port")
	}
	lit, ok := entry.Value.(myconfig.Literal)
	if !ok || lit.Value != "8080" {
		panic("invalid port literal")
	}

	// Test cloning and mutating
	cloned := doc.Clone()
	cloned.Section("server").Set("port", myconfig.Literal{Value: "9090"})
	if doc.Value("server", "port").(myconfig.Literal).Value != "8080" {
		panic("mutation of clone affected original")
	}
	if cloned.Value("server", "port").(myconfig.Literal).Value != "9090" {
		panic("clone mutation failed")
	}

	formatted := myconfig.Format(doc)
	if !strings.Contains(formatted, "port 8080") {
		panic("formatted output missing port")
	}
	fmt.Println("runtime consumer succeeded; port:", lit.Value)
}
`
	if err := os.WriteFile(filepath.Join(dir, "main.go"), []byte(consumerMain), 0644); err != nil {
		t.Fatal(err)
	}

	// 5. Verify dependencies using go list -deps ./...
	// gogenconf MUST NOT appear in the runtime build/dependency graph.
	deps := run(t, dir, []string{"GOWORK=off"}, "", "go", "list", "-deps", "./...")
	if strings.Contains(deps, "github.com/arran4/gogenconf") {
		t.Fatalf("go list -deps contains github.com/arran4/gogenconf:\n%s", deps)
	}

	// 6. Build the consumer application
	run(t, dir, []string{"GOWORK=off"}, "", "go", "build", "./...")

	// 7. Run the consumer application and verify output
	out := run(t, dir, []string{"GOWORK=off"}, "", "go", "run", ".")
	if !strings.Contains(out, "runtime consumer succeeded; port: 8080") {
		t.Fatalf("unexpected consumer output: %s", out)
	}
}

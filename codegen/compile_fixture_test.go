package codegen_test

import (
	"embed"
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"github.com/arran4/gogenconf/codegen"
	"github.com/arran4/gogenconf/codegen/internal/testschema"
	"github.com/arran4/gogenconf/internal/testutil"
)

//go:embed testdata/compile/*.txtar
var compileCases embed.FS

func TestGeneratedBinderCompileFixtures(t *testing.T) {
	cases, err := testutil.LoadCases(compileCases, "testdata/compile")
	if err != nil {
		t.Fatal(err)
	}
	for _, tc := range cases {
		t.Run(filepath.Base(tc.Path), func(t *testing.T) {
			input, err := testutil.Tree(tc.Archive, "input")
			if err != nil {
				t.Fatal(err)
			}
			dir := t.TempDir()
			for name, file := range input {
				path := filepath.Join(dir, filepath.FromSlash(name))
				if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
					t.Fatal(err)
				}
				if err := os.WriteFile(path, file.Data, 0644); err != nil {
					t.Fatal(err)
				}
			}
			root, err := filepath.Abs("..")
			if err != nil {
				t.Fatal(err)
			}
			mod := "module example.test/generatedfixture\n\ngo 1.25.0\n\nrequire github.com/arran4/gogenconf v0.0.0\nreplace github.com/arran4/gogenconf => " + filepath.ToSlash(root) + "\n"
			if err := os.WriteFile(filepath.Join(dir, "go.mod"), []byte(mod), 0644); err != nil {
				t.Fatal(err)
			}
			generated, err := codegen.Generate(testschema.Definition(), codegen.Options{Package: "fixture"})
			if err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(filepath.Join(dir, "config_generated.go"), generated, 0644); err != nil {
				t.Fatal(err)
			}
			for _, args := range [][]string{{"test", "./..."}, {"build", "./..."}} {
				cmd := exec.Command("go", args...)
				cmd.Dir = dir
				cmd.Env = append(os.Environ(), "GOWORK=off")
				if output, err := cmd.CombinedOutput(); err != nil {
					t.Fatalf("go %v: %v\n%s", args, err, output)
				}
			}
		})
	}
}

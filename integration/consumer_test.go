package integration

import (
	"bytes"
	"compress/gzip"
	"io"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
	"testing"
)

func root(t *testing.T) string {
	t.Helper()
	p, err := filepath.Abs("..")
	if err != nil {
		t.Fatal(err)
	}
	return p
}
func run(t *testing.T, dir string, env []string, input string, args ...string) string {
	t.Helper()
	c := exec.Command(args[0], args[1:]...)
	c.Dir = dir
	c.Env = append(os.Environ(), env...)
	c.Stdin = strings.NewReader(input)
	out, err := c.CombinedOutput()
	if err != nil {
		t.Fatalf("%v: %v\n%s", args, err, out)
	}
	return string(out)
}

func TestOutsideConsumer(t *testing.T) {
	src := filepath.Join(root(t), "examples/generated/basic")
	dir := t.TempDir()
	err := filepath.WalkDir(src, func(path string, e fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		rel, err := filepath.Rel(src, path)
		if err != nil {
			return err
		}
		if e.IsDir() {
			return os.MkdirAll(filepath.Join(dir, rel), 0755)
		}
		if strings.HasSuffix(path, "_generated.go") {
			return nil
		} // must generate, not borrow output
		b, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		b = bytes.ReplaceAll(b, []byte("github.com/arran4/gogenconf/examples/generated/basic"), []byte("example.test/consumer"))
		return os.WriteFile(filepath.Join(dir, rel), b, 0644)
	})
	if err != nil {
		t.Fatal(err)
	}
	// The replace is confined to a disposable consumer so CI tests this candidate,
	// never a network/cache version. Neither repository commits a replace.
	mod := "module example.test/consumer\n\ngo 1.25.0\n\nrequire github.com/arran4/gogenconf v0.0.0\nreplace github.com/arran4/gogenconf => " + filepath.ToSlash(root(t)) + "\n"
	if err := os.WriteFile(filepath.Join(dir, "go.mod"), []byte(mod), 0644); err != nil {
		t.Fatal(err)
	}
	run(t, dir, []string{"GOWORK=off"}, "", "go", "generate", "./...")
	run(t, dir, []string{"GOWORK=off"}, "", "go", "build", "./...")
	out := run(t, dir, []string{"GOWORK=off"}, "", "go", "run", ".")
	if !strings.Contains(out, "typed endpoint: https://example.test; credential bytes: 10") {
		t.Fatal(out)
	}
}

func TestInstalledCLIAndManual(t *testing.T) {
	dir := root(t)
	run(t, dir, nil, "", "go", "run", "./internal/cligen")
	binDir := t.TempDir()
	run(t, dir, []string{"GOBIN=" + binDir}, "", "go", "install", "./cmd/gogenconf")
	binary := filepath.Join(binDir, "gogenconf")
	if runtime.GOOS == "windows" {
		binary += ".exe"
	}
	help := run(t, dir, nil, "", binary, "--help")
	if !strings.Contains(help, "Application schemas live in Go") {
		t.Fatal(help)
	}
	v := run(t, dir, nil, "", binary, "version")
	if !strings.Contains(v, "Version: dev") || !strings.Contains(v, "Commit: none") {
		t.Fatal(v)
	}
	versioned := filepath.Join(binDir, "versioned")
	if runtime.GOOS == "windows" {
		versioned += ".exe"
	}
	run(t, dir, nil, "", "go", "build", "-o", versioned, "-ldflags=-X main.version=v0.0.1-test.1 -X main.commit=fixture -X main.date=2026-01-01", "./cmd/gogenconf")
	injected := run(t, dir, nil, "", versioned, "version")
	for _, want := range []string{"v0.0.1-test.1", "fixture", "2026-01-01"} {
		if !strings.Contains(injected, want) {
			t.Fatal("version injection missing", want)
		}
	}
	input := "config_version 1\nsection service\n value from_file(/never/read/this)\nend\n"
	run(t, dir, nil, input, binary, "validate")
	canonical := run(t, dir, nil, input, binary, "format")
	run(t, dir, nil, canonical, binary, "format", "--check")
	versionedDocument := "# operator note\n## managed documentation\nconfig_version 1\nsection service\n ## field documentation\n endpoint from_json_file(from_env(CONFIG), \".nested.key\")\nend\n"
	wantVersioned := "# operator note\n## managed documentation\nconfig_version 1\nsection service\n    ## field documentation\n    endpoint from_json_file(from_env(CONFIG), .nested.key)\nend\n"
	if got := run(t, dir, nil, versionedDocument, binary, "format"); got != wantVersioned {
		t.Fatalf("versioned format mismatch:\n%s", got)
	}
	run(t, dir, nil, versionedDocument, binary, "validate")
	legacy := "### legacy heading\nconfig_version 0\nsection worker worker-1\n    label \"two words\"\nend\n"
	if got := run(t, dir, nil, legacy, binary, "format"); got != legacy {
		t.Fatalf("legacy comment ownership changed:\n%s", got)
	}
	run(t, dir, nil, legacy, binary, "validate")
	file := filepath.Join(t.TempDir(), "service.conf")
	if err := os.WriteFile(file, []byte(input), 0600); err != nil {
		t.Fatal(err)
	}
	run(t, dir, nil, "", binary, "format", "--write", file)
	info, err := os.Stat(file)
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode().Perm() != 0600 {
		t.Fatal("write changed permissions")
	}
	for _, args := range [][]string{{"format", "--write", "--check", file}, {"format", "--check", "-"}, {"expr", "validate", "from_file("}, {"format", "--write"}, {"validate", "-"}} {
		c := exec.Command(binary, args...)
		badInput := input
		if args[0] == "validate" {
			badInput = "section service\n    endpoint value\n"
		}
		c.Stdin = strings.NewReader(badInput)
		if out, err := c.CombinedOutput(); err == nil {
			t.Fatalf("accepted invalid command %v: %s", args, out)
		}
	}
	got := run(t, dir, nil, "", binary, "expr", "format", "from_json_file( from_env(CONFIG), \".nested key\" )")
	if strings.TrimSpace(got) != "from_json_file(from_env(CONFIG), \".nested key\")" {
		t.Fatal(got)
	}
	run(t, dir, nil, "", binary, "expr", "validate", "from_json_file(from_env(CONFIG), .nested.key)")
	for _, command := range [][]string{{"format", "--help"}, {"validate", "--help"}, {"expr", "format", "--help"}} {
		if !strings.Contains(run(t, dir, nil, "", append([]string{binary}, command...)...), "Example") {
			t.Fatal("missing extended help")
		}
	}
	f, err := os.Open(filepath.Join(dir, "man/man1/gogenconf.1.gz"))
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	z, err := gzip.NewReader(f)
	if err != nil {
		t.Fatal(err)
	}
	defer z.Close()
	b, err := io.ReadAll(z)
	if err != nil {
		t.Fatal(err)
	}
	for _, s := range []string{".TH GOGENCONF 1", "SYNOPSIS", "DESCRIPTION", "--check", "from_file", "github.com/arran4/gogenconf"} {
		if !strings.Contains(string(b), s) {
			t.Fatalf("manual missing %q", s)
		}
	}
}

func TestRunnableExamples(t *testing.T) {
	base := filepath.Join(root(t), "examples")
	var examples []string
	err := filepath.WalkDir(base, func(path string, entry fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if entry.IsDir() {
			return nil
		}
		if entry.Name() != "main.go" {
			return nil
		}
		rel, err := filepath.Rel(base, filepath.Dir(path))
		if err != nil {
			return err
		}
		if strings.Contains(rel, "internal"+string(filepath.Separator)) || rel == "internal" {
			return nil
		}
		examples = append(examples, filepath.ToSlash(rel))
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	sort.Strings(examples)
	if len(examples) == 0 {
		t.Fatal("no runnable examples discovered")
	}
	for _, name := range examples {
		name := name
		t.Run(name, func(t *testing.T) { run(t, root(t), nil, "", "go", "run", "./examples/"+name) })
	}
}

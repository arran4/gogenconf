// cligen pins CLI/help/man generation without adding runtime dependencies.
package main

import (
	"bytes"
	"compress/gzip"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

func main() {
	if err := run(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
func run() error {
	root, err := os.Getwd()
	if err != nil {
		return err
	}
	for {
		if _, err := os.Stat(filepath.Join(root, "go.mod")); err == nil {
			break
		}
		parent := filepath.Dir(root)
		if parent == root {
			return fmt.Errorf("module root not found")
		}
		root = parent
	}
	c := exec.Command("go", "run", "github.com/arran4/go-subcommand/cmd/gosubc@v0.0.30", "generate", "--dir", root, "--path", "internal/cli", "--man-dir", "internal/cligen/man", "--timestamp=false", "--project-provenance=false", "--replace-template", "cmd/main.go.gotmpl=internal/cligen/main.go.gotmpl")
	c.Dir = root
	c.Stdout = os.Stdout
	c.Stderr = os.Stderr
	if err := c.Run(); err != nil {
		return err
	}
	c = exec.Command("go", "run", "./cmd/gogenconf", "--help")
	c.Dir = root
	help, err := c.CombinedOutput()
	if err != nil {
		return fmt.Errorf("generated help: %w: %s", err, help)
	}
	var man strings.Builder
	man.WriteString(".\\\" Generated from go-subcommand help/man output. DO NOT EDIT.\n.TH GOGENCONF 1 \"\" \"gogenconf\" \"User Commands\"\n.SH NAME\ngogenconf \\- versioned declarative configuration tooling\n.SH SYNOPSIS\n.B gogenconf\n[command] [options]\n.SH DESCRIPTION\n.nf\n")
	man.Write(help)
	man.WriteString(".fi\n")
	files, err := filepath.Glob(filepath.Join(root, "internal/cligen/man/gogenconf-*.1"))
	if err != nil {
		return err
	}
	for _, file := range files {
		b, err := os.ReadFile(file)
		if err != nil {
			return err
		}
		// Normalize upstream roff whitespace as part of deterministic generation.
		lines := strings.Split(string(b), "\n")
		for i := range lines {
			lines[i] = strings.TrimRight(lines[i], " \t")
		}
		if err := os.WriteFile(file, []byte(strings.Join(lines, "\n")), 0644); err != nil {
			return err
		}
		for _, line := range lines {
			if strings.HasPrefix(line, ".TH ") || strings.HasPrefix(line, ".\\\"") {
				continue
			}
			man.WriteString(line + "\n")
		}
	}
	var packed bytes.Buffer
	z := gzip.NewWriter(&packed)
	if _, err := z.Write([]byte(man.String())); err != nil {
		return err
	}
	if err := z.Close(); err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Join(root, "man/man1"), 0755); err != nil {
		return err
	}
	return os.WriteFile(filepath.Join(root, "man/man1/gogenconf.1.gz"), packed.Bytes(), 0644)
}

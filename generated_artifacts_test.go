package gogenconf_test

import (
	"bufio"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestGeneratedGoArtifactLocations(t *testing.T) {
	err := filepath.WalkDir(".", func(name string, entry fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if entry.IsDir() {
			if entry.Name() == ".git" || entry.Name() == "testdata" {
				return filepath.SkipDir
			}
			return nil
		}
		if !strings.HasSuffix(name, ".go") {
			return nil
		}
		file, err := os.Open(name)
		if err != nil {
			return err
		}
		defer file.Close()
		line := ""
		if scanner := bufio.NewScanner(file); scanner.Scan() {
			line = scanner.Text()
		}
		if !strings.Contains(line, "Code generated") {
			return nil
		}
		rel := filepath.ToSlash(strings.TrimPrefix(name, "./"))
		if strings.HasPrefix(rel, "cmd/") || strings.HasPrefix(rel, "internal/nativeconfig/") || strings.HasPrefix(rel, "examples/generated/") {
			return nil
		}
		t.Errorf("generated Go artifact outside an owned class: %s", rel)
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
}

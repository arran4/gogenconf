// Package testutil provides test-only txtar fixture helpers.
package testutil

import (
	"bytes"
	"fmt"
	"io/fs"
	"path"
	"sort"
	"strings"
	"testing"
	"testing/fstest"

	"golang.org/x/tools/txtar"
)

// Case is one embedded txtar scenario.
type Case struct {
	Path    string
	Archive *txtar.Archive
}

// LoadCases discovers archives deterministically and returns one case per file.
func LoadCases(fsys fs.FS, root string) ([]Case, error) {
	var paths []string
	err := fs.WalkDir(fsys, root, func(name string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if !d.IsDir() && strings.HasSuffix(name, ".txtar") {
			paths = append(paths, name)
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	sort.Strings(paths)
	cases := make([]Case, 0, len(paths))
	for _, name := range paths {
		data, err := fs.ReadFile(fsys, name)
		if err != nil {
			return nil, err
		}
		cases = append(cases, Case{Path: name, Archive: txtar.Parse(data)})
	}
	return cases, nil
}

// Tree projects a named txtar subtree into an ordinary in-memory filesystem.
func Tree(ar *txtar.Archive, prefix string) (fstest.MapFS, error) {
	prefix += "/"
	tree := fstest.MapFS{}
	for _, file := range ar.Files {
		if !strings.HasPrefix(file.Name, prefix) {
			continue
		}
		name := strings.TrimPrefix(file.Name, prefix)
		if !fs.ValidPath(name) {
			return nil, fmt.Errorf("invalid %s path %q", strings.TrimSuffix(prefix, "/"), file.Name)
		}
		if _, exists := tree[name]; exists {
			return nil, fmt.Errorf("duplicate %s path %q", strings.TrimSuffix(prefix, "/"), name)
		}
		tree[name] = &fstest.MapFile{Data: file.Data}
	}
	return tree, nil
}

// CompareTree reports exact path and content differences with per-file context.
func CompareTree(t testing.TB, want fstest.MapFS, got map[string][]byte) {
	t.Helper()
	paths := map[string]bool{}
	for name := range want {
		paths[name] = true
	}
	for name := range got {
		paths[name] = true
	}
	var sorted []string
	for name := range paths {
		sorted = append(sorted, name)
	}
	sort.Strings(sorted)
	for _, name := range sorted {
		wantFile, wantOK := want[name]
		gotData, gotOK := got[name]
		switch {
		case !wantOK:
			t.Errorf("unexpected generated file %s", name)
		case !gotOK:
			t.Errorf("missing generated file %s", name)
		case !bytes.Equal(wantFile.Data, gotData):
			t.Errorf("generated file differs: %s\nwant:\n%s\ngot:\n%s", name, wantFile.Data, gotData)
		}
	}
}

// Files returns a stable ordinary-file tree from fsys.
func Files(fsys fs.FS) (map[string][]byte, error) {
	result := map[string][]byte{}
	err := fs.WalkDir(fsys, ".", func(name string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			return nil
		}
		data, err := fs.ReadFile(fsys, name)
		if err != nil {
			return err
		}
		result[path.Clean(name)] = data
		return nil
	})
	return result, err
}

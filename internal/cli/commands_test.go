package cli

import (
	"go/parser"
	"go/token"
	"strconv"
	"testing"
)

func TestSyntaxCommandsUseGeneratedRuntime(t *testing.T) {
	f, err := parser.ParseFile(token.NewFileSet(), "commands.go", nil, parser.ImportsOnly)
	if err != nil {
		t.Fatal(err)
	}

	imports := map[string]bool{}
	for _, spec := range f.Imports {
		path, err := strconv.Unquote(spec.Path.Value)
		if err != nil {
			t.Fatal(err)
		}
		imports[path] = true
	}

	if imports["github.com/arran4/gogenconf"] {
		t.Fatal("syntax commands must not import the canonical root runtime")
	}
	if !imports["github.com/arran4/gogenconf/internal/nativeconfig"] {
		t.Fatal("syntax commands must import the generated native runtime")
	}
}

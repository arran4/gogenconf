package codegen_test

import (
	"bytes"
	"strings"
	"testing"

	m "github.com/arran4/gogenconf"
	"github.com/arran4/gogenconf/codegen"
	"github.com/arran4/gogenconf/codegen/internal/testschema"
)

func TestGenerationIsDeterministic(t *testing.T) {
	s := testschema.Definition()
	options := codegen.Options{Package: "testfixture", LibraryImport: "github.com/arran4/gogenconf"}
	first, err := codegen.Generate(s, options)
	if err != nil {
		t.Fatal(err)
	}
	second, err := codegen.Generate(s, options)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(first, second) {
		t.Fatal("generated output is nondeterministic")
	}
	for _, fragment := range []string{"// Code generated", "DO NOT EDIT.", "Key     []byte", "gogenconf.Provider[[]byte]", "ResolveField[[]byte]", `"service", "key"`} {
		if !strings.Contains(string(first), fragment) {
			t.Errorf("missing %q", fragment)
		}
	}
	if strings.Contains(string(first), "reflect.") {
		t.Fatal("runtime reflection in generated binding")
	}
	for _, forbidden := range []string{"legacy_location", "Snapshot(", "SetLiteral("} {
		if strings.Contains(string(first), forbidden) {
			t.Fatalf("unexpected generated runtime surface %s", forbidden)
		}
	}
}

func TestSeparateModelAndBinder(t *testing.T) {
	s := testschema.Definition()
	model, err := codegen.GenerateModel(s, codegen.Options{Package: "appconfig", LibraryImport: "example/model"})
	if err != nil {
		t.Fatal(err)
	}
	binder, err := codegen.GenerateBinder(s, codegen.Options{Package: "binding", LibraryImport: "example/model", ConfigImport: "example/appconfig"})
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"type Config struct", "gogenconf.Provider[[]byte]", "Key     []byte"} {
		if !strings.Contains(string(model), want) {
			t.Fatalf("model missing %s", want)
		}
	}
	for _, bad := range []string{"Resolve(", "reflect.", "type Config =", "legacy_location"} {
		if strings.Contains(string(model), bad) {
			t.Fatalf("bad model surface %s", bad)
		}
	}
	for _, want := range []string{"runtimeconfig.Config", "ResolveField[[]byte]", "FieldProvider[[]byte]", `"service", "content"`} {
		if !strings.Contains(string(binder), want) {
			t.Fatalf("binder missing %s", want)
		}
	}
	for _, bad := range []string{"type Config", "legacy_location", "Snapshot("} {
		if strings.Contains(string(binder), bad) {
			t.Fatalf("bad binder surface %s", bad)
		}
	}
	if _, err := codegen.GenerateBinder(s, codegen.Options{Package: "binding", LibraryImport: "model"}); err == nil {
		t.Fatal("binder accepted missing runtime import")
	}
}

func TestInvalidMappingsFailBeforeOutput(t *testing.T) {
	cases := []struct {
		name   string
		mutate func(*m.Schema)
	}{
		{"type", func(s *m.Schema) { s.Sections[0].Fields[0].GoType = "complex128" }},
		{"invalid type", func(s *m.Schema) { s.Sections[0].Fields[0].GoType = "not a type" }},
		{"duplicate path", func(s *m.Schema) { s.Sections[0].Fields[1].Key = "label" }},
		{"duplicate field", func(s *m.Schema) { s.Sections[0].Fields[1].GoName = "Label" }},
		{"duplicate section", func(s *m.Schema) { s.Sections = append(s.Sections, s.Sections[0]) }},
		{"policy", func(s *m.Schema) { s.Sections[0].Fields[1].Policy = "cache" }},
		{"unexported", func(s *m.Schema) { s.Sections[0].Fields[1].GoName = "key" }},
		{"input-only runtime mapping", func(s *m.Schema) { s.Sections[0].Fields[3].GoName = "Legacy" }},
		{"input-only duplicate", func(s *m.Schema) { s.Sections[0].Fields[3].Key = "label" }},
		{"nil example", func(s *m.Schema) { s.Sections[0].Fields[0].Examples = []m.Expr{nil} }},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			s := testschema.Definition()
			tc.mutate(&s)
			out, err := codegen.Generate(s, codegen.Options{Package: "test", LibraryImport: "model"})
			if err == nil || len(out) > 0 {
				t.Fatal("invalid mapping emitted output")
			}
			_, again := codegen.Generate(s, codegen.Options{Package: "test", LibraryImport: "model"})
			if again.Error() != err.Error() {
				t.Fatal("nondeterministic error")
			}
		})
	}
}

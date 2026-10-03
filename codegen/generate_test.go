package codegen_test

import (
	"bytes"
	"os"
	"strings"
	"testing"

	m "github.com/arran4/address/internal/configmodel"
	"github.com/arran4/address/internal/configmodel/codegen"
	"github.com/arran4/address/internal/configmodel/codegen/testschema"
)

func TestFixtureIsCurrentAndDeterministic(t *testing.T) {
	s := testschema.Definition()
	options := codegen.Options{Package: "testfixture", ModelImport: "github.com/arran4/address/internal/configmodel"}
	first, err := codegen.Generate(s, options)
	if err != nil {
		t.Fatal(err)
	}
	second, err := codegen.Generate(s, options)
	if err != nil {
		t.Fatal(err)
	}
	want, err := os.ReadFile("testfixture/config_generated.go")
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(first, second) || !bytes.Equal(first, want) {
		t.Fatal("generated fixture is stale or nondeterministic")
	}
	for _, fragment := range []string{"// Code generated", "DO NOT EDIT.", "Key     []byte", "configmodel.Provider[[]byte]", "ResolveField[[]byte]", `"service", "key"`} {
		if !strings.Contains(string(first), fragment) {
			t.Errorf("missing %q", fragment)
		}
	}
	if strings.Contains(string(first), "reflect.") {
		t.Fatal("runtime reflection in generated binding")
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
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			s := testschema.Definition()
			tc.mutate(&s)
			out, err := codegen.Generate(s, codegen.Options{Package: "test", ModelImport: "model"})
			if err == nil || len(out) > 0 {
				t.Fatal("invalid mapping emitted output")
			}
			_, again := codegen.Generate(s, codegen.Options{Package: "test", ModelImport: "model"})
			if again.Error() != err.Error() {
				t.Fatal("nondeterministic error")
			}
		})
	}
}

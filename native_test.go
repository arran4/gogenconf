package gogenconf

import (
	"bytes"
	"context"
	"errors"
	"os"
	"reflect"
	"strings"
	"testing"
)

func TestNativeDocumentRoundTripOrderingAndComments(t *testing.T) {
	input := "# before version\nconfig_version 1\n\nsection custom\n    ## managed description\n    # user description\n    unknown from_json_file(from_env(CONFIG), \".nested.key\")\n\n    empty \"\"\nend\n# between sections\n\nsection another\n    value \"a,b(c) \\\\ \\\"quoted\\\"\"\nend\n# trailing comment\n"
	d, err := Parse(strings.NewReader(input))
	if err != nil {
		t.Fatal(err)
	}
	first := Format(d)
	if first != strings.Replace(input, "\".nested.key\"", ".nested.key", 1) {
		t.Fatalf("structure changed:\n%s", first)
	}
	again, err := Parse(strings.NewReader(first))
	if err != nil {
		t.Fatal(err)
	}
	if Format(again) != first {
		t.Fatal("non-deterministic native roundtrip")
	}
	if d.Sections[0].Name != "custom" || d.Sections[1].Name != "another" {
		t.Fatal("section ordering")
	}
	again.Section("custom").Set("unknown", Literal{Value: "replacement"})
	again.Section("custom").Delete("empty")
	edited := Format(again)
	for _, comment := range []string{"# before version", "# user description", "# between sections", "# trailing comment", "## managed description"} {
		if !strings.Contains(edited, comment) {
			t.Fatalf("lost %s", comment)
		}
	}
}

func TestNativeMalformedDocuments(t *testing.T) {
	for _, input := range []string{
		"config_version 1 garbage\n", "config_version -1\n", "config_version 1\nconfig_version 1\n",
		"section x\n", "end\n", "section x\nsection y\nend\n",
		"section x\n a v\n a w\nend\n", "section x\nend\nsection x\nend\n",
		"section x\n config_version 1\nend\n", "section x\nend junk\n",
	} {
		if _, err := Parse(strings.NewReader(input)); err == nil {
			t.Errorf("accepted malformed document %q", input)
		}
	}
}

func TestExpressionStructureAndMalformedInput(t *testing.T) {
	expressions := []Expr{
		Literal{Value: "8080"}, Literal{Value: ""}, Literal{Value: " \t leading/trailing\n "},
		Literal{Value: "a,b(c) \"quoted\" \\path"}, Literal{Value: "\x00\xff"},
		Call{Name: "nested", Args: []Expr{Call{Name: "inner", Args: []Expr{Literal{Value: "x"}}}, Literal{Value: "a,b"}}},
	}
	for _, expr := range expressions {
		out := FormatExpr(expr)
		parsed, err := ParseExpr(out)
		if err != nil {
			t.Fatal(err)
		}
		if !reflect.DeepEqual(expr, parsed) {
			t.Fatalf("changed AST for %q", out)
		}
	}
	for _, bad := range []string{`"unterminated`, `"bad\q"`, "f(,a)", "f(a,)", "f(a", "f(a))", "f(\"a\" \"b\")", "f(a(b) c)", "literal(", "a)", "1bad(x)", strings.Repeat("f(", 70) + "x" + strings.Repeat(")", 70)} {
		if _, err := ParseExpr(bad); err == nil {
			t.Errorf("accepted %q", bad)
		}
	}
}

func TestSourcesMissingVersusEmptyAndLiteralTypes(t *testing.T) {
	r, _ := NewStandardRegistry()
	ctx := context.Background()
	name := "ADDRESS_CONFIGMODEL_MISSING_TEST"
	old, present := os.LookupEnv(name)
	os.Unsetenv(name)
	t.Cleanup(func() {
		if present {
			os.Setenv(name, old)
		} else {
			os.Unsetenv(name)
		}
	})
	e := Call{Name: "from_env", Args: []Expr{Literal{Value: name}}}
	if _, err := Resolve[string](ctx, r, e); err == nil {
		t.Fatal("missing environment not distinguished")
	}
	t.Setenv(name, "")
	if v, err := Resolve[string](ctx, r, e); err != nil || v != "" {
		t.Fatalf("explicit empty = %q %v", v, err)
	}
	t.Setenv(name, " \ttext\n")
	for _, expr := range []Expr{e, Literal{Value: " \ttext\n"}} {
		s, err := Resolve[string](ctx, r, expr)
		if err != nil || s != " \ttext\n" {
			t.Fatal("string normalization")
		}
		b, err := Resolve[[]byte](ctx, r, expr)
		if err != nil || !bytes.Equal(b, []byte(" \ttext\n")) {
			t.Fatal("bytes normalization")
		}
	}
}

func TestCallerOwnedTypedRegistry(t *testing.T) {
	r := NewRegistry()
	if err := r.Register("service", "", func(context.Context, *Registry, []Expr) (any, error) { return "text", nil }); err != nil {
		t.Fatal(err)
	}
	if err := r.Register("service", []byte(nil), func(context.Context, *Registry, []Expr) (any, error) { return []byte{0, 255}, nil }); err != nil {
		t.Fatal(err)
	}
	if err := r.Register("service", "", func(context.Context, *Registry, []Expr) (any, error) { return "", nil }); err == nil {
		t.Fatal("duplicate accepted")
	}
	if err := r.Register("bad", "", nil); err == nil {
		t.Fatal("nil service accepted")
	}
	v, err := Resolve[[]byte](context.Background(), r, Call{Name: "service"})
	if err != nil || !bytes.Equal(v, []byte{0, 255}) {
		t.Fatal("typed service")
	}
	if _, err := Resolve[string](context.Background(), NewRegistry(), Call{Name: "service"}); err == nil {
		t.Fatal("registry was global")
	}
}

func TestTransactionalMigrationsAndMissingPath(t *testing.T) {
	d, _ := Parse(strings.NewReader("# operator\nsection x\n secret from_file(/definitely/not/read)\nend\n"))
	before := Format(d)
	m := NewMigrations()
	if err := m.Apply(d, 1); err == nil {
		t.Fatal("missing path accepted")
	}
	_ = m.Register(0, func(d *Document) error {
		d.Section("x").Set("secret", Literal{Value: "mutated"})
		return errors.New("failure")
	})
	if err := m.Apply(d, 1); err == nil {
		t.Fatal("expected failure")
	}
	if d.HasVersion || Format(d) != before {
		t.Fatal("failed migration mutated input")
	}
	if err := m.Register(0, func(*Document) error { return nil }); err == nil {
		t.Fatal("duplicate migration accepted")
	}
}

func TestSchemaEvolutionAcrossSerialization(t *testing.T) {
	l := func(v string) Expr { return Literal{Value: v} }
	a := Schema{Version: 1, Sections: []SectionDefinition{{Name: "s", Documentation: []string{"section A"}, Fields: []Field{{Key: "one", Default: l("X"), Documentation: []string{"documentation A"}}}}}}
	d := a.Seed()
	d.Section("s").Set("one", l("custom"))
	d.Section("s").Items = append(d.Section("s").Items, Comment{Kind: UserComment, Text: "my reason"}, Entry{Key: "unknown", Value: Call{Name: "future", Args: []Expr{l("opaque")}}})
	d.Items = append(d.Items, Comment{Kind: UserComment, Text: "end note"})
	b := Schema{Version: 1, Sections: []SectionDefinition{
		{Name: "s", Documentation: []string{"section B"}, Fields: []Field{{Key: "one", Default: l("changed"), Documentation: []string{"documentation B"}}, {Key: "two", Default: l("Y"), Documentation: []string{"second field"}}}},
		{Name: "new", Fields: []Field{{Key: "three", Default: l("Z")}}},
	}}
	if err := b.Enrich(d); err != nil {
		t.Fatal(err)
	}
	first := Format(d)
	for _, want := range []string{"one custom", "two Y", "## documentation B", "# my reason", "unknown future(opaque)", "# end note", "section new"} {
		if !strings.Contains(first, want) {
			t.Fatalf("missing %s", want)
		}
	}
	if strings.Contains(first, "documentation A") || strings.Contains(first, "section A") {
		t.Fatal("old managed docs remain")
	}
	parsed, err := Parse(strings.NewReader(first))
	if err != nil {
		t.Fatal(err)
	}
	if err := b.Enrich(parsed); err != nil {
		t.Fatal(err)
	}
	if Format(parsed) != first {
		t.Fatalf("reload enrichment not idempotent\n%s\n%s", first, Format(parsed))
	}
	b.Sections[0].Fields[0].Documentation = nil
	if err := b.Enrich(parsed); err != nil {
		t.Fatal(err)
	}
	if strings.Contains(Format(parsed), "documentation B") {
		t.Fatal("removed documentation not refreshed")
	}
}

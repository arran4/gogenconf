package configmodel

import (
	"context"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

func TestExpressionRoundTrip(t *testing.T) {
	for _, input := range []string{"8080", `"a value, with (syntax) and \"quotes\""`, "from_file(from_env(KEY_FILE))", `from_json_file(from_env(CONFIG_FILE), ".oauth.client_secret")`} {
		expr, err := ParseExpr(input)
		if err != nil {
			t.Fatalf("ParseExpr(%q): %v", input, err)
		}
		formatted := FormatExpr(expr)
		again, err := ParseExpr(formatted)
		if err != nil {
			t.Fatalf("parse formatted %q: %v", formatted, err)
		}
		if FormatExpr(again) != formatted {
			t.Fatalf("round trip %q => %q => %q", input, formatted, FormatExpr(again))
		}
	}
	for _, input := range []string{"from_file(", "from_file(a b", "from_env(a,)"} {
		if _, err := ParseExpr(input); err == nil {
			t.Errorf("ParseExpr(%q) succeeded", input)
		}
	}
}

func TestRegistryTypedResolutionAndExactBytes(t *testing.T) {
	dir := t.TempDir()
	file := filepath.Join(dir, "key")
	payload := []byte{' ', '\t', 0, 'a', '\n', ' '}
	if err := os.WriteFile(file, payload, 0o600); err != nil {
		t.Fatal(err)
	}
	jsonFile := filepath.Join(dir, "config.json")
	if err := os.WriteFile(jsonFile, []byte(`{"oauth":{"client_secret":"json-secret"}}`), 0o600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("KEY_FILE", file)
	t.Setenv("JSON_FILE", jsonFile)
	t.Setenv("DIRECT_VALUE", "environment")
	registry, err := NewStandardRegistry()
	if err != nil {
		t.Fatal(err)
	}
	expr, _ := ParseExpr("from_file(from_env(KEY_FILE))")
	bytes, err := Resolve[[]byte](context.Background(), registry, expr)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(bytes, payload) {
		t.Fatalf("bytes changed: %v != %v", bytes, payload)
	}
	text, err := Resolve[string](context.Background(), registry, expr)
	if err != nil {
		t.Fatal(err)
	}
	if text != string(payload) {
		t.Fatalf("string changed: %q", text)
	}
	env, _ := ParseExpr("from_env(DIRECT_VALUE)")
	envBytes, err := Resolve[[]byte](context.Background(), registry, env)
	if err != nil || string(envBytes) != "environment" {
		t.Fatalf("env bytes = %q, %v", envBytes, err)
	}
	jsonExpr, _ := ParseExpr(`from_json_file(from_env(JSON_FILE), ".oauth.client_secret")`)
	jsonValue, err := Resolve[string](context.Background(), registry, jsonExpr)
	if err != nil || jsonValue != "json-secret" {
		t.Fatalf("json value = %q, %v", jsonValue, err)
	}
	if _, err := Resolve[int](context.Background(), registry, expr); err == nil || !strings.Contains(err.Error(), "does not support") {
		t.Fatalf("unsupported type error = %v", err)
	}
	unknown, _ := ParseExpr("not_registered(value)")
	if _, err := Resolve[string](context.Background(), registry, unknown); err == nil || !strings.Contains(err.Error(), "unknown declarer") {
		t.Fatalf("unknown error = %v", err)
	}
	if err := registry.Register("from_file", "", func(context.Context, *Registry, []Expr) (any, error) { return "", nil }); err == nil {
		t.Fatal("duplicate registration succeeded")
	}
}

func TestSchemaSeedAndEnrichPreservesUserContent(t *testing.T) {
	l := func(s string) Expr { return Literal{Value: s} }
	a := Schema{Version: 1, Sections: []SectionDefinition{{Name: "known", Fields: []Field{{Key: "field_one", Default: l("X"), Documentation: []string{"documentation A"}}}}}}
	d := a.Seed()
	section := d.Section("known")
	section.Items = append(section.Items, Comment{Kind: UserComment, Text: "my comment"})
	section.Set("field_one", l("custom"))
	d.Sections = append(d.Sections, &Section{Name: "unknown", Items: []Item{Comment{Kind: UserComment, Text: "keep me"}, Entry{Key: "other", Value: l("value")}}})
	b := Schema{Version: 1, Sections: []SectionDefinition{
		{Name: "known", Fields: []Field{
			{Key: "field_one", Default: l("changed default"), Documentation: []string{"documentation B"}},
			{Key: "field_two", Default: l("Y"), Documentation: []string{"documentation new"}},
		}},
		{Name: "added", Fields: []Field{{Key: "new", Default: l("Z")}}},
	}}
	if err := b.Enrich(d); err != nil {
		t.Fatal(err)
	}
	entry, _, _ := section.Entry("field_one")
	if FormatExpr(entry.Value) != "custom" {
		t.Fatalf("custom field overwritten: %s", FormatExpr(entry.Value))
	}
	if _, _, ok := section.Entry("field_two"); !ok {
		t.Fatal("new field missing")
	}
	if d.Section("unknown") == nil || d.Section("added") == nil {
		t.Fatal("sections not preserved/added")
	}
	first := snapshot(d)
	if err := b.Enrich(d); err != nil {
		t.Fatal(err)
	}
	if got := snapshot(d); got != first {
		t.Fatalf("enrich not idempotent\n%s\n%s", first, got)
	}
}

func snapshot(d *Document) string {
	var b strings.Builder
	for _, s := range d.Sections {
		b.WriteString(s.Name)
		for _, i := range s.Items {
			switch v := i.(type) {
			case Comment:
				b.WriteString("#" + v.Text)
			case Entry:
				b.WriteString(v.Key + "=" + FormatExpr(v.Value))
			}
		}
	}
	return b.String()
}

func TestMigrationsAdvanceOnlyAfterSuccess(t *testing.T) {
	m := NewMigrations()
	_ = m.Register(0, func(d *Document) error {
		d.Items = append(d.Items, Comment{Kind: UserComment, Text: "preserved"})
		return nil
	})
	_ = m.Register(1, func(*Document) error { return os.ErrInvalid })
	d := &Document{}
	if err := m.Apply(d, 2); err == nil {
		t.Fatal("migration succeeded")
	}
	if d.Version != 1 || !d.HasVersion {
		t.Fatalf("bad advancement: %#v", d)
	}
	future := &Document{Version: 3, HasVersion: true}
	if err := m.Apply(future, 1); err == nil {
		t.Fatal("future version accepted")
	}
}

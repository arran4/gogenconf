package gogenconf_test

import (
	_ "embed"
	"strings"
	"testing"

	m "github.com/arran4/gogenconf"
	"golang.org/x/tools/txtar"
)

//go:embed testdata/evolution.txtar
var evolutionFixture []byte

func evolutionSchema(version int) m.Schema {
	s := m.Schema{Version: version, Sections: []m.SectionDefinition{{Name: "service", Documentation: []string{"Service settings."}, Fields: []m.Field{
		{Key: "endpoint", Default: m.Literal{Value: "https://default.example.org"}, Documentation: []string{"Endpoint documentation A."}},
		{Key: "timeout", Default: m.Literal{Value: "10s"}, Documentation: []string{"Request timeout."}},
	}}}}
	if version == 2 {
		s.Sections[0].Fields[0].Default = m.Literal{Value: "https://new-default.example.org"}
		s.Sections[0].Fields[0].Documentation = []string{"Endpoint documentation B."}
		s.Sections[0].Fields = append(s.Sections[0].Fields, m.Field{Key: "retries", Default: m.Literal{Value: "3"}, Documentation: []string{"Retry count."}})
	}
	return s
}

func TestEvolutionGolden(t *testing.T) {
	archive := txtar.Parse(evolutionFixture)
	read := func(name string) string {
		t.Helper()
		for _, file := range archive.Files {
			if file.Name == name {
				return string(file.Data)
			}
		}
		t.Fatalf("fixture file %s not found", name)
		return ""
	}
	for _, tc := range []struct {
		version int
		name    string
	}{{1, "expected/v1-seed.conf"}, {2, "expected/v2-seed.conf"}} {
		if got := m.Format(evolutionSchema(tc.version).Seed()); got != read(tc.name) {
			t.Fatalf("%s seed differs:\n%s", tc.name, got)
		}
	}
	d, err := m.Parse(strings.NewReader(read("input/customized.conf")))
	if err != nil {
		t.Fatal(err)
	}
	migrations := m.NewMigrations()
	if err := migrations.Register(1, func(*m.Document) error { return nil }); err != nil {
		t.Fatal(err)
	}
	if err := migrations.Apply(d, 2); err != nil {
		t.Fatal(err)
	}
	if err := evolutionSchema(2).Enrich(d); err != nil {
		t.Fatal(err)
	}
	want := read("expected/enriched.conf")
	if got := m.Format(d); got != want {
		t.Fatalf("enrichment differs:\n%s", got)
	}
	d, err = m.Parse(strings.NewReader(want))
	if err != nil {
		t.Fatal(err)
	}
	if err := evolutionSchema(2).Enrich(d); err != nil {
		t.Fatal(err)
	}
	if m.Format(d) != want {
		t.Fatal("second enrichment changed the golden")
	}
}

func TestExamplesAreNotDefaults(t *testing.T) {
	s := m.Schema{Version: 1, Sections: []m.SectionDefinition{{Name: "service", Fields: []m.Field{
		{Key: "endpoint", Default: m.Literal{Value: "localhost"}, Examples: []m.Expr{m.Literal{Value: "another host"}}},
		{Key: "credential", Sensitive: true, Examples: []m.Expr{m.Call{Name: "from_file", Args: []m.Expr{m.Literal{Value: "/missing/credential"}}}}},
	}}}}
	for _, doc := range []*m.Document{s.Seed(), s.Sample()} {
		if doc.Value("service", "credential") != nil {
			t.Fatal("example became active")
		}
		if m.FormatExpr(doc.Value("service", "endpoint")) != "localhost" {
			t.Fatal("example replaced default")
		}
	}
	text := m.Format(s.Sample())
	if !strings.Contains(text, "##   credential from_file(/missing/credential)") {
		t.Fatal("optional example missing")
	}
	parsed, err := m.Parse(strings.NewReader(text))
	if err != nil {
		t.Fatal(err)
	}
	if parsed.Value("service", "credential") != nil {
		t.Fatal("comment activated on reload")
	}
}

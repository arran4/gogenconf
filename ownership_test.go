package gogenconf_test

import (
	"strings"
	"testing"

	m "github.com/arran4/gogenconf"
)

func TestCommentOwnershipFollowsWholeDocumentVersion(t *testing.T) {
	for _, version := range []string{"", "config_version 0\n", "config_version 1\n"} {
		input := "# ordinary legacy note\n## legacy heading\n### another legacy heading\n#### fourth heading\n" + version + "section service\n    ## field heading\n    endpoint custom\nend\n"
		d, err := m.Parse(strings.NewReader(input))
		if err != nil {
			t.Fatal(err)
		}
		current := d.Version > 0
		for i, item := range d.Items[:4] {
			c, ok := item.(m.Comment)
			if !ok {
				t.Fatal("missing comment")
			}
			want := m.UserComment
			if current && i > 0 {
				want = m.DocumentationComment
			}
			if c.Kind != want {
				t.Fatalf("version %q comment %d kind %v", version, i, c.Kind)
			}
		}
		if c := d.Section("service").Items[0].(m.Comment); (c.Kind == m.DocumentationComment) != current {
			t.Fatal("section comment ownership incorrect")
		}
		if got := m.Format(d); !current && got != input {
			t.Fatalf("parse/format changed spelling:\n%s", got)
		}
		if current {
			continue
		}
		migrations := m.NewMigrations()
		if err := migrations.Register(0, func(*m.Document) error { return nil }); err != nil {
			t.Fatal(err)
		}
		if err := migrations.Apply(d, 1); err != nil {
			t.Fatal(err)
		}
		schema := m.Schema{Version: 1, Sections: []m.SectionDefinition{{Name: "service", Fields: []m.Field{{Key: "endpoint", Documentation: []string{"Managed endpoint A."}}}}}}
		if err := schema.Enrich(d); err != nil {
			t.Fatal(err)
		}
		out := m.Format(d)
		for _, want := range []string{"# ordinary legacy note", "# # legacy heading", "# ## another legacy heading", "# ### fourth heading", "# # field heading", "## Managed endpoint A."} {
			if !strings.Contains(out, want) {
				t.Fatalf("lost comment %q:\n%s", want, out)
			}
		}
		d, err = m.Parse(strings.NewReader(out))
		if err != nil {
			t.Fatal(err)
		}
		for _, item := range d.Items {
			if c, ok := item.(m.Comment); ok && c.Kind != m.UserComment {
				t.Fatal("legacy comment reclaimed on reload")
			}
		}
		schema.Sections[0].Fields[0].Documentation = []string{"Managed endpoint B."}
		if err := schema.Enrich(d); err != nil {
			t.Fatal(err)
		}
		out = m.Format(d)
		if strings.Contains(out, "Managed endpoint A.") || !strings.Contains(out, "## Managed endpoint B.") || !strings.Contains(out, "# # field heading") {
			t.Fatal("refresh changed ownership")
		}
	}
}

func TestAcceptedAndWritableFields(t *testing.T) {
	s := m.Schema{Version: 1, Sections: []m.SectionDefinition{{Name: "service", Fields: []m.Field{
		{Key: "legacy", InputOnly: true}, {Key: "current"}, {Key: "runtime", RuntimeOnly: true},
	}}}}
	if _, ok := s.Field("service", "legacy"); !ok {
		t.Fatal("legacy input invisible to compatibility")
	}
	if _, ok := s.WritableField("service", "legacy"); ok {
		t.Fatal("legacy input writable")
	}
	if _, ok := s.WritableField("service", "current"); !ok {
		t.Fatal("current field not writable")
	}
	for _, key := range []string{"runtime", "unknown"} {
		if _, ok := s.WritableField("service", key); ok {
			t.Fatal("non-persisted field writable")
		}
	}
}

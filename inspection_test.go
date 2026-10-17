package gogenconf_test

import (
	"context"
	"strings"
	"testing"

	m "github.com/arran4/gogenconf"
)

func TestCustomResolverIsNotImplicitlyDisplaySafe(t *testing.T) {
	r := m.NewRegistry()
	if err := r.Register("local_value", "", func(context.Context, *m.Registry, []m.Expr) (any, error) { return "content", nil }); err != nil {
		t.Fatal(err)
	}
	e := m.Call{Name: "local_value", Args: []m.Expr{m.Literal{Value: "named-slot"}}}
	p := m.StandardDisplayPolicy()
	if p.SafeSource(e) {
		t.Fatal("resolver registration authorized display")
	}
	// Only the application knows that this particular implementation's argument
	// is a source identifier and never inline content. Authorization is explicit.
	p["local_value"] = func(args []m.Expr, _ m.DisplayPolicy) bool {
		if len(args) != 1 {
			return false
		}
		v, ok := args[0].(m.Literal)
		return ok && v.Value == "named-slot"
	}
	if !p.SafeSource(e) {
		t.Fatal("explicit source contract ignored")
	}
	if p.SafeSource(m.Call{Name: "local_value", Args: []m.Expr{m.Literal{Value: "raw-content"}}}) {
		t.Fatal("contract did not fail closed")
	}
}

func TestInspectionDocumentWithDottedAndCollidingRepeatedNames(t *testing.T) {
	s := m.Schema{
		Identity: "testapp-schema",
		Version:  1,
		Sections: []m.SectionDefinition{{
			Name:     "project",
			GoName:   "Projects",
			GoType:   "ProjectConfig",
			Repeated: true,
			Fields: []m.Field{
				{Key: "api_secret", GoName: "APISecret", GoType: "string", Sensitive: true},
			},
			Children: []m.SectionDefinition{{
				Name:   "build",
				GoName: "Build",
				GoType: "BuildConfig",
				Fields: []m.Field{
					{Key: "signing_key", GoName: "SigningKey", GoType: "string", Sensitive: true},
				},
			}},
		}},
	}

	// Verify Schema.Field path lookups with various formats
	if f, ok := s.Field("project", "api_secret"); !ok || !f.Sensitive {
		t.Errorf("expected Field(project, api_secret) sensitive, got ok=%v, f=%+v", ok, f)
	}
	if f, ok := s.Field("project.build", "signing_key"); !ok || !f.Sensitive {
		t.Errorf("expected Field(project.build, signing_key) sensitive, got ok=%v, f=%+v", ok, f)
	}
	if f, ok := s.Field("project.lxa.build", "signing_key"); !ok || !f.Sensitive {
		t.Errorf("expected Field(project.lxa.build, signing_key) sensitive, got ok=%v, f=%+v", ok, f)
	}
	if f, ok := s.Field("project.foo.bar.build", "signing_key"); !ok || !f.Sensitive {
		t.Errorf("expected Field(project.foo.bar.build, signing_key) sensitive, got ok=%v, f=%+v", ok, f)
	}
	if f, ok := s.Field("project.build.build", "signing_key"); !ok || !f.Sensitive {
		t.Errorf("expected Field(project.build.build, signing_key) sensitive, got ok=%v, f=%+v", ok, f)
	}

	input := "config for testapp version 1\n\n" +
		"project \"foo.bar\" {\n" +
		"  api_secret \"SECRET_DOTTED_API_123\"\n" +
		"  build {\n" +
		"    signing_key \"SECRET_DOTTED_SIGN_456\"\n" +
		"  }\n" +
		"}\n" +
		"project build {\n" +
		"  api_secret \"SECRET_COLLIDE_API_789\"\n" +
		"  build {\n" +
		"    signing_key \"SECRET_COLLIDE_SIGN_000\"\n" +
		"  }\n" +
		"}\n"

	doc, err := m.Parse(strings.NewReader(input))
	if err != nil {
		t.Fatal(err)
	}
	inspected := s.InspectionDocument(doc, m.StandardDisplayPolicy())
	formatted := m.Format(inspected)

	for _, secret := range []string{
		"SECRET_DOTTED_API_123",
		"SECRET_DOTTED_SIGN_456",
		"SECRET_COLLIDE_API_789",
		"SECRET_COLLIDE_SIGN_000",
	} {
		if strings.Contains(formatted, secret) {
			t.Errorf("LEAK: secret %q was not redacted:\n%s", secret, formatted)
		}
	}
}

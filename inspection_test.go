package gogenconf_test

import (
	"context"
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

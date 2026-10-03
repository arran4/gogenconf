package gogenconf

import (
	"context"
	"fmt"
)

type LookupEnv func(string) (string, bool)

// BindingValue encodes precedence explicitly: authored document, compatibility
// environment, schema default. Enriched defaults retain transient provenance.
func BindingValue(d *Document, section, key string, fallback Expr, env string, nonempty bool, lookup LookupEnv) Expr {
	entry, _, present := d.Section(section).Entry(key)
	if present && entry.Value != nil && !entry.Defaulted {
		return entry.Value
	}
	if lookup != nil && env != "" {
		if value, ok := lookup(env); ok && (!nonempty || value != "") {
			return Literal{Value: value}
		}
	}
	if present && entry.Value != nil {
		return entry.Value
	}
	return CloneExpr(fallback)
}

// ResolveField prevents a registered service error from disclosing a sensitive
// value through generated binders or providers.
func ResolveField[T any](ctx context.Context, r *Registry, e Expr, path string, sensitive bool) (T, error) {
	v, err := Resolve[T](ctx, r, e)
	if err != nil {
		var zero T
		if sensitive {
			return zero, fmt.Errorf("resolve %s: source resolution failed", path)
		}
		return zero, fmt.Errorf("resolve %s: %w", path, err)
	}
	return v, nil
}

func FieldProvider[T any](r *Registry, e Expr, path string, sensitive bool) Provider[T] {
	e = CloneExpr(e)
	return ProviderFunc[T](func(ctx context.Context) (T, error) { return ResolveField[T](ctx, r, e, path, sensitive) })
}

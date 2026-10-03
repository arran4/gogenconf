package configmodel

import "context"

// Provider is an application-facing content boundary, separate from declarer
// services. No cache or invalidation policy is imposed on ordinary fields.
type Provider[T any] interface {
	Resolve(context.Context) (T, error)
}
type ProviderFunc[T any] func(context.Context) (T, error)

func (f ProviderFunc[T]) Resolve(ctx context.Context) (T, error) { return f(ctx) }

// Deferred snapshots the declaration. Registry registration must be completed
// before providers are shared with concurrent callers.
func Deferred[T any](registry *Registry, expr Expr) Provider[T] {
	expr = CloneExpr(expr)
	return ProviderFunc[T](func(ctx context.Context) (T, error) { return Resolve[T](ctx, registry, expr) })
}

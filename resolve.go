package configmodel

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"reflect"
	"strings"
	"sync"
)

type Resolver func(context.Context, *Registry, []Expr) (any, error)
type Registry struct {
	mu        sync.RWMutex
	resolvers map[string]map[reflect.Type]Resolver
}

func NewRegistry() *Registry { return &Registry{resolvers: map[string]map[reflect.Type]Resolver{}} }
func (r *Registry) Register(name string, result any, resolver Resolver) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if resolver == nil {
		return fmt.Errorf("resolver cannot be nil")
	}
	if r.resolvers == nil {
		r.resolvers = map[string]map[reflect.Type]Resolver{}
	}
	t := reflect.TypeOf(result)
	if t == nil {
		return fmt.Errorf("resolver result type cannot be nil")
	}
	if r.resolvers[name] == nil {
		r.resolvers[name] = map[reflect.Type]Resolver{}
	}
	if r.resolvers[name][t] != nil {
		return fmt.Errorf("resolver %q for %s already registered", name, t)
	}
	r.resolvers[name][t] = resolver
	return nil
}
func Resolve[T any](ctx context.Context, r *Registry, expr Expr) (T, error) {
	var zero T
	v, err := r.ResolveAs(ctx, expr, reflect.TypeFor[T]())
	if err != nil {
		return zero, err
	}
	typed, ok := v.(T)
	if !ok {
		return zero, fmt.Errorf("resolver returned %T, expected %T", v, zero)
	}
	return typed, nil
}
func (r *Registry) ResolveAs(ctx context.Context, expr Expr, want reflect.Type) (any, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if r == nil {
		return nil, fmt.Errorf("nil resolver registry")
	}
	switch e := expr.(type) {
	case Literal:
		r.mu.RLock()
		fn := r.resolvers[""][want]
		r.mu.RUnlock()
		if fn != nil {
			return fn(ctx, r, []Expr{e})
		}
		return literalAs(e.Value, want)
	case *Literal:
		return r.ResolveAs(ctx, *e, want)
	case Call:
		r.mu.RLock()
		byType := r.resolvers[e.Name]
		if byType == nil {
			r.mu.RUnlock()
			return nil, fmt.Errorf("unknown declarer %q", e.Name)
		}
		resolver := byType[want]
		r.mu.RUnlock()
		if resolver == nil {
			return nil, fmt.Errorf("declarer %q does not support result type %s", e.Name, want)
		}
		value, err := resolver(ctx, r, e.Args)
		if err != nil {
			return nil, err
		}
		if reflect.TypeOf(value) != want {
			return nil, fmt.Errorf("declarer returned incorrect result type")
		}
		return value, nil
	case *Call:
		return r.ResolveAs(ctx, *e, want)
	default:
		return nil, fmt.Errorf("unsupported expression %T", expr)
	}
}
func literalAs(value string, want reflect.Type) (any, error) {
	switch want {
	case reflect.TypeFor[string]():
		return value, nil
	case reflect.TypeFor[[]byte]():
		return []byte(value), nil
	default:
		return nil, fmt.Errorf("literal does not support result type %s", want)
	}
}

// NewStandardRegistry provides a caller-owned registry with the built-in local
// declarers. It can be extended without changing the AST or parser.
func NewStandardRegistry() (*Registry, error) {
	r := NewRegistry()
	envName := func(ctx context.Context, r *Registry, args []Expr) (string, error) {
		if len(args) != 1 {
			return "", fmt.Errorf("from_env expects 1 argument")
		}
		name, err := Resolve[string](ctx, r, args[0])
		if err != nil {
			return "", err
		}
		return name, nil
	}
	if err := r.Register("from_env", "", func(ctx context.Context, r *Registry, args []Expr) (any, error) {
		name, err := envName(ctx, r, args)
		if err != nil {
			return nil, err
		}
		value, present := os.LookupEnv(name)
		if !present {
			return nil, fmt.Errorf("from_env: variable is not set")
		}
		return value, nil
	}); err != nil {
		return nil, err
	}
	if err := r.Register("from_env", []byte(nil), func(ctx context.Context, r *Registry, args []Expr) (any, error) {
		name, err := envName(ctx, r, args)
		if err != nil {
			return nil, err
		}
		value, present := os.LookupEnv(name)
		if !present {
			return nil, fmt.Errorf("from_env: variable is not set")
		}
		return []byte(value), nil
	}); err != nil {
		return nil, err
	}
	if err := r.Register("from_file", "", func(ctx context.Context, r *Registry, args []Expr) (any, error) {
		if len(args) != 1 {
			return nil, fmt.Errorf("from_file expects 1 argument")
		}
		path, err := Resolve[string](ctx, r, args[0])
		if err != nil {
			return nil, err
		}
		b, err := os.ReadFile(path)
		if err != nil {
			return nil, fmt.Errorf("from_file: cannot read source")
		}
		return string(b), nil
	}); err != nil {
		return nil, err
	}
	if err := r.Register("from_file", []byte(nil), func(ctx context.Context, r *Registry, args []Expr) (any, error) {
		if len(args) != 1 {
			return nil, fmt.Errorf("from_file expects 1 argument")
		}
		path, err := Resolve[string](ctx, r, args[0])
		if err != nil {
			return nil, err
		}
		b, err := os.ReadFile(path)
		if err != nil {
			return nil, fmt.Errorf("from_file: cannot read source")
		}
		return b, nil
	}); err != nil {
		return nil, err
	}
	jsonResolver := func(ctx context.Context, r *Registry, args []Expr) (string, error) {
		if len(args) != 2 {
			return "", fmt.Errorf("from_json_file expects 2 arguments")
		}
		path, err := Resolve[string](ctx, r, args[0])
		if err != nil {
			return "", err
		}
		selector, err := Resolve[string](ctx, r, args[1])
		if err != nil {
			return "", err
		}
		b, err := os.ReadFile(path)
		if err != nil {
			return "", fmt.Errorf("from_json_file: cannot read source")
		}
		var value any
		if err := json.Unmarshal(b, &value); err != nil {
			return "", fmt.Errorf("from_json_file: invalid JSON")
		}
		for _, part := range strings.Split(strings.TrimPrefix(selector, "."), ".") {
			if part == "" {
				continue
			}
			object, ok := value.(map[string]any)
			if !ok {
				return "", fmt.Errorf("from_json_file: selector does not traverse an object")
			}
			var exists bool
			value, exists = object[part]
			if !exists {
				return "", fmt.Errorf("from_json_file: selector not found")
			}
		}
		if value == nil {
			return "", fmt.Errorf("from_json_file resolved to null value")
		}
		return fmt.Sprint(value), nil
	}
	if err := r.Register("from_json_file", "", func(ctx context.Context, r *Registry, args []Expr) (any, error) { return jsonResolver(ctx, r, args) }); err != nil {
		return nil, err
	}
	if err := r.Register("from_json_file", []byte(nil), func(ctx context.Context, r *Registry, args []Expr) (any, error) {
		v, e := jsonResolver(ctx, r, args)
		return []byte(v), e
	}); err != nil {
		return nil, err
	}
	// Kept solely as a compatibility alias for legacy Address documents. New
	// documents use the composable spelling from_file(from_env(NAME)).
	for _, result := range []any{"", []byte(nil)} {
		result := result
		if err := r.Register("from_env_file", result, func(ctx context.Context, r *Registry, args []Expr) (any, error) {
			if len(args) != 1 {
				return nil, fmt.Errorf("from_env_file expects 1 argument")
			}
			return r.ResolveAs(ctx, Call{Name: "from_file", Args: []Expr{Call{Name: "from_env", Args: args}}}, reflect.TypeOf(result))
		}); err != nil {
			return nil, err
		}
	}
	return r, nil
}

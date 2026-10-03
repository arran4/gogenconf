package configmodel_test

import (
	"context"
	"fmt"
	"os"
	"strings"

	m "github.com/arran4/address/internal/configmodel"
)

func ExampleParseExpr() {
	for _, source := range []string{
		`literal`, `"quoted literal"`, `from_env(NAME)`,
		`from_file("/a path/key")`, `from_file(from_env(KEY_FILE))`,
		`from_json_file(from_env(CONFIG_FILE), ".nested.value")`,
	} {
		e, err := m.ParseExpr(source)
		if err != nil {
			panic(err)
		}
		fmt.Println(m.FormatExpr(e)) // canonical, unresolved; no source need exist
	}
	// Output:
	// literal
	// "quoted literal"
	// from_env(NAME)
	// from_file("/a path/key")
	// from_file(from_env(KEY_FILE))
	// from_json_file(from_env(CONFIG_FILE), .nested.value)
}

func ExampleResolve() {
	f, err := os.CreateTemp("", "configmodel-example-*")
	if err != nil {
		panic(err)
	}
	defer os.Remove(f.Name())
	if _, err = f.Write([]byte(" \x00content\n")); err != nil {
		panic(err)
	}
	if err = f.Close(); err != nil {
		panic(err)
	}
	e := m.Call{Name: "from_file", Args: []m.Expr{m.Literal{Value: f.Name()}}}
	r, err := m.NewStandardRegistry()
	if err != nil {
		panic(err)
	}
	text, err := m.Resolve[string](context.Background(), r, e)
	if err != nil {
		panic(err)
	}
	data, err := m.Resolve[[]byte](context.Background(), r, e)
	if err != nil {
		panic(err)
	}
	fmt.Printf("string: %q\nbytes: %v\n", text, data)
	// Output:
	// string: " \x00content\n"
	// bytes: [32 0 99 111 110 116 101 110 116 10]
}

func ExampleRegistry_Register() {
	r := m.NewRegistry()
	err := r.Register("local_value", "", func(ctx context.Context, r *m.Registry, args []m.Expr) (any, error) {
		if len(args) != 1 {
			return nil, fmt.Errorf("local_value requires one argument")
		}
		name, err := m.Resolve[string](ctx, r, args[0])
		if err != nil {
			return nil, err
		}
		return strings.ToUpper(name), nil
	})
	if err != nil {
		panic(err)
	}
	e, err := m.ParseExpr(`local_value(example)`)
	if err != nil {
		panic(err)
	}
	v, err := m.Resolve[string](context.Background(), r, e)
	if err != nil {
		panic(err)
	}
	fmt.Println(v)
	// Output: EXAMPLE
}

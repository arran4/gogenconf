package testfixture

import (
	"context"
	"fmt"
	"os"
	"path/filepath"

	m "github.com/arran4/address/internal/configmodel"
)

func ExampleResolve_provider() {
	dir, err := os.MkdirTemp("", "provider-example-*")
	if err != nil {
		panic(err)
	}
	defer os.RemoveAll(dir)
	path := filepath.Join(dir, "content") // does not exist at binding time
	d := &m.Document{Version: 2, HasVersion: true}
	s := &m.Section{Name: "service"}
	d.AddSection(s)
	s.Set("key", m.Literal{Value: "fixture bytes"})
	s.Set("content", m.Call{Name: "from_file", Args: []m.Expr{m.Literal{Value: path}}})
	r, err := m.NewStandardRegistry()
	if err != nil {
		panic(err)
	}
	cfg, err := Resolve(context.Background(), d, r, nil)
	if err != nil {
		panic(err)
	}
	fmt.Printf("eager: %s; bound before content exists\n", cfg.Service.Key)
	// Provider callers need no document or expression API. An external content
	// lifecycle/cache can wrap this interface; this provider itself does not cache.
	content := cfg.Service.Content
	s.Set("content", m.Literal{Value: "different target"})
	for _, value := range []string{"first", "later"} {
		if err := os.WriteFile(path, []byte(value), 0600); err != nil {
			panic(err)
		}
		v, err := content.Resolve(context.Background())
		if err != nil {
			panic(err)
		}
		fmt.Println(string(v))
	}
	// Output:
	// eager: fixture bytes; bound before content exists
	// first
	// later
}

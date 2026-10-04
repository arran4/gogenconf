//go:generate go run ./internal/generate

package main

import (
	"context"
	"fmt"
	"os"
	"path/filepath"

	"github.com/arran4/gogenconf"
	"github.com/arran4/gogenconf/examples/generated/provider/binding"
)

func main() {
	dir, err := os.MkdirTemp("", "gogenconf-generated-provider-*")
	if err != nil {
		panic(err)
	}
	defer os.RemoveAll(dir)
	path := filepath.Join(dir, "content")
	doc := &gogenconf.Document{Version: 1, HasVersion: true}
	section := &gogenconf.Section{Name: "service"}
	doc.AddSection(section)
	section.Set("content", gogenconf.Call{Name: "from_file", Args: []gogenconf.Expr{gogenconf.Literal{Value: path}}})
	registry, err := gogenconf.NewStandardRegistry()
	if err != nil {
		panic(err)
	}
	config, err := binding.Resolve(context.Background(), doc, registry, nil)
	if err != nil {
		panic(err)
	}
	if err := os.WriteFile(path, []byte("generated provider"), 0600); err != nil {
		panic(err)
	}
	value, err := config.Service.Content.Resolve(context.Background())
	if err != nil {
		panic(err)
	}
	fmt.Println(string(value))
}

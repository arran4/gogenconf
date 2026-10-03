package main

import (
	"context"
	"fmt"
	"github.com/arran4/gogenconf"
	"os"
	"path/filepath"
)

func main() {
	dir, err := os.MkdirTemp("", "gogenconf-provider-*")
	if err != nil {
		panic(err)
	}
	defer os.RemoveAll(dir)
	path := filepath.Join(dir, "large-content")
	r, err := gogenconf.NewStandardRegistry()
	if err != nil {
		panic(err)
	}
	p := gogenconf.FieldProvider[[]byte](r, gogenconf.Call{Name: "from_file", Args: []gogenconf.Expr{gogenconf.Literal{Value: path}}}, "service.content", false)
	fmt.Println("provider created before file exists")
	for _, s := range []string{"first content", "updated content"} {
		if err = os.WriteFile(path, []byte(s), 0600); err != nil {
			panic(err)
		}
		b, err := p.Resolve(context.Background())
		if err != nil {
			panic(err)
		}
		fmt.Println(string(b))
	}
}

package main

import (
	"bytes"
	"context"
	"fmt"
	"github.com/arran4/gogenconf"
	"os"
)

func main() {
	f, err := os.CreateTemp("", "gogenconf-source-*")
	if err != nil {
		panic(err)
	}
	defer os.Remove(f.Name())
	data := []byte(" \x00example\n")
	if _, err = f.Write(data); err != nil {
		panic(err)
	}
	if err = f.Close(); err != nil {
		panic(err)
	}
	if err = os.Setenv("EXAMPLE_CONTENT_FILE", f.Name()); err != nil {
		panic(err)
	}
	e, err := gogenconf.ParseExpr("from_file(from_env(EXAMPLE_CONTENT_FILE))")
	if err != nil {
		panic(err)
	}
	r, err := gogenconf.NewStandardRegistry()
	if err != nil {
		panic(err)
	}
	b, err := gogenconf.Resolve[[]byte](context.Background(), r, e)
	if err != nil {
		panic(err)
	}
	s, err := gogenconf.Resolve[string](context.Background(), r, e)
	if err != nil {
		panic(err)
	}
	fmt.Println("exact bytes:", bytes.Equal(data, b), "same string:", s == string(data))
	fmt.Println(gogenconf.FormatExpr(e))
}

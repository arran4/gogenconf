package main

import (
	"context"
	"fmt"
	"github.com/arran4/gogenconf"
	"strings"
)

func main() {
	d, err := gogenconf.Parse(strings.NewReader("config_version 1\nsection service\n endpoint https://example.test\nend\n"))
	if err != nil {
		panic(err)
	}
	r, err := gogenconf.NewStandardRegistry()
	if err != nil {
		panic(err)
	}
	v, err := gogenconf.Resolve[string](context.Background(), r, d.Value("service", "endpoint"))
	if err != nil {
		panic(err)
	}
	fmt.Println("endpoint:", v)
	fmt.Print(gogenconf.Format(d))
}

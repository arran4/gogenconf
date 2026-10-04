//go:generate go run ./internal/generate

package main

import (
	"context"
	"fmt"
	"strings"

	"github.com/arran4/gogenconf"
	"github.com/arran4/gogenconf/examples/generated/repeated/binding"
)

func main() {
	doc, err := gogenconf.Parse(strings.NewReader("config_version 1\nsection worker one\n label first\nend\nsection worker two\n label second\nend\n"))
	if err != nil {
		panic(err)
	}
	registry, err := gogenconf.NewStandardRegistry()
	if err != nil {
		panic(err)
	}
	config, err := binding.Resolve(context.Background(), doc, registry, nil)
	if err != nil {
		panic(err)
	}
	fmt.Printf("workers: %d (%s, %s)\n", len(config.Workers), config.Workers[0].Name, config.Workers[1].Name)
}

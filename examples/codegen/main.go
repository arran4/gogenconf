package main

//go:generate go run ./internal/generate

import (
	"context"
	"fmt"
	"github.com/arran4/gogenconf"
	"github.com/arran4/gogenconf/examples/codegen/appconfig"
	"github.com/arran4/gogenconf/examples/codegen/binding"
	"github.com/arran4/gogenconf/examples/codegen/internal/schema"
	"os"
	"strings"
)

func main() {
	f, err := os.CreateTemp("", "gogenconf-credential-*")
	if err != nil {
		panic(err)
	}
	defer os.Remove(f.Name())
	if _, err = f.Write([]byte(" \x00example\n")); err != nil {
		panic(err)
	}
	if err = f.Close(); err != nil {
		panic(err)
	}
	if err = os.Setenv("CREDENTIAL_FILE", f.Name()); err != nil {
		panic(err)
	}
	d, err := gogenconf.Parse(strings.NewReader("config_version 1\nsection service\n credential from_file(from_env(CREDENTIAL_FILE))\nend\n"))
	if err != nil {
		panic(err)
	}
	if err = schema.Definition().Enrich(d); err != nil {
		panic(err)
	}
	r, err := gogenconf.NewStandardRegistry()
	if err != nil {
		panic(err)
	}
	var cfg appconfig.Config
	cfg, err = binding.Resolve(context.Background(), d, r, nil)
	if err != nil {
		panic(err)
	}
	var endpoint string = cfg.Service.Endpoint
	var credential []byte = cfg.Service.Credential
	if string(credential) != " \x00example\n" {
		panic("credential bytes changed")
	}
	fmt.Printf("typed endpoint: %s; credential bytes: %d\n", endpoint, len(credential))
}

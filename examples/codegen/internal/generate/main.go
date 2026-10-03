package main

import (
	"github.com/arran4/gogenconf/codegen"
	"github.com/arran4/gogenconf/examples/codegen/internal/schema"
	"os"
	"path/filepath"
	"strings"
)

func main() {
	// This driver runs from examples/codegen (or a copied standalone module).
	module := "github.com/arran4/gogenconf/examples/codegen"
	if b, err := os.ReadFile("go.mod"); err == nil {
		module = strings.Fields(string(b))[1]
	}
	model, err := codegen.GenerateModel(schema.Definition(), codegen.Options{Package: "appconfig"})
	if err != nil {
		panic(err)
	}
	binder, err := codegen.GenerateBinder(schema.Definition(), codegen.Options{Package: "binding", ConfigImport: module + "/appconfig"})
	if err != nil {
		panic(err)
	}
	for path, data := range map[string][]byte{"appconfig/config_generated.go": model, "binding/binding_generated.go": binder} {
		if err = os.MkdirAll(filepath.Dir(path), 0755); err != nil {
			panic(err)
		}
		if err = os.WriteFile(path, data, 0644); err != nil {
			panic(err)
		}
	}
}

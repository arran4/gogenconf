package main

import (
	"os"

	"github.com/arran4/gogenconf/codegen"
	"github.com/arran4/gogenconf/examples/generated/provider/internal/schema"
)

func main() {
	const module = "github.com/arran4/gogenconf/examples/generated/provider"
	model, err := codegen.GenerateModel(schema.Definition(), codegen.Options{Package: "appconfig"})
	if err != nil {
		panic(err)
	}
	if err := os.MkdirAll("appconfig", 0755); err != nil {
		panic(err)
	}
	if err := os.WriteFile("appconfig/config_generated.go", model, 0644); err != nil {
		panic(err)
	}
	binder, err := codegen.GenerateBinder(schema.Definition(), codegen.Options{Package: "binding", ConfigImport: module + "/appconfig"})
	if err != nil {
		panic(err)
	}
	if err := os.MkdirAll("binding", 0755); err != nil {
		panic(err)
	}
	if err := os.WriteFile("binding/binding_generated.go", binder, 0644); err != nil {
		panic(err)
	}
}

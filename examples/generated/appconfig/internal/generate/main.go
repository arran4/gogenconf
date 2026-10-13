package main

import (
	"os"
	"path/filepath"
	"strings"

	"github.com/arran4/gogenconf/codegen"
	"github.com/arran4/gogenconf/examples/generated/appconfig/internal/schema"
)

func main() {
	module := "github.com/arran4/gogenconf/examples/generated/appconfig"
	if b, err := os.ReadFile("go.mod"); err == nil {
		module = strings.Fields(string(b))[1]
	}

	// 1. Generate standalone native runtime with zero gogenconf dependency
	rtFiles, err := codegen.GenerateRuntime(codegen.RuntimeOptions{
		Package:      "nativeconfig",
		Capabilities: codegen.FullRuntimeCapabilities(),
	})
	if err != nil {
		panic(err)
	}
	if err := rtFiles.WriteToDir("nativeconfig"); err != nil {
		panic(err)
	}

	// 2. Generate schema-specialized application config API
	apiCode, err := codegen.GenerateAppAPI(schema.Definition(), codegen.Options{
		Package:       "appconfig",
		RuntimeImport: module + "/nativeconfig",
	})
	if err != nil {
		panic(err)
	}

	if err := os.MkdirAll("appconfig", 0755); err != nil {
		panic(err)
	}
	if err := os.WriteFile(filepath.Join("appconfig", "config_generated.go"), apiCode, 0644); err != nil {
		panic(err)
	}
}

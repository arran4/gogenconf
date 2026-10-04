// runtimegen writes the production native syntax runtime used by the gogenconf CLI.
package main

import "github.com/arran4/gogenconf/codegen"

func main() {
	files, err := codegen.GenerateRuntime(codegen.RuntimeOptions{Package: "nativeconfig"})
	if err != nil {
		panic(err)
	}
	if err := files.WriteToDir("internal/nativeconfig"); err != nil {
		panic(err)
	}
}

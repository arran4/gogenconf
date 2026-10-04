package main

import "github.com/arran4/gogenconf/codegen"

func main() {
	files, err := codegen.GenerateRuntime(codegen.RuntimeOptions{Package: "native"})
	if err != nil {
		panic(err)
	}
	if err := files.WriteToDir("native"); err != nil {
		panic(err)
	}
}

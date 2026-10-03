// fixturegen regenerates the private compiled codegen fixture.
package main

import (
	"os"

	"github.com/arran4/gogenconf/codegen"
	"github.com/arran4/gogenconf/codegen/internal/testschema"
)

func main() {
	b, err := codegen.Generate(testschema.Definition(), codegen.Options{Package: "testfixture"})
	if err != nil {
		panic(err)
	}
	if err := os.WriteFile("internal/testfixture/config_generated.go", b, 0644); err != nil {
		panic(err)
	}
}

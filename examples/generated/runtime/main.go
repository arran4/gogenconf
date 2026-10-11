//go:generate go run ./internal/generate

// The runtime example uses only application-owned generated native syntax code.
package main

import (
	"fmt"
	"strings"

	"github.com/arran4/gogenconf/examples/generated/runtime/native"
)

func main() {
	doc, err := native.Parse(strings.NewReader("config_version 1\nsection service\n endpoint from_env(SERVICE_ENDPOINT)\nend\n"))
	if err != nil {
		panic(err)
	}
	fmt.Print(native.Format(doc))
}

---
title: "Quick Start"
weight: 2
---

<!-- Generated from README.md by internal/docgen. DO NOT EDIT. -->

## Quick start

There is not yet a tagged release. Clone the candidate branch referenced by the
open extraction PR until it merges; then use main. From the checkout:

```sh
go generate ./examples/codegen
go run ./examples/codegen
# typed endpoint: https://example.test; credential bytes: 10
go run ./cmd/gogenconf --help
printf 'config_version 1\nsection service\n endpoint https://example.test\nend\n' |
  go run ./cmd/gogenconf format
```

The [complete codegen application](https://github.com/arran4/gogenconf/blob/main/examples/codegen) includes a Go schema, local
generator, concrete model, separate binder, and loading code. It creates a
disposable credential file, places its path in `CREDENTIAL_FILE`, and loads the
neutral configuration below. Provision real sources separately; never print
credential contents.
## Configuration stays declarative

```text
config_version 1

section service
    ## Service endpoint.
    endpoint https://example.test

    # Operator-selected credential location.
    credential from_file(from_env(CREDENTIAL_FILE))
end
```

Parsing produces an unresolved `Document` containing sections, ordered entries,
comments, blank lines, and `Expr` values (`Literal` or recursive `Call`). Unknown
sections and fields survive. Formatting is deterministic and performs no source
I/O. Values can be bare literals, Go-style quoted/escaped strings, or nested calls
with multiple arguments, such as `from_json_file(from_env(CONFIG_FILE), .auth.token)`.

```go
doc, err := gogenconf.Parse(strings.NewReader(input))
if err != nil { return err }
registry, err := gogenconf.NewStandardRegistry()
if err != nil { return err }
credential, err := gogenconf.Resolve[[]byte](https://github.com/arran4/gogenconf/blob/main/
    ctx, registry, doc.Value("service", "credential"),
)
if err != nil { return err }
// Use credential; formatting doc still shows the original declaration.
```

Resolution is explicit and selected by **declarer name + requested Go type**.
`Resolve[string](https://github.com/arran4/gogenconf/blob/main/ctx, registry, expr)` and `Resolve[[]byte](https://github.com/arran4/gogenconf/blob/main/ctx, registry, expr)`
can consume the same `from_file(...)` expression. Both preserve exact content,
including NUL, whitespace, and newlines; there is no implicit trimming.

The standard registry supplies literals, `from_env`, `from_file`,
`from_json_file` (small dotted-object traversal), and the legacy `from_env_file`
alias. Missing environment variables differ from present-empty values. Registries
are caller-owned; applications register additional typed services without changing
the parser. No shell evaluation, scripting, network service, or mutable global
registry is required. See the [executable language examples](https://github.com/arran4/gogenconf/blob/main/example_test.go).

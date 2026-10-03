---
title: "Examples"
weight: 12
---

<!-- Generated from README.md by internal/docgen. DO NOT EDIT. -->

## Runnable examples

Run `go run ./examples/<name>` from this checkout; all examples are executed in CI:

- [basic](https://github.com/arran4/gogenconf/blob/main/examples/basic/main.go): parse, format, and explicit typed resolution.
- [nested-sources](https://github.com/arran4/gogenconf/blob/main/examples/nested-sources/main.go): env→file and exact string/bytes.
- [migration](https://github.com/arran4/gogenconf/blob/main/examples/migration/main.go): unresolved rename and enrichment preserving custom content.
- [provider](https://github.com/arran4/gogenconf/blob/main/examples/provider/main.go): deferred file content created after provider construction, then reloaded.
- [codegen](https://github.com/arran4/gogenconf/blob/main/examples/codegen): application schema, local generator, concrete Config and separate binder.

The integration suite copies codegen sources (not generated output) to a temporary
outside Go module, regenerates, compiles and executes it. A disposable local
replace tests the candidate library; published consumers use a normal module
version. Another test installs the CLI and exercises stdin, help, version,
canonicalization, validation, write permissions, and the compressed manual.

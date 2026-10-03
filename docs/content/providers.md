---
title: "Providers"
weight: 9
---

<!-- Generated from README.md by internal/docgen. DO NOT EDIT. -->

## Sensitive inspection and providers

`Schema.Inspect` and `InspectionDocument` use `Sensitive` metadata to redact literals
and calls without an explicit display-safety contract. `StandardDisplayPolicy`
permits known source-only declarations without resolving them. Registering a custom
resolver does **not** authorize displaying its arguments. Underlying document
formatting preserves authored content; inspection is a separate redacted view.

Most fields resolve eagerly into ordinary Go values. A field with
`Policy: gogenconf.ProviderBacked` instead generates `gogenconf.Provider[T]`.
Binding captures the expression without resolving it; `Resolve(ctx)` later reads
the source, including later file changes. Document edits cannot retarget the
captured declaration. Caching, invalidation, and content lifecycle remain outside
the core; adapters can wrap the small interface. See the [compiled provider
example](https://github.com/arran4/gogenconf/blob/main/codegen/internal/testfixture/example_test.go).

For a large template or refreshable local content file, choose ProviderBacked
on that schema field. Bind the ordinary endpoint eagerly while retaining
`Content gogenconf.Provider[[]byte]`. Later application code calls
`cfg.Service.Content.Resolve(ctx)`; it sees neither Document nor Expr. Each call
can read changed file data; environment arguments can also change at resolution
time. Only the declaration is snapshotted, not its source environment or bytes.

Inspection and persistence are deliberately different: `InspectionDocument` is
a redacted display copy, not a round-trip save representation. Formatting the
original document preserves even authored sensitive literals. A display-safe
source contract describes argument roles (locator versus secret content); an
unknown custom call fails closed even if its name begins with `from_`.
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

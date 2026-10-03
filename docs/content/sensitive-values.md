---
title: "Sensitive Values"
weight: 8
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

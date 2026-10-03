---
title: "Migrations and Enrichment"
weight: 7
---

<!-- Generated from README.md by internal/docgen. DO NOT EDIT. -->

## Migration walkthrough

The [migration example](https://github.com/arran4/gogenconf/blob/main/examples/migration/main.go) registers `v1 → v2` and renames
`old_endpoint` on the unresolved AST. Then enrichment adds a default and docs:

```text
# Before (v1)
section service
    # Operator choice
    old_endpoint https://private.example.test
    extension retain
end

# After explicit migration and enrichment (v2)
section service
    # Operator choice
    extension retain
    ## Current endpoint documentation.
    endpoint https://private.example.test
    retries 3
end
```

Each real document also carries its root `config_version`. Register one step per
consecutive version with `Migrations.Register(from, func(*Document) error)` and
call `Apply(doc, target)`. Each successful step commits its cloned document and
version; a failed step leaves that step's input untouched. Earlier successful
steps remain committed. Wrap the whole operation in your own document clone if
you require all-or-nothing across several steps. Migrations never need resolution;
application migration functions must not perform source I/O. Enrichment does not
guess renames or removals, and neither operation silently writes a file.
## Defaults, examples, comments, and evolution

- `Default` is an unresolved expression inserted by `Schema.Seed`.
  `Examples` are documentation only. `Schema.Sample` illustrates optional fields
  and repeated sections as inactive comments; it never activates examples.
- `Schema.Enrich` adds missing fields/defaults and sections, refreshes managed
  documentation, and preserves explicit values, user comments, and unknown
  content. A changed default does not overwrite an existing override. Enrichment
  is idempotent; the [evolution goldens](https://github.com/arran4/gogenconf/blob/main/testdata/evolution) make this visible.
- `#` is user-owned and `##` is managed documentation in versioned v1+ files.
  All unversioned/v0 comments are user-owned, even `##` or `###` headings. On
  upgrade, `## Heading` becomes `# # Heading` so a reload cannot claim it as managed.
- Registered consecutive `Migrations` operate transactionally on unresolved
  documents. They advance versions only on success; missing paths and unsupported
  future versions fail. Applications own semantic renames and compatibility
  transformations. Parsing and loading never implicitly save a file.
- `InputOnly` fields remain available through `Schema.Field` for compatibility,
  migration, inspection, and deliberate removal, but `WritableField` excludes them
  from normal editors. They generate no runtime member. `RuntimeOnly` does the
  converse: a concrete application member with no persisted input.
- `Required`, `Sensitive`, repeated sections, and eager/provider policy are explicit
  schema metadata, not inferred from the Go type.

For example, a credential field can have no Default but have an Example
`from_file(from_env(CREDENTIAL_FILE))`. `Seed()` leaves the field absent;
`Sample()` emits an inactive `##   credential from_file(...)` illustration.
`Enrich()` does not activate that example. Conversely, `Default: Literal{"3"}`
for retries inserts an active value when missing. Explicit values always win
over a changed schema default. Managed documentation is associated with fields
and section blocks; user comments never become schema property merely because
their prose resembles documentation.

`Schema` declares Version and ordered Sections. `SectionDefinition` gives Name,
GoName/GoType, Documentation, Fields, Repeated and ExampleNames. `Field` carries
Key, GoName/GoType, Default, Examples, Documentation, Required, Sensitive,
InputOnly, RuntimeOnly, Deprecated, Policy and legacy Environment fallback metadata.
Deprecated suppresses new sample/enrichment entries; InputOnly additionally blocks
normal editing/runtime targets. Required checks missing declarations, not arbitrary
application-specific value validation. Sensitive is inspection/error policy, not
encryption at rest. Applications must validate their domain constraints after binding.

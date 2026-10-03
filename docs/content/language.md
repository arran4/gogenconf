---
title: "Configuration Language"
weight: 3
---

<!-- Generated from README.md by internal/docgen. DO NOT EDIT. -->

## Native language reference

| Construct | Meaning |
| --- | --- |
| `config_version 1` | One nonnegative integer at document root; omitted means legacy v0. Applications reject unsupported versions. |
| `section service` … `end` | Named section. Nesting and duplicate section names are rejected. |
| `section remote west` | Named instance; a schema may mark `remote` as repeated. |
| `endpoint https://example.test` | Entry key plus literal. Duplicate keys within a section fail. |
| `label "two words"` | Go-style quoted string with escapes such as `\n`, `\"`, `\\`, `\x00`. |
| `from_file(from_env(FILE))` | Recursive call; whitespace around arguments/delimiters is allowed. |
| `from_json_file(config.json, .auth.token)` | Multiple arguments; dotted object lookup, not a query language. |
| `# user note` / `## schema docs` | Persisted ownership distinction for versioned v1+ files. |

Comments occupy whole lines; inline `#` is not a comment delimiter. Indentation
does not affect meaning. Useful blank lines and ordering are retained. Unknown
fields/sections are ordinary nodes, not silently discarded. A legacy key with no
value is not an override; use `""` for an explicit empty string. Quote punctuation
inside call arguments. A top-level bare comma remains literal for compatibility.
Canonical formatting may change unnecessary quoting but preserves expressions.

| Source | Arguments and results |
| --- | --- |
| literal | Implicit constant; string or exact []byte. |
| `from_env(NAME)` | Name resolves as string; missing is an error, present empty is valid. String/[]byte results. |
| `from_file(path)` | Path resolves as string. Exact string/[]byte, no trimming. |
| `from_env_file(NAME)` | Compatibility alias; prefer nested file-from-env for new declarations. |
| `from_json_file(path, .object.key)` | File path/selector resolve as strings; selected JSON values resolve as string/[]byte. |

Register application declarers with `registry.Register(name, resultTypeExample,
resolver)`. Duplicate name/type registrations fail; the same name can support
different types. Unknown names and unsupported requested types fail explicitly.
Outer resolvers choose argument types; the parser needs no changes.
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

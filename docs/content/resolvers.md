---
title: "Resolvers and Declarers"
weight: 6
---

<!-- Generated from README.md by internal/docgen. DO NOT EDIT. -->

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

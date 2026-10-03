---
title: "gogenconf Manual"
weight: 0
---

<!-- Generated from README.md by internal/docgen. DO NOT EDIT. -->

# gogenconf

A versioned declarative configuration format and schema system with generated
strongly typed Go bindings. It is a configuration language and evolution model,
not merely a file parser or struct generator.

Newly extracted; API stability is not yet promised. Go 1.25 or later is required.
The module currently uses only the standard library.

## Contents

- [Purpose and boundaries](#purpose-and-boundaries)
- [Quick start](#quick-start)
- [Configuration stays declarative](#configuration-stays-declarative)
- [Native language reference](#native-language-reference)
- [Application schema and generated Go](#application-schema-and-generated-go)
- [Defaults, examples, comments, and evolution](#defaults-examples-comments-and-evolution)
- [Migration walkthrough](#migration-walkthrough)
- [Sensitive inspection and providers](#sensitive-inspection-and-providers)
- [CLI reference](#cli-reference)
- [Installation and man pages](#installation-and-man-pages)
- [Runnable examples](#runnable-examples)
- [Packages and development](#packages-and-development)
- [Release and recovery](#release-and-recovery)
- [Provenance and stability](#provenance-and-stability)
- [License](#license)

## Purpose and boundaries

A configuration **language** describes syntax. Its **document AST** preserves
what the operator wrote. An application **schema** describes fields, types,
defaults and documentation. **Resolution** interprets a declaration using typed
services. **Code generation** emits a static binder and the ordinary Go types
the application's runtime consumes. These are separate layers.

This avoids naive string maps and eager parsing that lose `from_env(...)` or
`from_file(...)` provenance. Applications need a semantic `Credential []byte`,
not proliferating CredentialFile/CredentialEnv fields. Schema-owned defaults,
documentation and samples reduce drift. Explicit migrations prevent accidental
renames; document-first editing preserves customization. Inspection need not
fetch secrets, and normal field binding requires no runtime reflection.

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

## Application schema and generated Go

Applications define content in Go; the native language does not know their fields:

```go
schema := gogenconf.Schema{
    Version: 1,
    Sections: []gogenconf.SectionDefinition{{
        Name: "service", GoName: "Service", GoType: "ServiceConfig",
        Fields: []gogenconf.Field{
            {Key: "endpoint", GoName: "Endpoint", GoType: "string",
                Default: gogenconf.Literal{Value: "https://example.test"},
                Documentation: []string{"Service endpoint."}},
            {Key: "credential", GoName: "Credential", GoType: "[]byte",
                Sensitive: true, Required: true,
                Examples: []gogenconf.Expr{gogenconf.Call{
                    Name: "from_env", Args: []gogenconf.Expr{
                        gogenconf.Literal{Value: "CREDENTIAL"},
                    },
                }}},
        },
    }},
}
```

An application-local `go:generate` driver writes the outputs of:

```go
model, err := codegen.GenerateModel(schema, codegen.Options{Package: "appconfig"})
// Check err and write model to the application's model package.
binder, err := codegen.GenerateBinder(schema, codegen.Options{
    Package: "configbinding", ConfigImport: "example.org/myapp/appconfig",
})
// Check err and write binder to the application's binding package.
```

`ConfigImport` names the application model package. The optional `LibraryImport`
defaults to `github.com/arran4/gogenconf`. Generated files carry the canonical
`Code generated by gogenconf. DO NOT EDIT.` header. The CLI is language tooling;
generation uses an application-local driver, not runtime schema/plugin loading.

For example, put `//go:generate go run ./internal/generate` in your application
package. That driver imports your Go schema and `gogenconf/codegen`, checks errors,
and writes model/binder files. The [runnable driver](https://github.com/arran4/gogenconf/blob/main/examples/codegen/internal/generate/main.go)
demonstrates this without installing a generator binary. Arbitrary Go schema
values cannot safely be dynamically discovered by a generic CLI.

The generated model is concrete application-owned Go, not aliases or AST fields:

```go
type Config struct { Service ServiceConfig }
type ServiceConfig struct {
    Endpoint   string
    Credential []byte
}
```

The generated binder's `Resolve(ctx, doc, registry, lookup)` returns that Config
using explicit typed calls, with no reflection-based struct population. The
optional `lookup` supplies legacy environment fallbacks described by the schema.
Authored values take precedence over environment fallbacks, then schema defaults;
transient defaults inserted by enrichment retain that distinction until persisted.
Supported static mappings are deliberately narrow: `string`, `[]byte`, `[]string`
(the application registers its list semantics), and runtime-only `bool` fields.
Invalid/ambiguous mappings fail generation or compilation.

```text
config file → Document / Expr → Schema + migrations + enrichment
                                      ↓
                               generated binder
                                      ↓
                          application-owned Go Config
```

This flow is one-way. Edit and persist the original Document; resolved values have
lost provenance and cannot be converted back through a Config-to-Document API.

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

## CLI reference

The CLI is schema-independent and never resolves declarations. Every command
supports `--help`/`-h`; root help describes the architecture and documentation.

| Command | Arguments / flags | Behavior |
| --- | --- | --- |
| `gogenconf format [FILE]` | FILE defaults to `-` (stdin); `--write`, `--check` default false | Canonical stdout, atomic in-place write, or noncanonical check. |
| `gogenconf validate [FILE]` | FILE defaults to stdin | Syntax/structure only, silent success. |
| `gogenconf expr format [EXPRESSION]` | Omitted or `-` reads stdin | Canonical expression, no resolution. |
| `gogenconf expr validate [EXPRESSION]` | Omitted or `-` reads stdin | Expression grammar only, silent success. |
| `gogenconf version` | No flags | Build version, commit, and date; dev/none/unknown for development builds. |

```sh
gogenconf format service.conf
cat service.conf | gogenconf format
gogenconf format --check service.conf
gogenconf format --write service.conf
gogenconf validate service.conf
gogenconf expr format 'from_file(from_env(CREDENTIAL_FILE))'
gogenconf expr validate 'from_json_file(from_env(CONFIG_FILE), .auth.token)'
```

`--write` requires a regular input file, rejects symlinks/stdin, preserves its
permissions and uses a same-directory atomic replacement. `--check` and `--write`
are mutually exclusive. Syntax errors, I/O errors and noncanonical checks exit
nonzero. Do not pass secret literal contents on argv. Formatting is not redaction.
`validate` cannot check application fields, resolver registration, required values,
or supported schema versions; those require application schema/binding APIs.

CLI code, extended help, and per-command man pages are generated by pinned
go-subcommand **v0.0.30** from `internal/cli/commands.go`. The local `internal/cligen`
driver disables volatile provenance and the generated unpinned regeneration
shortcut via a main-template overlay. It aggregates generated command pages/help
into `gogenconf(1)` with deterministic gzip output. No go-subcommand runtime
dependency enters either the CLI or library.

## Installation and man pages

Tagged releases are pending. Until merge/release, use the extraction PR's public
commit in place of `REV`; after merge `main` is also available:

```sh
go get github.com/arran4/gogenconf@REV
go install github.com/arran4/gogenconf/cmd/gogenconf@REV
```

Once releases exist, the usual forms are:

```sh
go get github.com/arran4/gogenconf
go install github.com/arran4/gogenconf/cmd/gogenconf@latest
```

The release pipeline is configured for Linux/macOS/Windows on amd64/arm64,
tar.gz/Windows zip archives, checksums, and Linux deb/rpm/apk packages. Releases
will include README, license and man pages. None has been published by this PR.

```sh
gzip -dc man/man1/gogenconf.1.gz | man -l -
# Optional system installation from a checkout or release archive:
sudo install -Dm644 man/man1/gogenconf.1.gz /usr/local/share/man/man1/gogenconf.1.gz
man gogenconf
```

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

## Packages and development

- `github.com/arran4/gogenconf`: native format, ASTs, schema, migrations/enrichment,
  inspection, registry/resolvers, providers, and binding helpers.
- `github.com/arran4/gogenconf/codegen`: static model/binder generation.
- Test schemas, compiled generated fixtures, and their regeneration driver are
  private under `codegen/internal`, not supported application APIs.

```sh
go generate ./...
gofmt -w .
go vet ./...
go test -count=1 ./...
go test -race ./...
go build ./cmd/gogenconf
go mod tidy
git diff --check
hugo --source docs --minify
```

The fixture test checks deterministic regeneration. CI also requires a clean
generation/tidy diff. Repeat `go generate ./...` and require no diff before a PR.
README is the primary manual: `internal/docgen` generates navigable Hugo pages
from it, so edit README rather than generated site content. The site uses local
layouts, no vendored theme. CLI help/man metadata stays in command comments.
CI runs lint, vet, all examples/integration/race tests, deterministic generation,
GoReleaser config checks, Hugo build and Pages deployment for validated main.

## Release and recovery

Do not create a tag manually. After the standalone PR merges, the release operator
uses the [first-release issue](https://github.com/arran4/gogenconf/issues/2).
In Actions, dispatch **CI/CD** at main with a `release-major`, `release-minor`,
`release-patch`, `release-test`, `release-rc`, or `release-alpha` mode. For the first
release choose a **pre-v1** candidate, preferably alpha/test, with an explicit
`release_version_override` if necessary. This PR creates no release or tag.

All code/docs/generation gates run first. `arran4/git-tag-inc-action@v1` selects a
version when no override is supplied. The tag script rejects dirty checkouts and
conflicting tags, verifies the exact validated commit, and safely reuses an
existing same-commit tag (including annotated tags). It never force-pushes.
The workflow explicitly dispatches `publish-tag` at the tag because GITHUB_TOKEN
tag pushes do not trigger another push workflow. External v-tag pushes also take
the validation/publication path. GoReleaser v2 builds archives, checksums, packages
and GitHub release notes; prerelease suffixes are recognized automatically.

For a failed tag/publication run, retain the validated commit. Retry release mode
with the **same explicit version override**, or dispatch `publish-tag` on that tag.
A conflicting tag is an error to investigate, not permission to move it. If a
partial GitHub Release already exists, inspect its assets/logs before retrying;
do not silently delete published artifacts. No Docker or token-dependent tap
publishing is required. CLI version/commit/date are injected using GoReleaser's
`-X main.version`, `-X main.commit`, `-X main.date` flags.

Actions account credit/capacity failures are infrastructure limitations. Record
local gate results accurately, but actual CI release execution requires restored
capacity; do not make empty commits to retrigger billing failures.

## Provenance and stability

This pre-v1 project was extracted from the architecture proven in Address PR #52,
merge `7ca592d9aae476169c568d0bd4ce75654a099e48`, with subtree history preserved.
See [PROVENANCE.md](https://github.com/arran4/gogenconf/blob/main/PROVENANCE.md). Application-specific schemas and policy remain
outside this library. API stability is not yet promised; review migrations and
pin tested versions. No future declarers or release artifacts are implied to exist.

## License

GPL-3.0-only; see [LICENSE](https://github.com/arran4/gogenconf/blob/main/LICENSE). Copyright (c) 2026 Arran Ubels.

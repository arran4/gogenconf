---
title: "Introduction"
weight: 1
---

<!-- Generated from README.md by internal/docgen. DO NOT EDIT. -->

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
## Provenance and stability

This pre-v1 project was extracted from the architecture proven in Address PR #52,
merge `7ca592d9aae476169c568d0bd4ce75654a099e48`, with subtree history preserved.
See [PROVENANCE.md](https://github.com/arran4/gogenconf/blob/main/PROVENANCE.md). Application-specific schemas and policy remain
outside this library. API stability is not yet promised; review migrations and
pin tested versions. No future declarers or release artifacts are implied to exist.

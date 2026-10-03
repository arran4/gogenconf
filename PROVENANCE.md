# Extraction provenance

Extracted from the generic `internal/configmodel` subtree of
[`arran4/address` PR #52](https://github.com/arran4/address/pull/52), as merged at
`7ca592d9aae476169c568d0bd4ce75654a099e48`.

In a clean clone at that authoritative main commit:

```sh
git subtree split --prefix=internal/configmodel -b gogenconf-extract
```

This produced split tip `7d6155e311fe8eb5fe04256460327757024cf84b`.
The extracted history was fetched locally into a branch based on gogenconf/main
and merged with `--allow-unrelated-histories`. Original subtree commits remain
ancestors; neither repository's published history was rewritten.

The adaptation renames the root package, updates import/header identity, makes
codegen option names public-facing, and places test support under `codegen/internal`.
Application-specific schemas, compatibility policies, migration goldens, security
policy, runtime types/binders, and CLI remain in the originating application.

The new public module uses GPL version 3 by the owner's explicit instruction.
The originating application's private/non-distributable license was not copied.

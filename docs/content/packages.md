---
title: "Go API and Package Guide"
weight: 11
---

<!-- Generated from README.md by internal/docgen. DO NOT EDIT. -->

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

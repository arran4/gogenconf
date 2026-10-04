# Generated artifact policy

| Path | Producer | Class | Tracked | Regenerate |
| --- | --- | --- | --- | --- |
| `cmd/`, `cmd/errors.go`, `cmd/agents.md` | pinned go-subcommand | production self-hosting | yes | `go generate ./...` |
| `internal/nativeconfig/` | `internal/runtimegen` | production self-hosting | yes | `go generate ./...` |
| `examples/generated/**` | each example's local driver | runnable examples | yes | `go generate ./...` |
| `codegen/testdata/*.txtar` | generator tests | expected regression output | yes | `go test ./codegen -run TestGenerationFixtures -update` |
| `docs/content/` | `internal/docgen` | documentation build output | no | `go run ./internal/docgen` |
| `man/man1/gogenconf.1.gz` | `internal/cligen` | distribution build output | no | `go run ./internal/cligen` |
| `internal/cligen/man/` | go-subcommand | intermediate staging | no | temporary only |

The CI docs job generates documentation before Hugo. The release job generates
the compressed manual before GoReleaser packages it. Generated files are never
hand edited.

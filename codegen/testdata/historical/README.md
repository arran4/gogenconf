# Frozen Historical Baseline Fixture

This directory contains an isolated, hermetic snapshot of `arran4/gogenconf` at pinned revision [`d000175b840edf42166bf2f441b7dbce3153a7de`](https://github.com/arran4/gogenconf/commit/d000175b840edf42166bf2f441b7dbce3153a7de).

## Provenance & Purpose

- **Pinned Commit:** `d000175b840edf42166bf2f441b7dbce3153a7de` (PR #47 head / `lane-c/schema-docs`, commit immediately preceding PR #51 / Issue #49; `repo/document.go` formatted with `gofmt`).
- **Origin:** Upstream repository `github.com/arran4/gogenconf`.
- **License:** Apache License 2.0 (identical to parent repository `arran4/gogenconf`).
- **Purpose:** Serves as a hermetic, immutable baseline for:
  1. Differential testing in `codegen/differential_test.go` to guarantee strict behavioral parity between historical reflection-based binding and the new static decoder architecture.
  2. Benchmarking in `codegen/testdata/benchmark` to compare actual generated historical artifacts against modern static decoder outputs on identical schemas and inputs.
- **Reproducibility:** Extracted directly from git history:
  ```bash
  git archive d000175b840edf42166bf2f441b7dbce3153a7de | tar -x -C codegen/testdata/historical/repo
  ```
  Non-essential documentation and test assets were pruned to eliminate redundant vendoring while preserving all core runtime resolution (`resolve.go`, `document.go`, `binding.go`) and pre-#51 generated examples (`examples/generated/basic` and `examples/generated/repeated`).

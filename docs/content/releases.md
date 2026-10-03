---
title: "Release and Installation"
weight: 13
---

<!-- Generated from README.md by internal/docgen. DO NOT EDIT. -->

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

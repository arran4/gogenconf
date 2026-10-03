#!/usr/bin/env bash
# Only called after validation gates. Never moves an existing tag.
set -euo pipefail
tag=${1:?tag required}
target=${2:?validated commit required}
[[ "$tag" =~ ^v[0-9]+\.[0-9]+\.[0-9]+(-[0-9A-Za-z]+(\.[0-9A-Za-z]+)*)?$ ]] || { echo 'Invalid semantic version tag' >&2; exit 1; }
[[ -z $(git status --porcelain) ]] || { echo 'Refusing to tag a dirty worktree' >&2; exit 1; }
target=$(git rev-parse "$target^{commit}")
[[ $(git rev-parse HEAD) == "$target" ]] || { echo 'Checkout is not the validated commit' >&2; exit 1; }
remote_commit() {
  git ls-remote origin "refs/tags/$tag" "refs/tags/$tag^{}" | awk '
    /\^\{\}$/ { peeled=$1 }
    !/\^\{\}$/ { direct=$1 }
    END { if (peeled != "") print peeled; else print direct }'
}
existing=$(remote_commit)
if [[ -n "$existing" ]]; then
  [[ "$existing" == "$target" ]] || { echo 'Existing tag conflicts with validated commit' >&2; exit 1; }
  echo "Recovering existing correct tag $tag"
  exit 0
fi
if ! git push origin "$target:refs/tags/$tag"; then
  [[ $(remote_commit) == "$target" ]] || { echo 'Tag push failed or conflicted' >&2; exit 1; }
fi

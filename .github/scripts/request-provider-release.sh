#!/usr/bin/env bash
set -euo pipefail

repository=${REPOSITORY:?Use make release-provider REPOSITORY=certimate-go/plugin-provider-<name> VERSION=<major.minor.patch>}
version=${VERSION:?Set VERSION to a stable major.minor.patch version}
if [[ ! "$repository" =~ ^certimate-go/plugin-provider-[a-z0-9][a-z0-9_.-]*$ || ! "$version" =~ ^(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)$ ]]; then
    printf 'Expected an official provider repository and a stable version.\n' >&2
    exit 1
fi

remote="https://github.com/$repository.git"
formal="refs/tags/v$version"
request="$formal-detach"

stage=$(mktemp -d)
trap 'rm -rf -- "$stage"' EXIT
git -C "$stage" init --bare --quiet
git -C "$stage" fetch --quiet --depth=1 "$remote" refs/heads/main
source_sha=$(git -C "$stage" rev-parse FETCH_HEAD)
if ! git -C "$stage" show "$source_sha:manifest.json" |
    jq -e --arg version "$version" '.version == $version' >/dev/null; then
    printf '%s main does not declare %s at %s\n' "$repository" "$version" "$source_sha" >&2
    exit 1
fi

published=$(git ls-remote --refs "$remote" "$formal")
if [[ -n "$published" ]]; then
    printf '%s already has formal tag %s\n' "$repository" "${formal#refs/tags/}"
    exit 0
fi

case "${DRY_RUN:-0}" in
    0) git -C "$stage" push "$remote" "$source_sha:$request" ;;
    1) printf 'Preview only. ' ;;
    *) printf 'DRY_RUN must be 0 or 1\n' >&2; exit 1 ;;
esac
printf '%s %s at %s\n' "$repository" "${request#refs/tags/}" "$source_sha"
printf 'https://github.com/%s/actions/workflows/release.yml\n' "$repository"

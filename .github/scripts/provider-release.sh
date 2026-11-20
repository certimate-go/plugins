#!/usr/bin/env bash
set -euo pipefail

controller="${GITHUB_WORKSPACE:?}/controller"
provider="$GITHUB_WORKSPACE/provider-publisher.git"
formal_tag="${TRIGGER_TAG%-detach}"
formal_ref="refs/tags/$formal_tag"

die() {
    printf '%s\n' "$*" >&2
    exit 1
}

[[ "$TRIGGER_TAG" =~ ^v(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)-detach$ && ${#TRIGGER_TAG} -le 72 ]] ||
    die 'Expected a stable release request tag (vX.Y.Z-detach)'
[[ "$GITHUB_REPOSITORY" =~ ^certimate-go/plugin-provider-[a-z0-9][a-z0-9_.-]*$ ]] ||
    die 'Not a Certimate provider repository'

git_provider() { git --no-replace-objects -C "$provider" "$@"; }

controller_cmd() {
    (cd "$controller" && go run -mod=readonly ./cmd/registryctl "$@")
}

fetch_source() {
    git init --bare -q "$provider"
    git_provider remote add origin "https://github.com/$GITHUB_REPOSITORY.git"
    git_provider fetch --no-tags origin refs/heads/main:refs/remotes/origin/main \
        "refs/tags/$TRIGGER_TAG:refs/tags/$TRIGGER_TAG"
    source_sha="$(git_provider rev-parse --verify "refs/tags/$TRIGGER_TAG^{commit}")"
    [[ "$GITHUB_SHA" =~ ^[0-9a-f]{40}$ ]] || die 'Invalid protected branch commit'
    git_provider merge-base --is-ancestor "$GITHUB_SHA" refs/remotes/origin/main ||
        die 'Workflow commit is no longer reachable on the default branch'
    git_provider merge-base --is-ancestor "$source_sha" "$GITHUB_SHA" ||
        die 'Trigger is not reachable from the protected workflow commit'
}

fetch_existing_tag() {
    local result
    if git_provider ls-remote --exit-code --refs origin "$formal_ref" >/dev/null; then
        git_provider fetch --no-tags origin "$formal_ref:$formal_ref" || die 'Unable to fetch the immutable formal tag'
        return 0
    else
        result=$?
        [[ "$result" == 2 ]] || die "Unable to inspect the formal tag (Git status $result)"
        return 1
    fi
}

check_existing_source() {
    local existing
    existing="$(controller_cmd inspect-release -checkout "$provider" -repository "$GITHUB_REPOSITORY" -tag "$formal_tag")"
    jq -e --arg source "$1" '.source_commit == $source' <<<"$existing" >/dev/null ||
        die 'Formal tag already exists for a different source commit'
}

case "${1:-}" in
    prepare)
        fetch_source
        if fetch_existing_tag; then
            check_existing_source "$source_sha"
            printf 'present=true\n' >>"$GITHUB_OUTPUT"
        else
            printf 'present=false\n' >>"$GITHUB_OUTPUT"
        fi
        printf 'source=%s\n' "$source_sha" >>"$GITHUB_OUTPUT"
        ;;
    stage)
        fetch_source
        [[ "$SOURCE_SHA" == "$source_sha" ]] || die 'Release request tag moved during the build'
        fetch_existing_tag || [[ "$?" == 1 ]]
        staged="$(controller_cmd stage-release -checkout "$provider" -artifacts "$GITHUB_WORKSPACE/validated-output" \
            -repository "$GITHUB_REPOSITORY" -tag "$formal_tag" -source-sha "$source_sha" -run-id "$GITHUB_RUN_ID")"
        printf 'present=%s\nrelease=%s\n' "$(jq -r '.already_tagged' <<<"$staged")" "$(jq -er '.release_commit' <<<"$staged")" >>"$GITHUB_OUTPUT"
        ;;
    push)
        [[ "$RELEASE_SHA" =~ ^[0-9a-f]{40}$ && "$SOURCE_SHA" =~ ^[0-9a-f]{40}$ && -n "$GH_TOKEN" ]] ||
            die 'Missing release commit, source commit, or repository token'
        if GIT_CONFIG_COUNT=1 GIT_CONFIG_KEY_0=credential.https://github.com.helper \
            GIT_CONFIG_VALUE_0='!gh auth git-credential' git_provider push origin "$RELEASE_SHA:$formal_ref"; then
            printf 'Published %s from %s (run %s)\n' "$formal_tag" "$SOURCE_SHA" "$GITHUB_RUN_ID"
        elif fetch_existing_tag; then
            check_existing_source "$SOURCE_SHA"
            printf 'Another run already published %s from %s\n' "$formal_tag" "$SOURCE_SHA"
        else
            die 'Failed to create formal tag'
        fi
        ;;
    *)
        die 'Usage: provider-release.sh prepare|stage|push'
        ;;
esac

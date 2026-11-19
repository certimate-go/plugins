#!/usr/bin/env bash
set -euo pipefail

controller="${GITHUB_WORKSPACE:?}/controller"
provider="$GITHUB_WORKSPACE/provider-publisher.git"
formal_tag="${TRIGGER_TAG%-detach}"
formal_ref="refs/tags/$formal_tag"
provider_repo="${PROVIDER_REPOSITORY:-$GITHUB_REPOSITORY}"

die() {
    printf '%s\n' "$*" >&2
    exit 1
}

[[ "$TRIGGER_TAG" =~ ^v(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)-detach$ && ${#TRIGGER_TAG} -le 72 ]] ||
    die 'Expected a stable release request tag (vX.Y.Z-detach)'
[[ "$provider_repo" =~ ^certimate-go/plugin-provider-[a-z0-9][a-z0-9_.-]*$ ]] ||
    die 'Not a Certimate provider repository'
provider_type="$(jq -er --arg repo "$provider_repo" '
    to_entries | map(select(.value == $repo) | .key) | select(length == 1) | .[0]
    | select(test("^[a-z0-9][a-z0-9_.-]*$"))' "$controller/registry/_policy/admissions.json")" ||
    die 'Provider repository is not admitted'

git_provider() { git --no-replace-objects -C "$provider" "$@"; }

controller_cmd() {
    (cd "$controller" && go run -mod=readonly ./cmd/registryctl "$@")
}

fetch_source() {
    git init --bare -q "$provider"
    git_provider remote add origin "https://github.com/$provider_repo.git"
    git_provider fetch --no-tags origin refs/heads/main:refs/remotes/origin/main \
        "refs/tags/$TRIGGER_TAG:refs/tags/$TRIGGER_TAG"
    source_sha="$(git_provider rev-parse --verify "refs/tags/$TRIGGER_TAG^{commit}")"
    local protected_source="$GITHUB_SHA"
    if [[ "$GITHUB_REPOSITORY" == certimate-go/plugins ]]; then
        protected_source="$(gh api "repos/$provider_repo/branches/main" --jq 'select(.name == "main" and .protected == true).commit.sha')"
        [[ "$protected_source" == "$(git_provider rev-parse refs/remotes/origin/main)" ]] ||
            die 'Provider main changed during verification or is not protected'
    fi
    [[ "$protected_source" =~ ^[0-9a-f]{40}$ ]] || die 'Invalid protected branch commit'
    git_provider merge-base --is-ancestor "$protected_source" refs/remotes/origin/main ||
        die 'Verified commit is no longer reachable on the default branch'
    git_provider merge-base --is-ancestor "$source_sha" "$protected_source" ||
        die 'Trigger is not reachable from the protected provider branch'
    git_provider show "$source_sha:manifest.json" | jq -e --arg provider "$provider_type" '.provider_type == $provider' >/dev/null ||
        die 'Source manifest plugin ID does not match the repository admission'
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
    existing="$(controller_cmd inspect-release -checkout "$provider" -repository "$provider_repo" -tag "$formal_tag")"
    jq -e --arg source "$1" --arg provider "$provider_type" '.source_commit == $source and .provider_type == $provider' <<<"$existing" >/dev/null ||
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
            -repository "$provider_repo" -tag "$formal_tag" -source-sha "$source_sha" -run-id "$GITHUB_RUN_ID" -build-repository "${BUILD_REPOSITORY:-}")"
        printf 'present=%s\nrelease=%s\nrepository_name=%s\n' \
            "$(jq -r '.already_tagged' <<<"$staged")" "$(jq -er '.release_commit' <<<"$staged")" "${provider_repo#certimate-go/}" >>"$GITHUB_OUTPUT"
        ;;
    push)
        [[ "$RELEASE_SHA" =~ ^[0-9a-f]{40}$ && "$SOURCE_SHA" =~ ^[0-9a-f]{40}$ && -n "$GH_TOKEN" ]] ||
            die 'Missing checked release commit, source, or App token'
        source_ref="refs/tags/$TRIGGER_TAG"
        git_provider update-ref -d "$source_ref"
        git_provider fetch --no-tags origin "$source_ref:$source_ref"
        [[ "$(git_provider rev-parse --verify "$source_ref^{commit}")" == "$SOURCE_SHA" ]] ||
            die 'Release request tag moved after staging'
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

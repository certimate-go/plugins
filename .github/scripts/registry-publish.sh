#!/usr/bin/env bash
set -euo pipefail

die() {
    printf '%s\n' "$*" >&2
    exit 1
}

[[ "$GITHUB_REPOSITORY" == certimate-go/plugins && "$GITHUB_SHA" =~ ^[0-9a-f]{40}$ ]] || die 'Expected protected registry checkout'
[[ -n "$PUBLIC_KEYS" ]] || die 'Index trust keys are not configured'
tools="$RUNNER_TEMP/registryctl"
main_ref=refs/heads/main

remote_ref() {
    local response code
    if response="$(git ls-remote --exit-code --refs origin "$1")"; then
        printf '%s\n' "${response%%$'\t'*}"
    else
        code=$?
        [[ "$code" == 2 ]] || return "$code"
    fi
}

require_current_source() {
    [[ "$(git rev-parse --verify HEAD)" == "$GITHUB_SHA" &&
       "$(remote_ref "$main_ref")" == "$GITHUB_SHA" ]] || die 'Registry main advanced; rerun from the latest protected main'
}

require_legacy_release_stopped() {
    local legacy_file workflow state status count
    legacy_file="$(git --no-replace-objects ls-tree --name-only "$GITHUB_SHA" -- .github/workflows/release.yml)"
    [[ -n "$legacy_file" ]] || return 0
    [[ -n "${REGISTRY_ACTIONS_TOKEN:-}" ]] || die 'Missing read-only Actions token for the legacy release check'
    workflow="repos/$GITHUB_REPOSITORY/actions/workflows/release.yml"
    state="$(GH_TOKEN="$REGISTRY_ACTIONS_TOKEN" gh api "$workflow" --jq .state)" || die 'Cannot read the legacy release workflow state'
    [[ "$state" == disabled_manually ]] || die "Disable the legacy release workflow before publishing (current state: $state)"
    for status in queued in_progress waiting requested pending; do
        count="$(GH_TOKEN="$REGISTRY_ACTIONS_TOKEN" gh api "$workflow/runs?status=$status&per_page=1" --jq .total_count)" ||
            die "Cannot check legacy release runs in state $status"
        [[ "$count" == 0 ]] || die "Wait for legacy release runs to finish ($status: $count)"
    done
}

working="$(mktemp -d)"
trap 'rm -f -- "$working/seed" "$working/keys.json"; rmdir -- "$working"' EXIT
umask 077
printf '%s\n' "$PUBLIC_KEYS" >"$working/keys.json"

case "${1:-}" in
    stage)
        signing_seed="${INDEX_SEED:-}"
        unset INDEX_SEED
        require_current_source
        require_legacy_release_stopped
        [[ -n "$signing_seed" ]] || die 'Missing protected index signing seed'
        printf '%s' "$signing_seed" >"$working/seed"
        unset signing_seed
        publication="$("$tools" stage-publication -source-sha "$GITHUB_SHA" -market-sha "$GITHUB_SHA" \
            -key-id "$INDEX_KEY_ID" -epoch "$INDEX_EPOCH" -public-keys "$working/keys.json" -key-file "$working/seed" \
            -lifetime "$INDEX_LIFETIME" -renew-before "$INDEX_RENEW_BEFORE")"
        printf 'market_commit=%s\nchanged=%s\nepoch=%s\ngeneration=%s\n' \
            "$(jq -er .commit <<<"$publication")" "$(jq -r .changed <<<"$publication")" \
            "$(jq -er .epoch <<<"$publication")" "$(jq -er .generation <<<"$publication")" >>"$GITHUB_OUTPUT"
        jq -c '{commit,source_commit,key_id,epoch,generation,expires_at,changed,pending}' <<<"$publication"
        ;;
    propose)
        [[ "$MARKET_COMMIT" =~ ^[0-9a-f]{40}$ && "$INDEX_EPOCH" =~ ^[1-9][0-9]*$ &&
           "$INDEX_GENERATION" =~ ^[1-9][0-9]*$ && -n "$GH_TOKEN" ]] || die 'Missing staged snapshot or release App token'
        require_current_source
        require_legacy_release_stopped
        branch="registry/_publish/$GITHUB_SHA/$INDEX_EPOCH-$INDEX_GENERATION"
        branch_ref="refs/heads/$branch"
        current="$(remote_ref "$branch_ref")"
        if [[ -z "$current" ]]; then
            actual_parents="$(git --no-replace-objects rev-list -n 1 --parents "$MARKET_COMMIT")"
            [[ "$actual_parents" == "$MARKET_COMMIT $GITHUB_SHA" ]] || die 'Snapshot is not a child of the protected source'
            GIT_CONFIG_COUNT=1 GIT_CONFIG_KEY_0=credential.https://github.com.helper GIT_CONFIG_VALUE_0='!gh auth git-credential' \
                git push origin "$MARKET_COMMIT:$branch_ref"
            current="$MARKET_COMMIT"
        else
            [[ "$current" =~ ^[0-9a-f]{40}$ ]] || die 'Invalid publication branch commit'
            git fetch --no-tags origin "$branch_ref"
            [[ "$(git rev-parse --verify 'FETCH_HEAD^{commit}')" == "$current" ]] || die 'Publication branch changed'
        fi
        "$tools" registration-changes -base "$GITHUB_SHA" -candidate "$current" -public-keys "$working/keys.json" |
            jq -e 'length == 0' >/dev/null
        pull="$(gh api "repos/$GITHUB_REPOSITORY/pulls?state=open&head=certimate-go:$branch&base=main")"
        pull_id="$(jq -er --arg sha "$current" '
            if length == 0 then "" else
            .[0] | select(.head.sha == $sha and .user.login == "certimate-plugin-dispatcher[bot]") | .number
            end' <<<"$pull")" || die 'Existing publication pull request has an unexpected author or head'
        if [[ -z "$pull_id" ]]; then
            pull_id="$(gh api --method POST "repos/$GITHUB_REPOSITORY/pulls" \
                -f "title=Publish signed market index $INDEX_EPOCH/$INDEX_GENERATION" -f 'base=main' -f "head=$branch" \
                -f "body=Signed catalog from protected source $GITHUB_SHA." --jq .number)"
        fi
        [[ "$pull_id" =~ ^[1-9][0-9]*$ ]] || die 'Invalid publication pull request number'
        printf '### Signed index awaiting merge\n\n%s/%s/pull/%s\n' \
            "$GITHUB_SERVER_URL" "$GITHUB_REPOSITORY" "$pull_id" >>"$GITHUB_STEP_SUMMARY"
        GH_TOKEN="$REGISTRY_ACTIONS_TOKEN" gh workflow run registry-reconcile.yml --repo "$GITHUB_REPOSITORY" --ref main
        ;;
    observe)
        [[ "$MARKET_COMMIT" =~ ^[0-9a-f]{40}$ && "$(remote_ref "$main_ref")" == "$MARKET_COMMIT" ]] ||
            die 'This run is not observing the current protected main'
        observed="$("$tools" observe-publication -market-sha "$MARKET_COMMIT" -public-keys "$working/keys.json")"
        jq -c '{commit,source_commit,key_id,epoch,generation,expires_at,pending}' <<<"$observed"
        if jq -e '(.pending // []) | length == 0' <<<"$observed" >/dev/null; then
            printf 'state=published\n' >>"$GITHUB_OUTPUT"
            printf '### Plugin market published\n\nVerified %s and all referenced assets on the default CDN.\n' "$MARKET_COMMIT" >>"$GITHUB_STEP_SUMMARY"
        else
            printf 'state=propagating\n' >>"$GITHUB_OUTPUT"
            printf '### Plugin market propagating\n\nSnapshot %s is not fully observable on the default CDN.\n' "$MARKET_COMMIT" >>"$GITHUB_STEP_SUMMARY"
        fi
        ;;
    *)
        die 'Usage: registry-publish.sh stage|propose|observe'
        ;;
esac

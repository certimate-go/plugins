#!/usr/bin/env bash
set -euo pipefail

die() {
    printf '%s\n' "$*" >&2
    exit 1
}

[[ "$GITHUB_REPOSITORY" == certimate-go/plugins && "$GITHUB_SHA" =~ ^[0-9a-f]{40}$ ]] || die 'Expected protected registry checkout'
[[ -n "$PUBLIC_KEYS" ]] || die 'Index trust keys are not configured'
tools="$RUNNER_TEMP/registryctl"
market_ref=refs/heads/market

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
       "$(remote_ref refs/heads/main)" == "$GITHUB_SHA" ]] || die 'Registry main advanced; rerun from the latest protected main'
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
        market="$(remote_ref "$market_ref")"
        if [[ -n "$market" ]]; then
            git fetch --no-tags origin "$market_ref"
            [[ "$(git rev-parse --verify 'FETCH_HEAD^{commit}')" == "$market" ]] || die 'Market moved while being fetched; retry'
        fi
        [[ -n "$signing_seed" ]] || die 'Missing protected index signing seed'
        printf '%s' "$signing_seed" >"$working/seed"
        unset signing_seed
        publication="$("$tools" stage-publication -source-sha "$GITHUB_SHA" -market-sha "$market" \
            -key-id "$INDEX_KEY_ID" -epoch "$INDEX_EPOCH" -public-keys "$working/keys.json" -key-file "$working/seed" \
            -lifetime "$INDEX_LIFETIME" -renew-before "$INDEX_RENEW_BEFORE")"
        printf 'market_base=%s\nmarket_commit=%s\nchanged=%s\n' \
            "$market" "$(jq -er .commit <<<"$publication")" "$(jq -r .changed <<<"$publication")" >>"$GITHUB_OUTPUT"
        jq -c '{commit,source_commit,key_id,epoch,generation,expires_at,changed,pending}' <<<"$publication"
        ;;
    push)
        [[ "$MARKET_COMMIT" =~ ^[0-9a-f]{40}$ && -n "$GH_TOKEN" ]] || die 'Missing staged snapshot or market App token'
        require_current_source
        require_legacy_release_stopped
        remote_market="$(remote_ref "$market_ref")"
        [[ "$remote_market" == "$MARKET_BASE" ]] || die 'Market advanced; recompute against its new generation'
        actual_parents="$(git --no-replace-objects rev-list -n 1 --parents "$MARKET_COMMIT")"
        [[ "$actual_parents" == "$MARKET_COMMIT${MARKET_BASE:+ $MARKET_BASE}" ]] || die 'Snapshot is not a child of the verified market tip'
        GIT_CONFIG_COUNT=1 GIT_CONFIG_KEY_0=credential.https://github.com.helper GIT_CONFIG_VALUE_0='!gh auth git-credential' \
            git push origin "$MARKET_COMMIT:$market_ref"
        ;;
    observe)
        [[ "$MARKET_COMMIT" =~ ^[0-9a-f]{40}$ && "$(remote_ref "$market_ref")" == "$MARKET_COMMIT" ]] ||
            die 'This run is not observing the current market commit'
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
        die 'Usage: registry-publish.sh stage|push|observe'
        ;;
esac

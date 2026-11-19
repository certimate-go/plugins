#!/usr/bin/env bash
set -euo pipefail

tools="$RUNNER_TEMP/registryctl"

case "${1:-}" in
    stage)
        working=$(mktemp -d)
        trap 'rm -rf -- "$working"' EXIT
        umask 077
        printf '%s\n' "${INDEX_SEED:?Missing index signing seed}" >"$working/seed"
        unset INDEX_SEED
        printf '%s\n' "${PUBLIC_KEYS:?Missing index public keys}" >"$working/keys.json"

        source=$(git rev-parse HEAD)
        publication=$("$tools" stage-publication -source-sha "$source" -market-sha "$source" \
            -key-id "$INDEX_KEY_ID" -epoch "$INDEX_EPOCH" -public-keys "$working/keys.json" -key-file "$working/seed" \
            -lifetime "$INDEX_LIFETIME" -renew-before "$INDEX_RENEW_BEFORE")
        jq -r '"market_commit=\(.commit)\nchanged=\(.changed)"' <<<"$publication" >>"$GITHUB_OUTPUT"
        jq -c '{commit,source_commit,epoch,generation,changed,pending}' <<<"$publication"
        ;;
    push)
        GIT_CONFIG_COUNT=1 GIT_CONFIG_KEY_0=credential.https://github.com.helper \
            GIT_CONFIG_VALUE_0='!gh auth git-credential' git push origin "$MARKET_COMMIT:refs/heads/main"
        ;;
    observe)
        keys=$(mktemp)
        trap 'rm -f -- "$keys"' EXIT
        printf '%s\n' "${PUBLIC_KEYS:?Missing index public keys}" >"$keys"
        observed=$("$tools" observe-publication -market-sha "$MARKET_COMMIT" -public-keys "$keys")
        jq -c '{commit,source_commit,epoch,generation,expires_at,pending}' <<<"$observed"
        if jq -e '(.pending // []) | length == 0' <<<"$observed" >/dev/null; then
            printf '### Plugin market published\n\nVerified %s on the default CDN.\n' "$MARKET_COMMIT" >>"$GITHUB_STEP_SUMMARY"
        else
            printf '### Plugin market propagating\n\nSnapshot %s is pending on the default CDN.\n' "$MARKET_COMMIT" >>"$GITHUB_STEP_SUMMARY"
        fi
        ;;
    *)
        printf 'Usage: registry-publish.sh stage|push|observe\n' >&2
        exit 1
        ;;
esac

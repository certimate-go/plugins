#!/usr/bin/env bash
set -euo pipefail

die() {
    printf '%s\n' "$*" >&2
    exit 1
}

[[ "$GITHUB_REPOSITORY" == certimate-go/plugins && "$GITHUB_REF" == refs/heads/main &&
   "$GITHUB_REF_PROTECTED" == true && "$GITHUB_SHA" =~ ^[0-9a-f]{40}$ ]] || die 'Expected the protected registry checkout'

policy=registry/_policy/admissions.json
receipts=registry/_policy/admission-batches
git() { command git --no-replace-objects "$@"; }

remote_main() {
    git ls-remote --exit-code --refs origin refs/heads/main | cut -f1
}

verify_target() {
    local entry="$1" repo source tree provider version refs
    repo="$(jq -er .repository <<<"$entry")"
    source="$(jq -er .target_commit <<<"$entry")"
    tree="$(jq -er .target_tree <<<"$entry")"
    provider="$(jq -er .provider_type <<<"$entry")"
    version="$(jq -er .version <<<"$entry")"
    gh api "repos/$repo" | jq -e --arg repo "$repo" \
        '.full_name == $repo and .private == false and .archived == false and .default_branch == "main"' >/dev/null || return
    gh api "repos/$repo/branches/main" | jq -e --arg source "$source" \
        '.name == "main" and .protected == true and .commit.sha == $source' >/dev/null || return
    gh api "repos/$repo/git/commits/$source" | jq -e --arg tree "$tree" '.tree.sha == $tree' >/dev/null || return
    gh api -H 'Accept: application/vnd.github.raw+json' "repos/$repo/contents/manifest.json?ref=$source" |
        jq -e --arg provider "$provider" --arg version "$version" \
            '.provider_type == $provider and .version == $version and (has("release") | not)' >/dev/null || return
    refs="$(git ls-remote --refs "https://github.com/$repo.git" "refs/tags/v$version" "refs/tags/v$version-detach")" || return
    [[ -z "$refs" ]] || return 1
    local ci
    ci="$(gh api -H 'Accept: application/vnd.github.raw+json' "repos/$repo/contents/.github/workflows/ci.yml?ref=$source")" || return
    [[ "$ci" == "$(cat skills/certimate-plugin-dev/assets/github-actions/official-provider-ci.yml)" ]] || return 1
    gh api "repos/$repo/actions/workflows/ci.yml/runs?branch=main&event=push&head_sha=$source&status=success&per_page=1" |
        jq -e --arg source "$source" '.workflow_runs | any(.head_sha == $source and .conclusion == "success")' >/dev/null
}

plan() {
    local directory="$1" item
    mkdir -p "$directory"
    printf '%s\n' "$MIGRATION_BATCH" >"$directory/input.json"
    [[ "$(wc -c <"$directory/input.json")" -le 65536 ]] || die 'Migration batch exceeds the input size limit'
    go build -mod=readonly -o "$directory/registryctl" ./cmd/registryctl
    "$directory/registryctl" plan-admission -base "$GITHUB_SHA" -batch "$directory/input.json" >"$directory/plan.json"
    jq -c '.entries[]' "$directory/plan.json" >"$directory/pending.ndjson"
    : >"$directory/ready.ndjson"
    while IFS= read -r item; do
        if verify_target "$item"; then
            printf '%s\n' "$item" >>"$directory/ready.ndjson"
        else
            printf 'Target preflight failed for %s\n' "$(jq -r .repository <<<"$item")" >&2
        fi
    done <"$directory/pending.ndjson"
    if [[ ! -s "$directory/ready.ndjson" ]]; then
        [[ ! -s "$directory/pending.ndjson" ]] || die 'No pending target passed preflight'
        printf 'ready=false\n' >>"$GITHUB_OUTPUT"
        return
    fi
    mkdir "$directory/artifact"
    jq -sS . "$directory/ready.ndjson" >"$directory/artifact/batch.json"
    "$directory/registryctl" plan-admission -base "$GITHUB_SHA" -batch "$directory/artifact/batch.json" |
        jq -S '.admissions' >"$directory/artifact/admissions.json"
    printf 'ready=true\nattempt=%s\n' "$GITHUB_RUN_ATTEMPT" >>"$GITHUB_OUTPUT"
}

propose() {
    local directory="$1" digest branch remote pull receipt commit url
    [[ "$(remote_main)" == "$GITHUB_SHA" ]] || die 'Protected main advanced, rerun the migration with the same batch'
    digest="$(sha256sum <"$directory/batch.json" | cut -d' ' -f1)"
    branch="registry/_admit/$digest"
    remote="$(git ls-remote --refs origin "refs/heads/$branch" | cut -f1)"
    pull="$(gh api "repos/$GITHUB_REPOSITORY/pulls?state=all&head=certimate-go:$branch&base=main&per_page=10")"
    url="$(jq -er --arg sha "$remote" '
        if length == 0 then "" else
        .[0] | select(.state == "open" and .head.sha == $sha and .user.login == "certimate-plugin-dispatcher[bot]") | .html_url
        end' <<<"$pull")" || die 'Existing admission PR is closed or conflicts with this batch'
    local parents=(-p "$GITHUB_SHA")
    local rebuild=false
    if [[ -z "$remote" ]]; then
        rebuild=true
    else
        git fetch --no-tags origin "refs/heads/$branch"
        [[ "$(git rev-parse 'FETCH_HEAD^{commit}')" == "$remote" ]] || die 'Admission branch moved'
        if ! git merge-tree --write-tree "$GITHUB_SHA" "$remote" >/dev/null; then
            [[ -n "$url" ]] || die 'The existing admission branch conflicts and has no open bot PR'
            parents=(-p "$remote" -p "$GITHUB_SHA")
            rebuild=true
        fi
    fi
    if [[ "$rebuild" == true ]]; then
        [[ "$GITHUB_RUN_ID" =~ ^[1-9][0-9]{0,19}$ && "$PREPARE_ATTEMPT" =~ ^[1-9][0-9]{0,8}$ ]] || die 'Invalid GitHub workflow attempt'
        receipt="$receipts/$GITHUB_RUN_ID-$PREPARE_ATTEMPT.json"
        mkdir -p "$receipts"
        cp "$directory/batch.json" "$receipt"
        cp "$directory/admissions.json" "$policy"
        git add -- "$policy" "$receipt"
        commit="$(git -c user.name='Certimate Plugin Registrar' -c user.email='release@certimate.me' \
            -c commit.gpgSign=false commit-tree "$(git write-tree)" "${parents[@]}" -m 'Admit migrated providers')"
        GIT_CONFIG_COUNT=1 GIT_CONFIG_KEY_0=credential.https://github.com.helper \
            GIT_CONFIG_VALUE_0='!gh auth git-credential' git push origin "$commit:refs/heads/$branch"
    fi
    if [[ -z "$url" ]]; then
        gh api --method POST "repos/$GITHUB_REPOSITORY/pulls" -f 'title=Admit migrated providers' \
            -f 'base=main' -f "head=$branch" -f "body=Migration batch $digest." --jq .html_url
        return
    fi
    printf '%s\n' "$url"
}

pending() {
    local directory="$1" entry provider repo version source refs record trigger formal
    shopt -s nullglob
    local files=("$receipts"/*.json)
    : >"$directory/requests.ndjson"
    if (( ${#files[@]} != 0 )); then
        jq -c '.[]' "${files[@]}" >"$directory/entries.ndjson"
        while IFS= read -r entry; do
            provider="$(jq -er .provider_type <<<"$entry")"
            repo="$(jq -er .repository <<<"$entry")"
            version="$(jq -er .version <<<"$entry")"
            source="$(jq -er .target_commit <<<"$entry")"
            jq -e --arg provider "$provider" --arg repo "$repo" '.[$provider] == $repo' "$policy" >/dev/null || continue
            record="registry/$provider/v$version.json"
            if [[ -f "$record" ]]; then
                if ! jq -e --arg source "$source" '.source_commit == $source' "$record" >/dev/null; then
                    printf 'Conflicting version record for %s v%s\n' "$repo" "$version" >&2
                fi
                continue
            fi
            if ! refs="$(git ls-remote --refs "https://github.com/$repo.git" "refs/tags/v$version" "refs/tags/v$version-detach")"; then
                printf 'Could not read tags for %s\n' "$repo" >&2
                continue
            fi
            trigger="$(awk -v ref="refs/tags/v$version-detach" '$2 == ref {print $1}' <<<"$refs")"
            formal="$(awk -v ref="refs/tags/v$version" '$2 == ref {print $1}' <<<"$refs")"
            if [[ -n "$trigger" && "$trigger" != "$source" ]]; then
                printf 'Conflicting release request for %s v%s\n' "$repo" "$version" >&2
            elif [[ -n "$formal" ]]; then
                local parent
                parent="$(gh api "repos/$repo/git/commits/$formal" --jq '.parents | select(length == 1) | .[0].sha')" || continue
                if [[ "$parent" != "$source" ]]; then
                    printf 'Conflicting formal tag for %s v%s\n' "$repo" "$version" >&2
                fi
            elif [[ -z "$trigger" ]]; then
                printf '%s\n' "$entry" >>"$directory/requests.ndjson"
            fi
        done <"$directory/entries.ndjson"
    fi
    printf 'requests=%s\n' "$(jq -cs '.[0:24]' "$directory/requests.ndjson")" >>"$GITHUB_OUTPUT"
}

request() {
    local repo source tree version target refs branch
    repo="$(jq -er .repository <<<"$ENTRY")"
    source="$(jq -er .target_commit <<<"$ENTRY")"
    tree="$(jq -er .target_tree <<<"$ENTRY")"
    version="$(jq -er .version <<<"$ENTRY")"
    target="tags/v$version-detach"
    refs="$(git ls-remote --refs "https://github.com/$repo.git" "refs/tags/v$version" "refs/$target")"
    if [[ -n "$refs" ]]; then
        [[ "$refs" == "$source"$'\t'"refs/$target" ]] || die "Conflicting or completed version for $repo v$version"
        return
    fi
    branch="$(gh api "repos/$repo/branches/main" --jq 'select(.protected == true).commit.sha')"
    [[ "$branch" =~ ^[0-9a-f]{40}$ ]] || die "No protected main for $repo"
    gh api "repos/$repo/compare/$source...$branch" --jq .status | grep -Eq '^(identical|ahead)$' ||
        die "Migration commit is no longer on protected main of $repo"
    gh api "repos/$repo/git/commits/$source" | jq -e --arg tree "$tree" '.tree.sha == $tree' >/dev/null
    gh api --method POST "repos/$repo/git/refs" -f "ref=refs/$target" -f "sha=$source" --jq .ref
}

case "${1:-}" in
    plan) plan "$2" ;;
    propose) propose "$2" ;;
    pending) pending "$2" ;;
    request) request ;;
    *) die 'Expected plan, propose, pending or request' ;;
esac

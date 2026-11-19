#!/usr/bin/env bash
set -euo pipefail

die() {
    printf '%s\n' "$*" >&2
    exit 1
}

[[ "$GITHUB_REPOSITORY" == certimate-go/plugins && "$GITHUB_SHA" =~ ^[0-9a-f]{40}$ ]] || die 'Expected protected registry checkout'
[[ -n "$DISPATCH_TOKEN" && -n "$REGISTRATION_TOKEN" && -n "$CHECK_DISPATCH_TOKEN" && -n "$REGISTRATION_BOT" ]] || die 'Missing scoped registry credentials'
policy=registry/_policy/admissions.json
registry_git() { git --no-replace-objects "$@"; }

existing_ref() {
    local remote="$1" ref="$2" answer status
    if answer="$(git ls-remote --exit-code --refs "$remote" "$ref")"; then
        printf '%s\n' "${answer%%$'\t'*}"
    else
        status=$?
        [[ "$status" == 2 ]] || return "$status"
    fi
}

provider_api() { GH_TOKEN="$DISPATCH_TOKEN" gh api "$@"; }
registrar_api() { GH_TOKEN="$REGISTRATION_TOKEN" gh api "$@"; }

dispatch_request() {
    local repo="$1" trigger="$2" title="Release " status active
    title+="$trigger"
    for status in queued in_progress waiting requested pending; do
        active="$(provider_api --paginate "repos/$repo/actions/workflows/release.yml/runs?branch=main&event=workflow_dispatch&status=$status&per_page=100" \
            --jq '.workflow_runs[].display_title')"
        if grep -Fxq -- "$title" <<<"$active"; then
            printf '%s %s already has a pending run\n' "$repo" "$trigger"
            return
        fi
    done
    GH_TOKEN="$DISPATCH_TOKEN" gh workflow run release.yml --repo "$repo" --ref main -f "trigger_tag=$trigger"
    printf 'Dispatched %s %s\n' "$repo" "$trigger"
}

register_release() {
    local directory="$1" provider="$2" repo="$3" tag="$4" expected="$5"
    local record="$directory/release.json" source_repo="$directory/providers/$provider" proof="$directory/evidence/$provider"
    local receipt="registry/$provider/$tag.json" branch="registry/$provider/$tag"
    local branch_ref="refs/heads/$branch" run_id commit current pull pull_id actor source_commit
    mkdir -p "$proof"
    git init --bare -q "$source_repo"
    git -C "$source_repo" fetch --no-tags "https://github.com/$repo.git" \
        refs/heads/main:refs/remotes/origin/main "refs/tags/$tag:refs/tags/$tag"
    "$REGISTRYCTL" inspect-release -checkout "$source_repo" -repository "$repo" -tag "$tag" >"$record"
    jq -e --arg commit "$expected" '.release_commit == $commit' "$record" >/dev/null || die "Formal tag $repo@$tag moved during the scan"
    run_id="$(jq -er .build_run_id "$record")"
    source_commit="$(jq -er .source_commit "$record")"
    provider_api "repos/$repo/branches/main" >"$proof/main.json"
    provider_api "repos/$repo/actions/runs/$run_id" >"$proof/$tag.json"
    provider_api --paginate --slurp "repos/$repo/actions/runs/$run_id/jobs?filter=all&per_page=100" |
        jq -ce '{jobs: [.[].jobs[]]}' >"$proof/$tag-jobs.json"
    actor="$(jq -r '.triggering_actor.login // .actor.login // "(unavailable)"' "$proof/$tag.json")"

    local index="$directory/index" blob tree
    GIT_INDEX_FILE="$index" registry_git read-tree "$GITHUB_SHA"
    blob="$(registry_git hash-object -w --stdin <"$record")"
    GIT_INDEX_FILE="$index" registry_git update-index --add --cacheinfo "100644,$blob,$receipt"
    tree="$(GIT_INDEX_FILE="$index" registry_git write-tree)"
    commit="$(registry_git -c user.name='Certimate Plugin Registrar' -c user.email='release@certimate.me' -c commit.gpgSign=false \
        commit-tree "$tree" -p "$GITHUB_SHA" -m "Register $provider $tag")"
    "$REGISTRYCTL" verify-registrations -checkout "$PWD" -base "$GITHUB_SHA" -candidate "$commit" \
        -providers "$directory/providers" -evidence "$directory/evidence" >/dev/null

    current="$(existing_ref origin "$branch_ref")"
    if [[ -z "$current" ]]; then
        GIT_CONFIG_COUNT=1 GIT_CONFIG_KEY_0=credential.https://github.com.helper GIT_CONFIG_VALUE_0='!gh auth git-credential' \
            GH_TOKEN="$REGISTRATION_TOKEN" registry_git push origin "$commit:$branch_ref"
    else
        [[ "$current" =~ ^[0-9a-f]{40}$ ]] || die 'Invalid registration branch commit'
        registry_git fetch --no-tags origin "$branch_ref"
        [[ "$(registry_git rev-parse --verify 'FETCH_HEAD^{commit}')" == "$current" ]] || die 'Registration branch changed'
        local parent_line parent
        parent_line="$(registry_git rev-list --parents -n 1 "$current")"
        parent="${parent_line#* }"
        [[ "$parent" =~ ^[0-9a-f]{40}$ ]] || die 'Registration branch must only have its original parent'
        registry_git merge-base --is-ancestor "$parent" "$GITHUB_SHA" || die 'Registration branch did not originate on protected main'
        [[ "$(registry_git diff-tree -r --no-commit-id --no-renames --name-status "$parent" "$current")" == $'A\t'"$receipt" ]] &&
            registry_git show "$current:$receipt" | cmp -s "$record" - || die 'Existing registration branch has conflicting content'
        commit="$current"
    fi

    pull="$(registrar_api "repos/$GITHUB_REPOSITORY/pulls?state=all&head=certimate-go:$branch&base=main&per_page=10")"
    pull_id="$(jq -er --arg sha "$commit" --arg bot "$REGISTRATION_BOT" '
        if length == 0 then "" else
        .[0] | select(.state == "open" and .head.sha == $sha and .user.login == $bot) | .number
        end' <<<"$pull")" || die 'Existing registration pull request is closed or conflicts with this release'
    if [[ -z "$pull_id" ]]; then
        pull_id="$(registrar_api --method POST "repos/$GITHUB_REPOSITORY/pulls" \
            -f "title=Register $provider $tag" -f 'base=main' -f "head=$branch" \
            -f "body=Provider: $repo; formal tag: $tag; artifact commit: $expected; build run: $run_id." --jq .number)"
    fi
    [[ "$pull_id" =~ ^[1-9][0-9]*$ ]] || die 'Invalid registration pull request number'
    GH_TOKEN="$CHECK_DISPATCH_TOKEN" gh workflow run registry-check.yml --repo "$GITHUB_REPOSITORY" --ref main \
        -f "candidate_ref=refs/pull/$pull_id/merge"
    GH_TOKEN="$REGISTRATION_TOKEN" gh pr merge "$pull_id" --repo "$GITHUB_REPOSITORY" --auto --match-head-commit "$commit"
    printf 'Registration queued: %s %s PR #%s, source %s, artifacts %s, run %s, actor %s\n' \
        "$repo" "$tag" "$pull_id" "$source_commit" "$expected" "$run_id" "$actor"
}

case "${1:-}" in
    scan)
        [[ "$(existing_ref origin refs/heads/main)" == "$GITHUB_SHA" ]] || die 'Registry main changed; dispatch reconciliation again'
        scratch="$(mktemp -d)"
        export REGISTRYCTL="$scratch/registryctl"
        go build -mod=readonly -o "$REGISTRYCTL" ./cmd/registryctl
        jq -er 'to_entries | .[] | [.key,.value] | @tsv' "$policy" >"$scratch/admissions.tsv"
        result=0
        while IFS=$'\t' read -r provider repo; do
            bash "$0" provider "$scratch/$provider" "$provider" "$repo" || result=1
        done <"$scratch/admissions.tsv"
        git ls-remote --heads --refs origin 'refs/heads/gh-readonly-queue/main/*' >"$scratch/queue"
        while IFS=$'\t' read -r _ ref; do
            if [[ "$ref" =~ ^refs/heads/gh-readonly-queue/main/pr-[1-9][0-9]*-[0-9a-f]{7,40}$ ]]; then
                GH_TOKEN="$CHECK_DISPATCH_TOKEN" gh workflow run registry-check.yml --repo "$GITHUB_REPOSITORY" --ref main \
                    -f "candidate_ref=$ref" || result=1
            fi
        done <"$scratch/queue"
        exit "$result"
        ;;
    provider)
        directory="$2" provider="$3" repo="$4"
        [[ "$provider" =~ ^[a-z0-9][a-z0-9_.-]*$ && "$repo" =~ ^certimate-go/plugin-provider-[a-z0-9][a-z0-9_.-]*$ ]] &&
            jq -e --arg provider "$provider" --arg repo "$repo" '.[$provider] == $repo' "$policy" >/dev/null || die 'Malformed admission'
        mkdir -p "$directory"
        git ls-remote --tags "https://github.com/$repo.git" >"$directory/refs"
        "$REGISTRYCTL" plan-provider -repository "$repo" -refs "$directory/refs" >"$directory/plan"
        jq -r '.dispatch[]' "$directory/plan" >"$directory/dispatch"
        jq -r '.register[] | [.tag,.release_commit] | @tsv' "$directory/plan" >"$directory/register"
        result=0
        while IFS= read -r trigger; do
            bash "$0" dispatch "$repo" "$trigger" || result=1
        done <"$directory/dispatch"
        while IFS=$'\t' read -r tag sha; do
            bash "$0" register "$directory/$tag" "$provider" "$repo" "$tag" "$sha" || result=1
        done <"$directory/register"
        exit "$result"
        ;;
    dispatch)
        dispatch_request "$2" "$3"
        ;;
    register)
        register_release "$2" "$3" "$4" "$5" "$6"
        ;;
    *)
        die 'Usage: registry-reconcile.sh scan'
        ;;
esac

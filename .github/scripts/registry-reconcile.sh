#!/usr/bin/env bash
set -euo pipefail

register_release() {
    local directory="$1" repository="$2" tag="$3" expected="$4"
    local record="$directory/record.json" source_repo="$directory/provider.git"
    local provider receipt branch branch_ref commit listing pulls url
    mkdir -p "$directory"
    git init --bare -q "$source_repo"
    git -C "$source_repo" fetch --no-tags --depth=2 "https://github.com/$repository.git" \
        "refs/tags/$tag:refs/tags/$tag"
    "$REGISTRYCTL" inspect-release -checkout "$source_repo" -repository "$repository" -tag "$tag" >"$record"
    provider=$(jq -er --arg commit "$expected" 'select(.release_commit == $commit) | .provider_type' "$record")
    if jq -e --arg provider "$provider" --arg version "${tag#v}" \
        'any(.[]; .provider_type == $provider and .version == $version)' registry/_policy/withdrawals.json >/dev/null; then
        return
    fi
    shopt -s nullglob
    local records=(registry/"$provider"/v*.json)
    if (( ${#records[@]} > 0 )); then
        jq -es --arg repository "$repository" 'all(.[]; .repository == $repository)' "${records[@]}" >/dev/null
    fi
    receipt="registry/$provider/$tag.json"
    branch="registry/$provider/$tag"
    branch_ref="refs/heads/$branch"
    pulls=$(gh api "repos/$GITHUB_REPOSITORY/pulls?state=all&head=certimate-go:$branch&base=main")
    if jq -e 'length > 0' <<<"$pulls" >/dev/null; then
        jq -r '.[0] | "Registration PR already exists: " + .html_url' <<<"$pulls"
        return
    fi

    listing=$(git ls-remote --heads --refs origin "$branch_ref")
    if [[ -n "$listing" ]]; then
        git fetch --no-tags origin "$branch_ref"
        commit=$(git rev-parse FETCH_HEAD)
        git show "$commit:$receipt" | cmp -s "$record" -
        [[ "$(git diff-tree --no-commit-id -r --name-only "$commit^" "$commit")" == "$receipt" ]]
    else
        export GIT_INDEX_FILE="$directory/index"
        local blob tree
        git read-tree "$GITHUB_SHA"
        blob=$(git hash-object -w "$record")
        git update-index --add --cacheinfo "100644,$blob,$receipt"
        tree=$(git write-tree)
        unset GIT_INDEX_FILE
        commit=$(git -c user.name='github-actions[bot]' -c user.email='41898282+github-actions[bot]@users.noreply.github.com' \
            -c commit.gpgSign=false commit-tree "$tree" -p "$GITHUB_SHA" -m "Register $provider $tag")
        GIT_CONFIG_COUNT=1 GIT_CONFIG_KEY_0=credential.https://github.com.helper \
            GIT_CONFIG_VALUE_0='!gh auth git-credential' git push origin "$commit:$branch_ref"
    fi

    url=$(gh api --method POST "repos/$GITHUB_REPOSITORY/pulls" \
        -f "title=Register $provider $tag" -f 'base=main' -f "head=$branch" \
        -f "body=Provider: $repository. Formal tag: $tag. Artifact commit: $expected." --jq .html_url)
    printf '%s\n' "$url"
}

case "${1:-}" in
    scan)
        scratch=$(mktemp -d)
        trap 'rm -rf -- "$scratch"' EXIT
        export REGISTRYCTL="$scratch/registryctl"
        go build -mod=readonly -o "$REGISTRYCTL" ./cmd/registryctl
        gh api 'orgs/certimate-go/repos?type=public&per_page=100' --paginate \
            --jq '.[] | select(.archived == false and (.full_name | test("^certimate-go/plugin-provider-[a-z0-9][a-z0-9_.-]*$"))) | .full_name' >"$scratch/repositories"
        result=0
        while IFS= read -r repository; do
            bash "$0" provider "$scratch/${repository#*/}" "$repository" || result=1
        done <"$scratch/repositories"
        exit "$result"
        ;;
    provider)
        directory="$2" repository="$3"
        mkdir -p "$directory"
        git ls-remote --tags "https://github.com/$repository.git" >"$directory/refs"
        "$REGISTRYCTL" plan-provider -repository "$repository" -refs "$directory/refs" |
            jq -r '.register[] | [.tag,.release_commit] | @tsv' >"$directory/register"
        result=0
        while IFS=$'\t' read -r tag sha; do
            bash "$0" register "$directory/$tag" "$repository" "$tag" "$sha" || result=1
        done <"$directory/register"
        exit "$result"
        ;;
    register)
        register_release "$2" "$3" "$4" "$5"
        ;;
    *)
        printf 'Usage: registry-reconcile.sh scan\n' >&2
        exit 1
        ;;
esac

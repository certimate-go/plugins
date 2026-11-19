#!/usr/bin/env bash
set -euo pipefail

die() {
    printf '%s\n' "$*" >&2
    exit 1
}

pull_id=
if [[ "$CANDIDATE_REF" =~ ^refs/pull/([1-9][0-9]*)/merge$ ]]; then
    pull_id="${BASH_REMATCH[1]}"
elif [[ ! "$CANDIDATE_REF" =~ ^refs/heads/gh-readonly-queue/main/pr-[1-9][0-9]*-[0-9a-f]{7,40}$ ]]; then
    die 'Not a GitHub pull merge or merge-queue ref'
fi

[[ -n "$EVIDENCE_TOKEN" ]] || die 'Missing read-only provider evidence token'
[[ "$GITHUB_SHA" =~ ^[0-9a-f]{40}$ ]] || die 'Invalid protected registry revision'
git --no-replace-objects fetch --no-tags origin refs/heads/main:refs/remotes/origin/main
git --no-replace-objects fetch --no-tags origin "$CANDIDATE_REF"
candidate="$(git --no-replace-objects rev-parse --verify 'FETCH_HEAD^{commit}')"
base="$(git --no-replace-objects rev-parse --verify 'refs/remotes/origin/main^{commit}')"
[[ "$base" == "$GITHUB_SHA" ]] || die 'Protected registry changed; dispatch the check again from main'
if [[ -n "$pull_id" ]]; then
    pull="$(gh api "repos/$GITHUB_REPOSITORY/pulls/$pull_id")"
    head="$(jq -er --arg repo "$GITHUB_REPOSITORY" '
        select(.state == "open" and .draft == false and .user.login == "certimate-plugin-dispatcher[bot]" and .base.ref == "main" and
            .base.repo.full_name == $repo and .head.repo.full_name == $repo) | .head.sha' <<<"$pull")"
    [[ "$head" =~ ^[0-9a-f]{40}$ ]] || die 'Invalid registrar pull request head'
    actual_parents="$(git --no-replace-objects rev-list -n 1 --parents "$candidate")"
    [[ "$actual_parents" == "$candidate $base $head" ]] || die 'Pull request changed; dispatch the check again'
fi

scratch="$(mktemp -d)"
trap 'rm -rf "$scratch"' EXIT
mkdir "$scratch/providers" "$scratch/evidence" "$scratch/evidence/admission-batches"
go build -mod=readonly -o "$scratch/registryctl" ./cmd/registryctl
common=(-checkout "$PWD" -base "$base" -candidate "$candidate")
if [[ -n "${PUBLIC_KEYS:-}" ]]; then
    printf '%s\n' "$PUBLIC_KEYS" >"$scratch/keys.json"
    common+=(-public-keys "$scratch/keys.json")
fi
if [[ -z "$pull_id" ]]; then
    common+=(-queue)
fi
git --no-replace-objects diff --name-only -z "$base" "$candidate" >"$scratch/paths"
while IFS= read -r -d '' path; do
    if [[ ! "$path" =~ ^registry/_policy/admission-batches/([1-9][0-9]{0,19})-([1-9][0-9]{0,8})\.json$ ]]; then
        continue
    fi
    run_id="${BASH_REMATCH[1]}"
    attempt="${BASH_REMATCH[2]}"
    run_path="repos/$GITHUB_REPOSITORY/actions/runs/$run_id/attempts/$attempt"
    source="$(gh api "$run_path" | jq -er --arg repo "$GITHUB_REPOSITORY" --arg run "$run_id" --arg attempt "$attempt" '
        select((.id | tostring) == $run and (.run_attempt | tostring) == $attempt and
            .repository.full_name == $repo and .head_repository.full_name == $repo and
            .head_branch == "main" and .event == "workflow_dispatch" and
            .path == ".github/workflows/registry-admit.yml")
        | .head_sha | select(test("^[0-9a-f]{40}$"))
    ')"
    git --no-replace-objects merge-base --is-ancestor "$source" "$base" || die 'Migration workflow did not run on protected history'
    gh api "$run_path/jobs?per_page=100" |
        jq -e '.jobs | any(.name == "Prepare migration batch" and .conclusion == "success")' >/dev/null
    gh run download "$run_id" --repo "$GITHUB_REPOSITORY" --name "migration-batch-$attempt" --dir "$scratch/batch-$run_id-$attempt"
    cp "$scratch/batch-$run_id-$attempt/batch.json" "$scratch/evidence/admission-batches/$run_id-$attempt.json"
done <"$scratch/paths"

common+=(-evidence "$scratch/evidence")
"$scratch/registryctl" registration-changes "${common[@]}" >"$scratch/changes.json"
jq -r '.[] | [.provider_type, .repository, .tag, .build_run_id] | @tsv' "$scratch/changes.json" >"$scratch/changes.tsv"
while IFS=$'\t' read -r provider repository tag run_id; do
    git_dir="$scratch/providers/$provider"
    proof="$scratch/evidence/$provider"
    mkdir -p "$proof"
    if [[ ! -d "$git_dir" ]]; then
        git init --bare -q "$git_dir"
        git -C "$git_dir" remote add origin "https://github.com/$repository.git"
        GH_TOKEN="$EVIDENCE_TOKEN" gh api "repos/$repository/branches/main" >"$proof/main.json"
        git -C "$git_dir" fetch --no-tags origin refs/heads/main:refs/remotes/origin/main
    fi
    git -C "$git_dir" fetch --no-tags origin "refs/tags/$tag:refs/tags/$tag"
    GH_TOKEN="$EVIDENCE_TOKEN" gh api "repos/$repository/actions/runs/$run_id" >"$proof/$tag.json"
    GH_TOKEN="$EVIDENCE_TOKEN" gh api --paginate --slurp "repos/$repository/actions/runs/$run_id/jobs?filter=all&per_page=100" |
        jq -ce '{jobs: [.[].jobs[]]}' >"$proof/$tag-jobs.json"
done <"$scratch/changes.tsv"
"$scratch/registryctl" verify-registrations "${common[@]}" -providers "$scratch/providers"

actual="$(git ls-remote --exit-code --refs origin "$CANDIDATE_REF" | cut -f1)"
actual_base="$(git ls-remote --exit-code --refs origin refs/heads/main | cut -f1)"
[[ "$actual" == "$candidate" && "$actual_base" == "$base" ]] || die 'Registry candidate changed during verification'
printf 'sha=%s\n' "$candidate" >>"$GITHUB_OUTPUT"

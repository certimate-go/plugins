# Official plugin release contract

An official repository uses the `plugin-provider-` prefix and describes the
integration and its capability. For example, `plugin-provider-axisnow-cdn-deployer`
publishes the plugin whose `provider_type` is `axisnow`. The protected admissions
map binds each plugin ID to its repository. Each repository contains one
buildable Go plugin at its root. The root `manifest.json` uses the
runtime manifest fields and has no `release` object. The manifest version must
match the stable version in the `vX.Y.Z-detach` trigger tag.

Use the final role suffix `deployer`, `purger`, `notifier`, or
`acmedns01challenger` for the manifest's `deploy`, `purge`, `notify`, or `dns`
capability. Run `go run ./cmd/pluginlint -repository certimate-go/<repository> <plugin-dir>`
from the registry checkout to check the repository name, Go module, icon,
schema, and translations. A local directory named `plugin-provider-*` is
recognized automatically.

## Build output

From a trusted revision of the registry tooling, run:

```sh
go run ./cmd/providerrelease \
  -source-dir "$PROVIDER_CHECKOUT" \
  -repository "$PROVIDER_REPOSITORY" \
  -trigger-tag "$TRIGGER_TAG" \
  -source-sha "$VERIFIED_SOURCE_SHA" \
  -run-id "$BUILD_RUN_ID"
```

Use a clean, isolated checkout of the verified SHA. Initialize its submodules
recursively. The packager checks that each submodule is at its recorded commit,
then builds all eight supported platform ZIPs with Go workspace mode off.
Each `build/<os>_<arch>.zip` contains the executable, a
manifest with platform and executable SHA-256, and the declared icon, if any.
Each ZIP must fit within 20,000,000 bytes, and the Git tree including all release
artifacts must fit within 150,000,000 bytes. A failing build does not emit a
publishable `build/` directory.

The packager writes `build/release.json` with provider, repository, version,
formal tag, source commit, trusted build run ID, and the path, size, and SHA-256
of each ZIP. The registry must derive the **release** commit SHA from the
protected formal tag, not from an embedded field: a file committed in that tree
cannot include the SHA of the commit that contains it.

## Publish boundary

The trigger tag is data, not the workflow revision. Only a protected workflow
may verify its ancestry, build without write credentials, and hand its outputs
to the protected tag writer. The tag writer accepts output only from the expected
run and source, writes `build/` on top of that source, and publishes the immutable
formal tag. It does not execute the artifacts or merge binaries into `main`.
Registry ingestion independently verifies provenance and digests. It does not
consider a release published until the signed index is observable through the
default CDN.

### Provider release configuration

Install the [official provider CI](../skills/certimate-plugin-dev/assets/github-actions/official-provider-ci.yml)
for source checks after `.github/workflows/provider-ci.yml` lands on the registry's
`main`. The shared workflow runs `pluginlint` with the calling repository's
`GITHUB_REPOSITORY` and runs the module tests with pinned submodules and Go
workspace mode off.

Install the [official release workflow](../skills/certimate-plugin-dev/assets/github-actions/official-provider-release.yml)
as `.github/workflows/release.yml` and protect `main` in both repositories.
Pushing `vX.Y.Z-detach` starts a job with Actions write permission that
dispatches the same workflow on `main` with the tag as `trigger_tag`.
The job on `main` calls the registry's
`.github/workflows/provider-release.yml@main`. The reusable workflow resolves
the tag and checks ancestry relative to the protected dispatch commit.
The build runs with read permissions only.

Configure an environment named `provider-release` restricted to the protected
default branch with `PROVIDER_RELEASE_APP_CLIENT_ID` and
`PROVIDER_RELEASE_APP_PRIVATE_KEY`. The provider caller uses `secrets: inherit`
so the reusable publish job can read the environment secret.
Install the tag-writing App on the provider
repositories with Contents write, not on the registry. Require its identity
for formal tag creation and prevent tag updates and deletion with separate
rulesets, so creation bypass does not grant a way to rewrite a tag. Protect the
trigger tags from update; the publisher also rechecks the trigger commit before
staging and before pushing.

Upload the ZIPs and record as one immutable workflow artifact, then download by
its run-scoped artifact ID into a different runner. The writer runs
`registryctl stage-release` against a bare provider repository. This command
validates the source, run ID, archive contents and full release tree, and
returns an unreferenced commit. It does not execute provider code, change a
branch, or push. The workflow issues the App token only after staging and
pushes the commit directly to the formal tag without force. If the formal tag
already points to a valid release of the same source, a rerun leaves it alone;
a version sourced from any other commit fails.

## Registry records and signed output

Admissions are maintained at `registry/_policy/admissions.json` as a map of
provider IDs to repositories. Each accepted immutable record is stored at
`registry/<provider>/vX.Y.Z.json`. It extends `build/release.json` with the
formal tag's peeled commit, the plain source manifest, title and description
translations, and optional base64-encoded PNG or JPEG. If the source manifest
uses an SVG runtime icon, add `market-icon.png` to supply its market image.

`registryctl inspect-release` reads the local formal tag without checking out
or executing source code. It checks the parent, changed paths, archive digests,
and the archived manifest, executable checksum, and icon. Its output is still a
candidate: protected GitHub workflows, branch ancestry, build run provenance and
PR changes must be verified by the registration job.

`registryctl plan-provider` reads complete `git ls-remote --tags` output and
the protected registry checkout. A pending request without a formal tag needs
dispatch; a formal tag not yet registered needs registration even if its
request tag is gone. A registered tag must continue to point to the exact
recorded artifact commit. There is no time cursor to drift past pending work.

### Registration reconciliation

Use the organization's private scheduler to dispatch
`registry-reconcile.yml` on `certimate-go/plugins@main`. The controller
processes all admitted providers, opens a registration PR for each verified
release, and retries outstanding work on the next scan. It checks for queued
builds before dispatching a pending tag again. The same workflow can be
triggered manually.

For GitHub Actions, install the [private scheduler workflow](examples/registry-scheduler.yml)
as `.github/workflows/registry-scheduler.yml` in the organization's private
operations repository. Configure its `registry-scheduler` environment for
the protected default branch. Set `REGISTRY_SCHEDULER_APP_CLIENT_ID` and
`REGISTRY_SCHEDULER_APP_PRIVATE_KEY` for a dedicated App installed only on
`certimate-go/plugins` with Actions write. The scheduler dispatches the two
central workflows independently and records both run URLs. Open each linked
run to check its result. A successful dispatch confirms that GitHub accepted
the request. The target workflow has its own verification and result.

The template starts at ten-minute intervals. Adjust the interval for the
measured renewal window and registration check timeouts. After a manual
dispatch passes, verify a scheduled run and its downstream results with no
public-repository schedule enabled.
[GitHub workflow dispatch API](https://docs.github.com/en/rest/actions/workflows#create-a-workflow-dispatch-event)

The `registry-reconcile` environment needs two Apps:

- `REGISTRY_DISPATCH_APP_CLIENT_ID` / `REGISTRY_DISPATCH_APP_PRIVATE_KEY`:
  installed only on admitted providers, with Actions write for dispatch and
  Contents read. It must not be able to publish tags.
- `REGISTRY_REGISTRATION_APP_CLIENT_ID` /
  `REGISTRY_REGISTRATION_APP_PRIVATE_KEY`: installed only on the registry,
  with Contents and Pull requests write. It must not bypass protections on
  `main`, modify policy, or write required check runs.

Each validated release gets one stable `registry/<provider>/<tag>` branch and
one PR. A rerun reuses an identical pending branch and PR; it does not overwrite
conflicting contents or reopen a deliberately closed PR. After the independent
check passes on the test merge commit, reconciliation requests entry into the
merge queue with the verified PR head as a precondition. GitHub enforces the
required check from the check App. Reconciliation dispatches verification for
live server-generated merge queue refs.

### Required registration check

Dispatch `registry-check.yml` on protected registry `main` with the
server-generated `refs/pull/<number>/merge` or
`refs/heads/gh-readonly-queue/main/pr-<number>-<sha>` ref. The controller
checks only Git objects and compares the complete candidate tree with the
trusted base. It accepts one canonical version record added by the registrar
in a pull request, or a group of independently verified version additions at
the queue SHA. It checks the provider's formal tag, all ZIPs, protected branch
ancestry and the successful provider run's referenced reusable-workflow
revision against the trusted registry history. It also matches the run title
to the trigger version and requires a completed tag publication step from
that run. It reads jobs across run attempts so a later no-op rerun cannot
prove the release or hide an earlier successful tag write. The verifier reads
the candidate as Git data.
[GitHub workflow jobs API](https://docs.github.com/en/rest/actions/workflow-jobs#list-jobs-for-a-workflow-run)

Set `REGISTRY_REGISTRATION_BOT` to the registrar App login including
`[bot]`. Configure the `registry-check` environment for protected
`main` only, with `REGISTRY_CHECK_APP_CLIENT_ID` and
`REGISTRY_CHECK_APP_PRIVATE_KEY`. Give the independent check App Checks
write on the registry. The App reports `plugin-registration` on the exact
commit it inspected: GitHub's test merge commit before enqueueing and the
merge queue commit after enqueueing. If the candidate or the protected base
moves during verification, the workflow emits no successful check.

Use two active rulesets on `main` so the
registration requirement can be kept separate from maintainer review:

- Require pull requests, prevent force pushes and branch deletion, and require
  review by the repository's code owners for policy and release automation.
  Assign those paths, including `CODEOWNERS` itself, to the designated
  maintainers. Leave the general approval count at zero so registration files
  can merge automatically after verification.
- Require the merge queue and `plugin-registration` from the independent check
  App. Start with build concurrency of two and allow enough time for the next
  private scheduler dispatch and ZIP verification. The checker accepts at most
  32 registry additions per queue commit. GitHub's downstream merge limit
  does not cap the commits sent for checks. Give only the maintainer team a
  pull-request-only bypass in this ruleset so reviewed policy and workflow
  changes can merge. The registrar has no bypass in either ruleset.

After configuring the rulesets, verify that GitHub accepts both checks from
the expected App and merges two registration PRs through the queue. Also
verify that a policy PR requires code-owner review and can be merged by a
maintainer after review.
[GitHub rulesets](https://docs.github.com/en/repositories/configuring-branches-and-merges-in-your-repository/managing-rulesets/creating-rulesets-for-a-repository)
· [GitHub merge queue](https://docs.github.com/en/repositories/configuring-branches-and-merges-in-your-repository/configuring-pull-request-merges/managing-a-merge-queue)
· [GitHub required checks](https://docs.github.com/en/pull-requests/how-tos/merge-and-close-pull-requests/troubleshooting-required-status-checks#conflicts-between-head-commit-and-test-merge-commit)

The `registry-evidence` environment uses the dispatcher App with downgraded
Actions read and Contents read permissions to read provider run and branch
evidence. Its token cannot update provider repositories.

`registry/_policy/withdrawals.json` is a protected array of provider, version
and reason records. Generate with
`make index READY=/path/to/observations.json INDEX_STAGE=/path/to/stage`.
The independently audited observations enumerate `provider_type`, `version`
and `release_commit` that passed the default CDN gate. A ready list is not user
input and must not be derived from an unmerged registration PR. Generation
selects one stable version per provider, excludes withdrawn versions and leaves
a newly registered version out until it is ready. The generated `catalog.json`
is staging data, not a public index. Details and raster images are stored at
`market/details/<sha256>.json` and `market/icons/<sha256>.<format>` under
the eventual `registry/` publish directory. ZIP paths are always
`build/<os>_<arch>.zip` under the provider's pinned release commit.

`registryctl observe-release -record <file>` emits one ready observation only
after fully downloading and verifying every platform ZIP from the default CDN.
It requests files using the immutable release commit, not the formal tag.

The public `registry/index.json` is a JSON object with `key_id` and padded
standard-base64 `payload` and `signature`. The Ed25519 signature covers the
ASCII bytes `certimate:plugin-market:index:v1`, one zero byte, then the exact
decoded payload bytes. The payload has `format_version: 1`, `epoch`,
`generation`, UTC RFC 3339 `issued_at` and `expires_at`, the full merged
registry `source_commit` and `plugins`. Do not re-encode JSON for verification.
The payload limit is 2 MiB. `registryctl sign-index` regenerates the catalog
from the approved records and observations and emits the envelope.

### Signed market publication

`registry-publish.yml` runs when registry records or policy change on
protected `main`. Use the organization's private scheduler to dispatch
this workflow as well as `registry-reconcile.yml` on `main`. Scheduled
runs retry pending CDN observations and renew the index even when registry
records stay the same.

The publisher checks each provider's candidate ZIPs on the default CDN.
A candidate still propagating does not hold up updates or withdrawals for
other providers. A previously published version can remain visible while a
newer candidate propagates. At renewal, that version's ZIPs must pass the
CDN check again. The publisher verifies the previous index signature before
allocating the next generation. It retains previously published details and
icons for clients still reading their references.

Restrict the `registry-publish` environment to protected `main`. Set
`REGISTRY_INDEX_PUBLIC_KEYS_JSON` to the map of key IDs to base64 Ed25519
public keys and `REGISTRY_INDEX_KEY_ID` to the active key ID. Store its
base64 Ed25519 seed in the `REGISTRY_INDEX_SEED` environment secret. Keep
keys needed to verify the current market tip in the map during rotation, and
ship new trust keys to Core before signing with them. Configure
`REGISTRY_INDEX_EPOCH`, `REGISTRY_INDEX_LIFETIME` and
`REGISTRY_INDEX_RENEW_BEFORE` to match the host's trust policy. Measure
propagation on the default CDN and mainland mirrors before setting the
renewal window and dispatch interval.

Set `REGISTRY_MARKET_APP_CLIENT_ID` and `REGISTRY_MARKET_APP_PRIVATE_KEY`
for a dedicated App installed only on the registry. Configure branch rules
to limit its writes to `market` and forbid force pushes and deletions. The
workflow rechecks `main` and the verified `market` tip before pushing one
fast-forward commit containing the index and all addressed assets. The job
reports `published` only after verifying that exact signed index and every
asset it references on the default CDN. A job that reports `propagating`
must be observed again.

### Withdrawal and recovery

To withdraw a release, merge a maintainer-reviewed PR adding its provider,
version and reason to `registry/_policy/withdrawals.json`. Record the PR and
merged source SHA. Dispatch `registry-publish.yml` on `main` and check for a
higher generation. Verify its signed index on the default CDN and each enabled
mirror. Keep the withdrawal in subsequent releases. Publish a new version to
restore the plugin after a fix. The publisher can also select an earlier
eligible version after verifying its ZIPs. A stale mirror can keep serving the
previous snapshot until it refreshes or the signature expires. Installed
plugins are neither removed nor downgraded by a catalog withdrawal.

If an enabled mirror cannot serve the verified index or its files, switch the
Core mirror setting to a healthy origin serving the current generation and
refresh the official market. If publication is stale or failed, correct the
problem and dispatch from current protected `main` against the current
`market` tip. Keep the signed-index state in the Core database. Reverting the
market ref, resetting the generation, or clearing the database could defeat
the protection against stale snapshots.

### Signing key rotation

For a scheduled rotation, the key custodian generates a new Ed25519 key under
a new key ID. Add its public key alongside the current one to Core's built-in
trust map and deploy that Core release to the affected environments. Keep the
epoch. Add the new public key to `REGISTRY_INDEX_PUBLIC_KEYS_JSON`. Quiesce
publisher runs while the custodian updates `REGISTRY_INDEX_KEY_ID` and
`REGISTRY_INDEX_SEED`, then dispatch the publisher. The first snapshot signed
by the new key increments the previous generation. Once mirror audits show
the new snapshot and the old snapshot's validity window closes, remove the
retired key from the publisher's map and a subsequent Core release.

If a key may have been exposed, stop publisher dispatches, disable the publish
workflow, and cancel any in-flight signing runs. Confirm the current `market`
tip and `main` ancestry using GitHub audit evidence. If the market ref itself
may have been changed by an unauthorized actor, investigate and identify the
last trusted publication before proceeding. Prepare a Core release which
increments the built-in epoch and excludes the affected key. Configure the new
key and matching epoch for the publisher and keep the old public key in the
publisher's map temporarily to verify the protected market tip. Re-enable
publication, then verify a fast-forward snapshot at generation 1 of the new
epoch on the default CDN and enabled mirrors. Roll out the updated Core. Core
trusts a single epoch, so installations and updates may be unavailable during
propagation and rollout. Previously installed plugins can continue to run.
After the new tip is verified, remove the old public key from the publisher's
map and keep the evidence with the incident record.

For both procedures, retain the signing key ID, source and market commits,
epoch, generation, expiration time, workflow run and mirror reports. Verify
with an upgraded Core using an existing cache. For emergency rotation, confirm
that the new epoch is accepted even if the cached old epoch has a higher
generation, and the old envelope cannot be used again. Record an installation
or update through the verified mirror.

### Mirror admission

Run `registryctl audit-mirror` with trusted Git objects for the expected
`market` commit and its signed source commit. Use the public key map
from the signing environment. Run the command from each deployment network
and supply a network label that identifies where the request originated.

```sh
go run ./cmd/registryctl audit-mirror \
  -checkout "$REGISTRY_CHECKOUT" -market-sha "$EXPECTED_MARKET_SHA" \
  -public-keys "$PUBLIC_KEYS_FILE" -base-url https://cdn.jsdmirror.com \
  -network "$DEPLOYMENT_NETWORK" -provider volcengine-cdn \
  -provider volcengine-certcenter-purger -provider volcengine-dns
```

The command verifies the expected signed index before downloading the selected
providers' details, icons and all platform ZIPs. The JSON report includes the
signing key ID, generation, timestamps, response status, byte counts, SHA-256, cache headers,
redirect destination host and errors. An incomplete observation exits nonzero
and still emits its report. An older index leaves a failed index observation;
the command begins file downloads once the expected index is available.

To measure withdrawal propagation, pass the new `market` commit and add
`-withdrawn provider@vX.Y.Z` for each version being withdrawn. The command
checks that the exact version is covered by the withdrawal policy in the
signed source commit. It marks the withdrawal `confirmed` after the mirror
serves the expected signed index. If the withdrawn provider has a replacement
version in the index, the audit records it and automatically verifies its
details, icon and ZIPs for all platforms. A withdrawal audit can run without
`-provider`. Historical ZIP files may remain reachable at their immutable URLs.

Save observations for consecutive generations from the default CDN and each
candidate on both deployment networks. Repeat successful checks to observe
warm-cache behavior. Use issuance time and the first complete observation
to bound propagation. Record the operator's permitted usage and verify
installation in Core before adding a preset. Cross-origin redirects, including
redirects to GitHub Raw, do not count as availability.

Do not merge the new index command while the old automatic Release workflow
is enabled or still running. While its file is tracked on `main`, the signed
publisher uses its read-only Actions token to check that the legacy workflow
is manually disabled and its runs have drained. It repeats the check before
writing `market`. Remove the legacy workflow as part of the completed
migration.

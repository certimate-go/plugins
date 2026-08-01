# Releasing a plugin

Plugins are released automatically by the `release` workflow on every push to
`main`. There is no manual release step.

## How to ship a change

1. Make your code change in the plugin directory.
2. Bump the `version` field in `<plugin>/manifest.json` (semver: `major.minor.patch`).
3. Push to `main` (via a pull request).

The workflow detects the changed plugin, cross-compiles it for every supported
platform, computes per-binary SHA256 checksums, publishes a GitHub Release
tagged `<provider_type>/v<version>`, writes a `release` block into the plugin's
`manifest.json`, and regenerates `index.json`. The certimate market consumer
reads that index to resolve and verify downloads.

## The version-bump rule

A build-affecting change (Go source, `go.mod`/`go.sum`, anything under
`schema/`) **must** bump the plugin's `version`. If it does not, the workflow
fails the run with an actionable message, because the release tag
`<provider_type>/v<version>` already exists. A forgotten bump must never result
in a changed plugin silently not shipping.

## Metadata-only changes

Changes that do not affect the compiled binary — display name, icon, `usages`,
`priority` — do **not** need a version bump. The workflow regenerates
`index.json` so the metadata update is reflected, but no new release is cut.

## Trust preconditions

The whole integrity model rests on `main` staying trusted: the workflow
auto-publishes executable binaries and auto-commits the consumer-fetched
`index.json` on every push. Keep `main` under branch protection — no direct
pushes, pull requests require review, and tag pushing is restricted. The bot
commits use the default `GITHUB_TOKEN`, which does not re-trigger the workflow.

## Force a release

To release one or more plugins without a code change, run the workflow from the
Actions tab (`workflow_dispatch`) and pass the plugin directories in the
`plugins` input.

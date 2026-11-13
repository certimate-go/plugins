# Releasing a plugin

Plugins are released automatically by the `release` workflow on every push to
`main`. There is no manual release step.

## How to ship a change

1. Make your code change in the plugin directory.
2. Bump the `version` field in `<plugin>/manifest.json` (semver: `major.minor.patch`).
3. Push to `main` (via a pull request).

The workflow detects the changed plugin, cross-compiles it for every supported
platform, packages each platform build as a ZIP archive (`<plugin>_<os>_<arch>.zip`)
containing the plugin binary (named per the manifest `binary` field), the
plugin's `manifest.json`, and its icon, computes a per-ZIP SHA256 checksum,
publishes a GitHub Release tagged `<provider_type>/v<version>` with the ZIPs as
assets, writes a `release` block into the plugin's `manifest.json`, and
regenerates `index.json`. The certimate market consumer reads that index to
resolve, verify, and extract downloads.

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

## Distributing from your own market

Follow the [independent development guide](../skills/certimate-plugin-dev/references/third-party-development.md#add-github-actions-and-release-the-plugin) to install CI and release workflows in your repository. Push to `main` to publish the first version or a version bump. **Release plugin** creates the release in the current GitHub repository, uploads the platform ZIPs, and then updates the catalog. Use the manual workflow entry to retry from `main`. If your catalog lives elsewhere, point its details to the repository that actually hosts the release assets.

Publish the generated `index.json`, `market/details/`, and referenced icons in a public GitHub repository on `main`. Keep old detail files while cached indexes can refer to them. Copying an existing catalog carries over its original release references.

On a host that supports custom markets, sign in as a superuser and open **Plugins → Manage markets**. Enter the GitHub repository URL. Adding it fetches the catalog. Select the new source on the Plugins page, then choose **Install** on the intended card. Certimate saves the new source after it fetches and validates the catalog. If the add fails, fix the reported issue and try again.

The official badge identifies distribution through the built-in market. An installed provider can be updated from its recorded source. When another source lists the same provider, that listing cannot change the existing installation. Removing a market keeps its installed plugins available and suspends updates until you add the same repository again. You can uninstall the retained plugin from its original card.

Back up the database and plugin directory before upgrading. Restoring both from that backup is required before reverting to an older host after installing from custom markets. Those hosts lack source-aware update checks.

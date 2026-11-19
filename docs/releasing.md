# Releasing a plugin

## Releasing an official provider

1. Update `manifest.json` in the provider repository. Give every change to
   the package or its metadata a new stable `major.minor.patch` version.
2. Merge the release source into the protected `main` branch. Create and
   push `vX.Y.Z-detach` at that source commit.
3. In GitHub Actions, follow the **Release provider** run. The tag push
   dispatches the build on protected `main`. The workflow builds eight
   platform ZIPs and creates the immutable `vX.Y.Z` tag with the ZIPs and
   release metadata under `build/`.
4. The registry reconciliation workflow verifies the release and opens a PR
   with the version metadata. After the PR merges, the publication workflow
   generates the signed `index.json` and verifies it on the default CDN.

You can delete the `-detach` tag after the registration PR merges.
Retry a pending release through the protected reconciliation workflow. To
fix an already tagged version, release a new version. The market publication
job verifies the exact signed generation on the default CDN and reports
`published` when it can be read.

See the [migration inventory](provider-migration.md) for provider status and
the [release contract](release-contract.md) for protected workflows, GitHub
Apps, and the signature policy.

## Distributing from your own market

Follow the [independent development guide](../skills/certimate-plugin-dev/references/third-party-development.md#add-github-actions-and-release-the-plugin) to install CI and release workflows in your repository. Push to `main` to publish the first version or a version bump. **Release plugin** creates the release in the current GitHub repository, uploads the platform ZIPs, and then updates the catalog. Use the manual workflow entry to retry from `main`. If your catalog lives elsewhere, point its details to the repository that actually hosts the release assets.

Publish the generated `index.json`, `market/details/`, and referenced icons in a public GitHub repository on `main`. Keep old detail files while cached indexes can refer to them. Copying an existing catalog carries over its original release references.

On a host that supports custom markets, sign in as a superuser and open **Plugins → Manage markets**. Enter the GitHub repository URL. Adding it fetches the catalog. Select the new source on the Plugins page, then choose **Install** on the intended card. Certimate saves the new source after it fetches and validates the catalog. If the add fails, fix the reported issue and try again.

The official badge identifies distribution through the built-in market. An installed provider can be updated from its recorded source. When another source lists the same provider, that listing cannot change the existing installation. Removing a market keeps its installed plugins available and suspends updates until you add the same repository again. You can uninstall the retained plugin from its original card.

Back up the database and plugin directory before upgrading. Restoring both from that backup is required before reverting to an older host after installing from custom markets. Those hosts lack source-aware update checks.

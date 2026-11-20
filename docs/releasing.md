# Releasing a plugin

## Releasing an official provider

1. Update `version` in the provider's `manifest.json` and merge the source PR
   into the provider's protected `main`. Use a new stable `major.minor.patch` version.
2. From an up-to-date `plugins` checkout, request the release by provider type
   and version. Use Git credentials with permission to create the request tag.

   ```sh
   make release-provider REPOSITORY=certimate-go/plugin-provider-volcengine-dns-acmedns01challenger VERSION=0.1.2
   ```

   The command reads the repository’s merged manifest
   and pushes `v0.1.2-detach` at that commit. Add `DRY_RUN=1` to preview the
   commit and tag without pushing.
3. Follow **Release provider** until it creates the formal tag. **Register
   provider releases** reads the tag on its next scheduled run and opens a
   PR in `plugins`. Merge the registration PR.
4. **Publish signed plugin market** generates `registry/index.json` and
   pushes the generated files directly to `main`. Install the version once
   the CDN serves the new index.

You can delete the `-detach` tag after the formal tag is available. Rerun a
failed build in the provider repository. To rebuild a published version, dispatch **Release provider** on `main` with the original request tag and `rebuild=true`. The workflow uploads the rebuilt ZIPs to the run and preserves the formal tag. To fix an already tagged version,
release a new version. Run **Register provider releases** manually to skip the
wait for the schedule.

See the [migration inventory](provider-migration.md) for provider status and
the [release contract](release-contract.md) for repository settings and the
signature policy.

## Distributing from your own market

Follow the [independent development guide](../skills/certimate-plugin-dev/references/third-party-development.md#add-github-actions-and-release-the-plugin) to install CI and release workflows in your repository. Push to `main` to publish the first version or a version bump. **Release plugin** creates the release in the current GitHub repository, uploads the platform ZIPs, and then updates the catalog. Use the manual workflow entry to retry from `main`. If your catalog lives elsewhere, point its details to the repository that actually hosts the release assets.

Publish the generated `index.json`, `market/details/`, and referenced icons in a public GitHub repository on `main`. Keep old detail files while cached indexes can refer to them. Copying an existing catalog carries over its original release references.

On a host that supports custom markets, sign in as a superuser and open **Plugins → Manage markets**. Enter the GitHub repository URL. Adding it fetches the catalog. Select the new source on the Plugins page, then choose **Install** on the intended card. Certimate saves the new source after it fetches and validates the catalog. If the add fails, fix the reported issue and try again.

The official badge identifies distribution through the built-in market. An installed provider can be updated from its recorded source. When another source lists the same provider, that listing cannot change the existing installation. Removing a market keeps its installed plugins available and suspends updates until you add the same repository again. You can uninstall the retained plugin from its original card.

Back up the database and plugin directory before upgrading. Restoring both from that backup is required before reverting to an older host after installing from custom markets. Those hosts lack source-aware update checks.

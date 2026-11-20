# Official plugin release contract

## Configure a provider repository

An official provider lives in its own public repository. Name it
`certimate-go/plugin-provider-<service>-<role>`. Choose `deployer`,
`purger`, `notifier`, or `acmedns01challenger` for the manifest capability.

Copy the [CI template](../skills/certimate-plugin-dev/assets/github-actions/official-provider-ci.yml)
and the [release template](../skills/certimate-plugin-dev/assets/github-actions/official-provider-release.yml)
to the provider's `.github/workflows/ci.yml` and `.github/workflows/release.yml`.
Protect the provider's `main` branch. Allow the repository's Actions token
to create formal tags. Keep tag rules that prevent updates and deletion of
formal tags. Remove the old rule that restricted tag creation to a dedicated
App. The release job uses the provider repository's `GITHUB_TOKEN`.

## Publish a provider version

Push `vX.Y.Z-detach` at a commit in the protected `main` history whose
`manifest.json` has version `X.Y.Z`. The tag workflow dispatches the release
workflow on `main` in the same provider repository. The build job checks out
the request commit and compiles the eight supported platform ZIPs with a
read-only token and Go workspace mode off.

The publish job writes `build/` to a Git commit whose parent is the source
commit. It pushes `vX.Y.Z` at that commit. The formal tag contains eight ZIPs
and `build/release.json` with the source SHA, build run, and each ZIP's
path, size, and SHA-256. Each ZIP includes the binary, platform manifest,
and declared runtime icon. ZIPs fit within 20,000,000 bytes each and the
release Git tree fits within 150,000,000 bytes. Rerunning a published
request reuses the same formal tag. Delete the request tag after the formal
tag is available if you want to clean it up.

## Register and publish the market index

Enable **Allow GitHub Actions to create and approve pull requests** for
`certimate-go/plugins`. Allow the repository's `GITHUB_TOKEN` to fast-forward
`main`. The existing rule that requires all changes to go through a PR, the
`plugin-registration` required check, and the merge queue block that push.
Adjust those rules before enabling publication. Keep protections against
force-pushes and branch deletion.

**Register provider releases** runs every ten minutes and on demand. It
discovers public `certimate-go/plugin-provider-*` repositories and checks each new formal tag’s
release record and ZIPs, and opens one PR per release in `certimate-go/plugins`
with `registry/<provider_type>/vX.Y.Z.json`. It uses the registry repository's
`GITHUB_TOKEN` to write only to its own repository. Merge the registration
PR into `main`. Keep existing version records and their formal tags immutable.

**Publish signed plugin market** starts after a registry change lands on
`main`. It builds and signs the index from merged records, then pushes a
commit containing `registry/index.json` and its content-addressed files
directly to `main`. The same push creates tag `v<epoch>.<generation>.0` at
that commit. jsDelivr `@latest` follows the highest semver tag in this
repository, so this tag has to stay the highest one. The job then asks
`purge.jsdelivr.net` to refresh `@latest` for the index, details, and icons
whose CDN body does not match the commit. A finished purge does not replace
that byte check. If `main` advances during generation, Git rejects the push
and the next run generates from the new tip. Scheduled runs retry CDN
propagation and renew the index before it expires.

Restrict the `registry-publish` environment to protected `main`. In that
environment, set `REGISTRY_INDEX_PUBLIC_KEYS_JSON`, `REGISTRY_INDEX_KEY_ID`,
`REGISTRY_INDEX_EPOCH`, `REGISTRY_INDEX_LIFETIME`, and
`REGISTRY_INDEX_RENEW_BEFORE`. Store the base64 Ed25519 seed in
`REGISTRY_INDEX_SEED`. When rotating keys, keep the previous public key during
the transition and ship the new key to Core before using it.

[Signed index](https://cdn.jsdelivr.net/gh/certimate-go/plugins@latest/registry/index.json)

To withdraw a release, merge an entry for the provider and version in
`registry/_policy/withdrawals.json`. The next index excludes the withdrawn
version. Installed plugins remain in place.

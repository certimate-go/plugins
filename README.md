# certimate plugins

English | [中文](README.zh.md)

Out-of-process provider plugins for [certimate](https://github.com/certimate-go/certimate). Each plugin is a standalone binary that certimate launches as a gRPC subprocess ([hashicorp/go-plugin](https://github.com/hashicorp/go-plugin)) to deploy, resolve, notify, or purge certificates. Adding a plugin requires **no changes to the certimate core**.

This repository runs the official plugin registry. Admitted plugins publish ZIPs from their own provider repositories. Certimate reads the signed `registry/index.json` on `main` through the CDN and downloads ZIPs from each provider's pinned release commit. [Signed index](https://cdn.jsdelivr.net/gh/certimate-go/plugins@main/registry/index.json)

## Contribute a plugin

For an existing official plugin, find its repository in the admissions list, fork it to your personal account, and open a pull request against its `main` branch. [Official admissions](https://github.com/certimate-go/plugins/blob/main/registry/_policy/admissions.json)

For a new official plugin, open an issue with the target service, plugin capability, proposed `provider_type`, and how you tested it or plan to test it. If you have code, include the public repository, commit, and license. If the maintainers accept the proposal, they create and configure the official repository. Fork that repository and submit your source in a pull request. Maintainers handle admission and the first release after merging the source. [Request official inclusion](https://github.com/certimate-go/plugins/issues/new)

To publish independently from your own repository, follow the [custom market guide](skills/certimate-plugin-dev/references/third-party-development.md).

## Plugin anatomy

Each plugin implements the unified `Plugin` interface defined in `certimate/pkg/plugin`:

- `GetMetadata` — identity: provider type, access type, capability, category, display name.
- `GetConfigSchema` — form/v1 provider (and optionally access) schema + i18n.
- `Process` — performs the capability's work with access config, provider config, and a typed payload.
- `PostProcess` — finalization for the same execution (embed `plugin.NoopFinalizer` when the capability needs no cleanup).

The source `manifest.json` describes the plugin.

| field | meaning |
|---|---|
| `provider_type` | Stable plugin ID in kebab case |
| `access_provider_type` | the access credential type this plugin consumes |
| `version` | Semantic version that must match the official release trigger tag |
| `capability` | one of `deploy`, `dns`, `notify`, `purge` |
| `category` | market grouping (e.g. `cdn`, `storage`, `other`) |
| `binary` | built binary name |
| `icon` | icon filename shipped with the plugin |
| `protocol_version` | must match the certimate plugin protocol |
| `usages` / `priority` | market display hints |

## Develop a plugin

The repository includes a [certimate-plugin-dev skill](skills/certimate-plugin-dev/SKILL.md) for AI coding assistants developing deploy, DNS, notification, and purge plugins. It covers credential reuse, forms and Chinese/English translations, builds, and verification through the real host. From this repository, ask your assistant:

```text
Read skills/certimate-plugin-dev/SKILL.md and develop a <deploy/dns/notify/purge> plugin for <platform>.
Expected behavior: <deployment target / DNS service / notification channel / expired-certificate purge>; API docs: <URL>; credentials: <existing type or new>.
```

To install in Codex, run these commands from this repository root to copy the entire skill into your personal skills directory. Back up an existing installation with the same name before updating it.

```sh
mkdir -p "${CODEX_HOME:-$HOME/.codex}/skills"
cp -R skills/certimate-plugin-dev "${CODEX_HOME:-$HOME/.codex}/skills/"
```

Once installed, open your own project and ask: “Use `$certimate-plugin-dev` to build a DNS plugin for my provider and prepare this repository as a custom market.” For other assistants supporting `SKILL.md`, copy the entire directory into their skills directory. See the [custom market setup](docs/releasing.md#distributing-from-your-own-market).

Use `_template` as a source example. For an official provider, place `go.mod`, `manifest.json`, the Go entry point, and `schema/` at the root of its repository. Put the icon file there and set `manifest.icon` to its filename. Run `go test ./...` in the provider repository.

A plugin may declare a **new** access type (add `schema/access.json`) or reuse an existing one (built-in or from another plugin).

## Build plugins still in this repository

```bash
make build PLUGIN=my-deployer         # installable host package -> dist/<plugin>/
make build-all PLUGIN=my-deployer     # cross-compile the release matrix
make test                             # go test ./...
```

## Release an official plugin

After the source PR and any first-time admission PR merge, a maintainer pushes a `vX.Y.Z-detach` tag pointing to the provider's protected `main` revision, matching the version in `manifest.json`. The release workflow builds eight platform ZIPs and publishes an immutable `vX.Y.Z` tag containing `build/`. The registry merges a version registration PR, then a separate PR with the signed market snapshot. The release is available in the market when the CDN serves the verified snapshot. [Official release contract](https://github.com/certimate-go/plugins/blob/main/docs/release-contract.md)

## Upgrading from built-in purge providers

The sixteen certificate purge providers that formerly shipped inside certimate core live in this repository as `purge` capability plugins: `aliyun-cas-purger`, `aliyun-clb-purger`, `aliyun-esa-purger`, `aws-acm-purger`, `aws-iam-purger`, `azure-keyvault-purger`, `baiducloud-cert-purger`, `huaweicloud-elb-purger`, `huaweicloud-scm-purger`, `huaweicloud-waf-purger`, `jdcloud-ssl-purger`, `tencentcloud-gaap-purger`, `tencentcloud-ssl-purger`, `tencentcloud-tse-purger`, `ucloud-ussl-purger`, and `volcengine-certcenter-purger`.

A workflow node that references a legacy purge provider keeps working once the mapped plugin is installed — certimate resolves the legacy provider ids to these plugins automatically. Until the plugin is installed the node fails with a missing-provider error. certimate never downloads plugins or rewrites workflows unprompted: install the plugin explicitly from the plugin market (or a custom market), and re-save a workflow only if you want its node to carry the new provider id.

## Tooling

- `cmd/registryctl` handles official release verification, registration, and signed index publication.
- `cmd/releaseinfo` writes Release asset metadata for custom markets.
- `make clean` removes `dist/`.

## Requirements

- Go 1.25+ (`GOTOOLCHAIN=auto` fetches the required toolchain).
- Depends on `github.com/certimate-go/certimate/pkg/plugin` for the `Plugin` interface and handshake.

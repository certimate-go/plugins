# certimate plugins

English | [中文](README.zh.md)

Out-of-process provider plugins for [certimate](https://github.com/certimate-go/certimate). Each plugin is a standalone binary that certimate launches as a gRPC subprocess ([hashicorp/go-plugin](https://github.com/hashicorp/go-plugin)) to deploy, resolve, notify, or purge certificates. Adding a plugin requires **no changes to the certimate core**.

This repository indexes official plugins. Provider repositories build and publish their own releases. Certimate reads the [signed index](https://cdn.jsdelivr.net/gh/certimate-go/plugins@main/registry/index.json).

## Repository layout

```
plugins/
├── <plugin-name>/        one directory per plugin
│   ├── manifest.json       identity + market metadata
│   ├── main.go             plugin.Serve entrypoint
│   ├── server.go           unified Plugin interface implementation
│   ├── schema/             form/v1 provider (+ optional access) schema and i18n
│   └── *_test.go
├── _template/            scaffold for new plugins
├── cmd/
│   ├── genindex/           regenerates index.json from manifests
│   └── releaseinfo/        writes the release block (sha256/assets) into a manifest
├── internal/ pkg/        shared plugin helpers
├── docs/releasing.md     how releases work
├── index.json            generated market index (consumed by certimate)
└── Makefile
```

## Plugin anatomy

Each plugin implements the unified `Plugin` interface defined in `certimate/pkg/plugin`:

- `GetMetadata` — identity: provider type, access type, capability, category, display name.
- `GetConfigSchema` — form/v1 provider (and optionally access) schema + i18n.
- `Process` — performs the capability's work with access config, provider config, and a typed payload.
- `PostProcess` — finalization for the same execution (embed `plugin.NoopFinalizer` when the capability needs no cleanup).

`manifest.json` carries the market metadata:

| field | meaning |
|---|---|
| `provider_type` | unique plugin id (kebab-case); also the directory and binary name |
| `access_provider_type` | the access credential type this plugin consumes |
| `version` | semver; bumping it is what triggers a new release |
| `capability` | one of `deploy`, `dns`, `notify`, `purge` |
| `category` | market grouping (e.g. `cdn`, `storage`, `other`) |
| `binary` | built binary name |
| `icon` | icon filename shipped with the plugin |
| `protocol_version` | must match the certimate plugin protocol |
| `usages` / `priority` | market display hints |
| `release` | generated pointer to the GitHub Release (repo/tag/assets/checksums) |

## Add a plugin

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

```bash
make init PLUGIN=my-deployer          # scaffold from _template
```

Then edit `my-deployer/manifest.json` (set `provider_type`, `access_provider_type`, `capability`, `category`, `icon`, ...), implement `Process()` in `server.go`, and define `schema/config.json`. Drop the icon file (svg/png) into the plugin directory and point `manifest.icon` at it.

A plugin may declare a **new** access type (add `schema/access.json`) or reuse an existing one (built-in or from another plugin).

## Build & test

```bash
make build PLUGIN=my-deployer         # installable host package -> dist/<plugin>/
make build-all PLUGIN=my-deployer     # cross-compile the release matrix
make test                             # go test ./...
```

`build-all` produces stripped, reproducible binaries (`CGO_ENABLED=0 -trimpath -ldflags='-s -w'`) named the way the certimate market downloads them — `<plugin>_<os>_<arch>` (`<plugin>_windows_amd64.exe` on Windows) — for `linux/amd64`, `linux/arm64`, `darwin/amd64`, `darwin/arm64`, `windows/amd64`.

## Releasing

Push a release request tag in the provider repository. After its formal tag appears, this registry opens a version PR. Merging it triggers publication to `main`. [Official release guide](docs/releasing.md)

## Upgrading from built-in purge providers

The sixteen certificate purge providers that formerly shipped inside certimate core are available as `purge` capability plugins: `aliyun-cas-purger`, `aliyun-clb-purger`, `aliyun-esa-purger`, `aws-acm-purger`, `aws-iam-purger`, `azure-keyvault-purger`, `baiducloud-cert-purger`, `huaweicloud-elb-purger`, `huaweicloud-scm-purger`, `huaweicloud-waf-purger`, `jdcloud-ssl-purger`, `tencentcloud-gaap-purger`, `tencentcloud-ssl-purger`, `tencentcloud-tse-purger`, `ucloud-ussl-purger`, and `volcengine-certcenter-purger`.

A workflow node that references a legacy purge provider keeps working once the mapped plugin is installed — certimate resolves the legacy provider ids to these plugins automatically. Until the plugin is installed the node fails with a missing-provider error. certimate never downloads plugins or rewrites workflows unprompted: install the plugin explicitly from the plugin market (or a custom market), and re-save a workflow only if you want its node to carry the new provider id.

## Tooling

- `make index` → `cmd/genindex` regenerates `index.json` from all manifests.
- `cmd/releaseinfo` computes checksums and writes the `release` block (used by the release workflow).
- `make clean` removes `dist/`.

## Requirements

- Go 1.25+ (`GOTOOLCHAIN=auto` fetches the required toolchain).
- Depends on `github.com/certimate-go/certimate/pkg/plugin` for the `Plugin` interface and handshake.

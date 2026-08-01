# certimate plugins

English | [中文](README.zh.md)

Out-of-process deployer plugins for [certimate](https://github.com/certimate-go/certimate). Each plugin is a standalone binary that certimate launches as a gRPC subprocess ([hashicorp/go-plugin](https://github.com/hashicorp/go-plugin)) to deploy certificates to a target platform. Adding a plugin requires **no changes to the certimate core**.

This repository is also the plugin **market** source: it builds plugins, publishes them as GitHub Releases, and serves a market index (`index.json`) that certimate's plugin-market UI reads to list, install, and update plugins.

## Repository layout

```
plugins/
├── <plugin-name>/        one directory per plugin
│   ├── manifest.json       identity + market metadata
│   ├── main.go             plugin.Serve entrypoint
│   ├── server.go           Deployer interface implementation
│   ├── schema/             form/v1 deploy (+ optional access) schema and i18n
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

Each plugin implements the `Deployer` interface defined in `certimate/pkg/plugin`:

- `GetMetadata` — identity: provider type, access type, deploy category, display name.
- `GetConfigSchema` — form/v1 deploy (and optionally access) schema + i18n.
- `Deploy` — performs the deployment with access config, deploy config, and certificate PEM.

`manifest.json` carries the market metadata:

| field | meaning |
|---|---|
| `provider_type` | unique plugin id (kebab-case); also the directory and binary name |
| `access_provider_type` | the access credential type this plugin consumes |
| `version` | semver; bumping it is what triggers a new release |
| `deploy_category` | market grouping (e.g. `cdn`, `storage`, `other`) |
| `binary` | built binary name |
| `icon` | icon filename shipped with the plugin |
| `protocol_version` | must match the certimate plugin protocol |
| `usages` / `priority` | market display hints |
| `release` | generated pointer to the GitHub Release (repo/tag/assets/checksums) |

## Add a plugin

```bash
make init PLUGIN=my-deployer          # scaffold from _template
```

Then edit `my-deployer/manifest.json` (set `provider_type`, `access_provider_type`, `deploy_category`, `icon`, ...), implement `Deploy()` in `server.go`, and define `schema/deploy.json`. Drop the icon file (svg/png) into the plugin directory and point `manifest.icon` at it.

A plugin may declare a **new** access type (add `schema/access.json`) or reuse an existing one (built-in or from another plugin).

## Build & test

```bash
make build PLUGIN=my-deployer         # host binary -> dist/<plugin>
make build-all PLUGIN=my-deployer     # cross-compile the release matrix
make test                             # go test ./...
```

`build-all` produces stripped, reproducible binaries (`CGO_ENABLED=0 -trimpath -ldflags='-s -w'`) named the way the certimate market downloads them — `<plugin>_<os>_<arch>` (`<plugin>_windows_amd64.exe` on Windows) — for `linux/amd64`, `linux/arm64`, `darwin/amd64`, `darwin/arm64`, `windows/amd64`.

## Releasing

Releases are **change-driven and version-gated**. On push to `main`, the `release` workflow detects which plugins changed; for each build-affecting change whose `version` was bumped, it cross-compiles the plugin, publishes a GitHub Release tagged `<provider_type>/v<version>`, computes per-platform SHA256 checksums, and writes a `release` block into the manifest. `genindex` then regenerates `index.json` so the certimate market can resolve and verify downloads.

- To ship a change: change the code, **bump `version`** in the plugin's `manifest.json`, push to `main`.
- A build-affecting change (Go source, `go.mod`/`go.sum`, `schema/`) without a version bump fails the run.
- Metadata-only changes (icon, display name) only regenerate the index; no release.

See [`docs/releasing.md`](docs/releasing.md) for the full flow and trust preconditions.

## Tooling

- `make index` → `cmd/genindex` regenerates `index.json` from all manifests.
- `cmd/releaseinfo` computes checksums and writes the `release` block (used by the release workflow).
- `make clean` removes `dist/`.

## Requirements

- Go 1.25+ (`GOTOOLCHAIN=auto` fetches the required toolchain).
- Depends on `github.com/certimate-go/certimate/pkg/plugin` for the `Deployer` interface and handshake.

# Writing a Provider Plugin

A provider plugin is a standalone Go binary that Certimate's core loads at runtime over gRPC (via `hashicorp/go-plugin`). Every capability — `deploy`, `dns`, `notify`, `purge` — uses the same interface and the same wire contract. Adding a new provider requires **zero changes to the core** — you only write the plugin and drop it into the plugin directory.

## Run a standalone plugin locally

From the `plugins` checkout, build a neighboring provider repository into a local plugin directory. Initialize its submodules first if it uses any.

```sh
make dev PROVIDER_DIR=../plugin-provider-volcengine-dns-acmedns01challenger
```

The command prints the absolute `CERTIMATE_PLUGIN_DIR` to use when starting Core with a test data directory. Rerun the same command after editing the provider. To use another development plugin directory, set `PLUGIN_ROOT`. Run `go test ./...` in the provider repository when you need its tests. Provider CI checks the source PR.

This guide walks through the full author flow using `_template/` as the reference.

## Prerequisites

- Go 1.25+
- The plugin imports the protocol from Certimate:

  ```
  require github.com/certimate-go/certimate <version>
  ```

  Pin a version compatible with your target Certimate host. For a standalone repository, use the [independent development guide](../skills/certimate-plugin-dev/references/third-party-development.md) for project setup, capability-specific examples, and host verification. Keep local overrides in a separate Go workspace.

## 0. Scaffold a new plugin

Run `make init` to generate a complete plugin skeleton:

```bash
make init PLUGIN=my-provider
```

Copy the generated `my-provider/` scaffold into your standalone Go module.

```
my-provider/
  manifest.json       # metadata with placeholders filled in
  main.go             # plugin.Serve boilerplate (no changes needed)
  server.go           # unified Plugin interface skeleton (includes //go:embed schema)
  server_test.go      # test skeleton with schema/i18n coverage
  schema/
    config.json       # form/v1 provider schema (edit to define your form)
    access.json       # form/v1 access schema (delete if reusing an existing access type)
    i18n/
      zh.json         # Chinese translations
      en.json         # English translations
```

After scaffolding, fill in `access_provider_type` and `capability` in `manifest.json`:
- To **reuse an existing access type** (recommended): set it to the existing type name (e.g., `"webhook"`, `"aliyun"`) and delete `schema/access.json`.
- To **define a new access type**: set it to your new type name and edit `schema/access.json` to define its credential form.

## 1. Implement the Plugin

Implement `pkg/plugin.Plugin` — four methods, shared by every capability:

```go
type Plugin interface {
    GetMetadata(ctx context.Context) (*plugin.Metadata, error)
    GetConfigSchema(ctx context.Context) (*plugin.ConfigSchema, error)
    Process(ctx context.Context, req *plugin.Request, logger *slog.Logger) (*plugin.Result, error)
    PostProcess(ctx context.Context, req *plugin.Request, logger *slog.Logger) error
}
```

- **GetMetadata** declares identity: `ProviderType` (plugin-owned and unique), `AccessProviderType` (the credential type this provider references), `Capability` (`deploy`, `dns`, `notify`, or `purge`), `ProtocolVersion` (must equal `plugin.ProtocolVersion`), the neutral `Category`, and the i18n key for the display name.
- **GetConfigSchema** returns the `form/v1` provider schema as JSON bytes (`ProviderSchemaJSON`) and an i18n resource bundle (`I18n`). Use the shared helpers from `pkg/plugin` (`plugin.LoadProviderSchema(schemaFS)`, `plugin.LoadAccessSchema(schemaFS)`, `plugin.LoadI18n(schemaFS)`). The access schema (`AccessSchemaJSON`) is **optional** — see "Access types: new vs. reused" below.
- **Process** performs the work. `req` carries `ExecutionID`, `Capability`, `LogLevel`, `AccessConfigJSON` / `ExtendedConfigJSON` (the two-segment opaque config), and exactly one typed payload matching the capability:
  - `req.Deploy` — `CertificatePEM` / `PrivateKeyPEM`
  - `req.Dns` — `Domain` / `Token` / `KeyAuth`
  - `req.Notify` — `Subject` / `Message`
  - `req.Purge` — `ExpirySeconds`

  Deserialize the config segments with your own structs and return a `Result` with the matching result arm, e.g. `&plugin.Result{Deploy: &plugin.DeployResult{ExtendedDataJSON: ...}}`. A `nil` result is a protocol violation even on success.
- **PostProcess** finalizes the same execution. The host calls it exactly once per execution after `Process` returns or its stream is cancelled, with a context detached from caller cancellation (default budget 30s). Embed `plugin.NoopFinalizer` when your capability needs no cleanup — that is the common case for deploy, notify, and purge. DNS providers implement it to remove the TXT record created in `Process`.

A provider may additionally implement `plugin.AccessChecker` to support the access connectivity test introduced with protocol v4:

```go
CheckAccess(ctx context.Context, req *plugin.AccessCheckRequest, logger *slog.Logger) (*plugin.AccessCheckResult, error)
```

The core detects the implementation automatically and exposes the test button in the UI — no manifest or metadata change is needed. `req` carries only `AccessConfigJSON`. Return an `AccessCheckResult` with an overall `Status` (`passed`, `failed`, `inconclusive`), an optional machine-readable `ReasonCode`, per-aspect `Items` (conventionally `connect` and `auth`, each `passed`/`failed`/`skipped`), and human-readable `Diagnostics` that never contain secrets. Validate the config first, probe with the cheapest read-only authenticated API call, and classify failures (connectivity, TLS, authentication, remote error, timeout, cancellation) so the UI can attribute the cause; return a result rather than an error for every checkable outcome. The host redacts secrets from logs and error messages. See `apisix/access_check.go` for the reference implementation and `integration/apisix_access_check_test.go` for its end-to-end coverage.

Honor `ctx` in both methods: when the caller cancels, the host still waits (bounded by the finalization budget) for your handler to return.

### Access types: new vs. reused

A provider does **not** have to invent its own credential type. `AccessProviderType` may reference:

- **A new, plugin-owned access type** — return a non-empty `AccessSchemaJSON` from `plugin.LoadAccessSchema()`. The core registers the schema and the type appears in the access picker. Use this only if no existing access type fits.
- **An existing access type** (built-in, or one another plugin already declared) — return an empty `AccessSchemaJSON`. The core reuses the existing access type's schema and picker entry; your `Process` must parse that type's config shape. This is how multiple plugins share one credential (e.g. several Aliyun providers all read an `aliyun` access), and how a deploy plugin shares a credential with `notify`/`apply` (e.g. the `webhook-deployer` pilot reuses the built-in `webhook` access that notifications also use).

When reusing an existing access type, one credential record can serve every consumer. The owner and consumer may come from separate markets. Match the owner’s credential fields, install the owner first, and remove consumers before uninstalling the owner.

Errors you return are transported to the core and surfaced to the user — return plain `fmt.Errorf(...)`.

## 2. Define the form schema

Edit `schema/config.json` — a JSON object in the `form/v1` envelope shape:

```json
{
  "schemaVersion": "form/v1",
  "provider": "my-provider",
  "category": "deploy",
  "schema": { "columns": [ { "name": "...", "valueType": "text", "labelKey": "...", ... } ] }
}
```

`valueType` is one of: `text`, `textarea`, `select`, `radio`, `switch`, `number`, `secret`, `code`, `autocomplete`. Fields flagged `"secret": true` are masked in the UI. Every `labelKey` / `placeholderKey` / `tooltipKey` must resolve via your i18n bundle (step 3) or the renderer falls back to showing the raw key.

The `category` field is included for spec conformance — the core overrides it at registration time according to the plugin capability.

If you define a new access type, edit `schema/access.json` in the same format.

## 3. Provide i18n

Edit `schema/i18n/zh.json` and `schema/i18n/en.json` — flat key-value JSON files. Use fully-qualified dotted keys namespaced under `plugin.<providerType>`:

```
plugin.my-provider.name                       → display name
plugin.my-provider.description                → market description
plugin.my-provider.access.apiKey.label        → access field label
plugin.my-provider.deploy.target.label        → provider field label
```

To add a new locale, create a new JSON file in `schema/i18n/` (e.g., `ja.json`) — no Go code changes needed. The `plugin.LoadI18n` helper discovers locales at runtime by reading the directory.

The core injects these into the SPA's i18n store under the default namespace at the `plugin.<type>` path, so `t("plugin.<type>.foo")` resolves in pickers and dynamic forms. Missing locale/key falls back to the raw key (graceful degradation).

For a market description, add `plugin.<providerType>.description` in each language file and keep the English text in `manifest.json` as the `description` fallback. The official catalog includes the translated name and description when it publishes the receipt, using `display_name_key` for the name. Users can read these translations and search by name before installing the plugin. The host selects the requested locale, its base language, then English, and falls back to the provider identifier or manifest description when a translation is absent.

## 4. Write the manifest

`manifest.json` sits next to the binary. The scaffolding command fills in `provider_type`, `binary`, and `display_name_key` automatically. You must fill in `access_provider_type` and `capability`:

```json
{
  "version": "0.1.0",
  "provider_type": "my-provider",
  "access_provider_type": "webhook",
  "capability": "deploy",
  "display_name_key": "plugin.my-provider.name",
  "category": "other",
  "protocol_version": 4,
  "min_core_version": "",
  "max_core_version": "",
  "binary": "my-provider",
  "icon": "",
  "sha256": "",
  "priority": 0,
  "usages": ["hosting"],
  "description": ""
}
```

- `capability` selects the payload your `Process` receives: `deploy`, `dns`, `notify`, or `purge`.
- `protocol_version` must match the core's current protocol (hard gate; mismatched plugins are rejected with a readable error and never executed).
- `min_core_version` / `max_core_version` are optional soft gates.
- `binary` is the executable filename (same directory). The core refuses to load if the binary or directory is group/world-writable.
- `icon` optionally references an image file in the same directory; omit for the default placeholder.
- `sha256` is advisory (mismatch warns but does not block).
- `usages` declares the access type usage categories. Valid values: `dns`, `hosting`, `notification`, `ca`.

## 5. Serve the plugin

`main.go` calls `githubplugin.Serve` with the shared handshake config. The scaffolding generates this file — no changes needed:

```go
githubplugin.Serve(&githubplugin.ServeConfig{
    HandshakeConfig: plugin.HandshakeConfig,
    Plugins: map[string]githubplugin.Plugin{
        plugin.PluginName: &plugin.GRPCPlugin{Impl: &server{}},
    },
    GRPCServer: githubplugin.DefaultGRPCServer,
})
```

## 6. Build and install

Build from the standalone checkout with `make dev PROVIDER_DIR=/path/to/provider`. The output under `PLUGIN_ROOT` contains the manifest, the executable named by `binary`, and the declared icon. Launch Core with the printed `CERTIMATE_PLUGIN_DIR`. A manual installation uses the same per-provider directory layout.

The Core reloader detects package changes. In-flight executions continue on their previous runtime, and a package that fails to start leaves the working runtime available.

Run the provider tests in its standalone module. Use a temporary Go workspace outside that repository when testing against a local Core checkout. Keep the published Core module pin in the provider's `go.mod`.

Official releases build eight platform ZIPs through the reusable release workflow. Use a `binary` name ending in `.exe` for the Windows targets. Follow the [release guide](releasing.md) for registration and the signed catalog.

## Verification

After load: the provider appears in the picker for its capability (tagged "Plugin"), its dynamic config form renders with translated labels, and an execution routes through the plugin subprocess with streamed logs and a finalization pass. See the core's `internal/pluginhost` E2E tests and this repo's `integration` workflow tests for the full host-to-plugin path.

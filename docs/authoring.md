# Writing a Deploy Provider Plugin

A deploy provider plugin is a standalone Go binary that Certimate's core loads at runtime over gRPC (via `hashicorp/go-plugin`). Adding a new deploy provider requires **zero changes to the core** — you only write the plugin and drop it into the plugin directory.

This guide walks through the full author flow using `webhook-deployer/` as the reference.

## Prerequisites

- Go 1.25+
- The plugin imports the protocol from Certimate:

  ```
  require github.com/certimate-go/certimate <version>
  ```

  During local development, add a replace to your local checkout:

  ```
  replace github.com/certimate-go/certimate => ../certimate
  ```

## 0. Scaffold a new plugin

Run `make init` to generate a complete plugin skeleton:

```bash
make init PLUGIN=my-deployer
```

This creates `my-deployer/` with:

```
my-deployer/
  manifest.json       # metadata with placeholders filled in
  main.go             # plugin.Serve boilerplate (no changes needed)
  server.go           # Deployer interface skeleton (includes //go:embed schema)
  server_test.go      # test skeleton with schema/i18n coverage
  schema/
    deploy.json       # form/v1 deploy schema (edit to define your form)
    access.json       # form/v1 access schema (delete if reusing an existing access type)
    i18n/
      zh.json         # Chinese translations
      en.json         # English translations
```

After scaffolding, fill in the placeholder `access_provider_type` in `manifest.json`:
- To **reuse an existing access type** (recommended): set it to the existing type name (e.g., `"webhook"`, `"aliyun"`) and delete `schema/access.json`.
- To **define a new access type**: set it to your new type name and edit `schema/access.json` to define its credential form.

## 1. Implement the Deployer

Implement `pkg/plugin.Deployer` — three methods:

```go
type Deployer interface {
    GetMetadata(ctx context.Context) (*plugin.Metadata, error)
    GetConfigSchema(ctx context.Context) (*plugin.ConfigSchema, error)
    Deploy(ctx context.Context, req *plugin.DeployRequest) (*plugin.DeployResult, error)
}
```

- **GetMetadata** declares identity: `ProviderType` (deploy type, plugin-owned and unique), `AccessProviderType` (the credential type this deployer references), `ProtocolVersion` (must equal `plugin.ProtocolVersion`), the frontend `DeployCategory`, and the i18n key for the deploy display name.
- **GetConfigSchema** returns the `form/v1` deploy schema as JSON bytes (`DeploySchemaJSON`) and an i18n resource bundle (`I18n`). Use the shared helpers from `pkg/plugin` (`plugin.LoadDeploySchema(schemaFS)`, `plugin.LoadAccessSchema(schemaFS)`, `plugin.LoadI18n(schemaFS)`). The access schema (`AccessSchemaJSON`) is **optional** — see "Access types: new vs. reused" below.
- **Deploy** performs the actual deployment. `req` carries `AccessConfigJSON` / `ExtendedConfigJSON` (the two-segment opaque config), `CertificatePEM` / `PrivateKeyPEM`, and `LogLevel`. Deserialize the config segments with your own structs. Return a `DeployResult` with optional `ExtendedDataJSON`.

### Access types: new vs. reused

A deployer does **not** have to invent its own credential type. `AccessProviderType` may reference:

- **A new, plugin-owned access type** — return a non-empty `AccessSchemaJSON` from `loadAccessSchema()`. The core registers the schema and the type appears in the access picker. Use this only if no existing access type fits.
- **An existing access type** (built-in, or one another plugin already declared) — return `nil` for `AccessSchemaJSON`. The core reuses the existing access type's schema and picker entry; your `Deploy` must parse that type's config shape. This is how multiple deploy plugins share one credential (e.g. several Aliyun deployers all read an `aliyun` access), and how a deploy plugin shares a credential with `notify`/`apply` (e.g. the `webhook-deployer` pilot reuses the built-in `webhook` access that notifications also use).

When in doubt, prefer **reusing** an existing access type — one credential record then serves every consumer.

Errors you return are transported to the core as gRPC status messages and surfaced to the user — return plain `fmt.Errorf(...)`.

## 2. Define the form schema

Edit `schema/deploy.json` — a JSON object in the `form/v1` envelope shape:

```json
{
  "schemaVersion": "form/v1",
  "provider": "my-deployer",
  "category": "deploy",
  "schema": { "columns": [ { "name": "...", "valueType": "text", "labelKey": "...", ... } ] }
}
```

`valueType` is one of: `text`, `textarea`, `select`, `radio`, `switch`, `number`, `secret`, `code`, `autocomplete`. Fields flagged `"secret": true` are masked in the UI. Every `labelKey` / `placeholderKey` / `tooltipKey` must resolve via your i18n bundle (step 3) or the renderer falls back to showing the raw key.

The `category` field is included for spec conformance — the core overrides it at registration time.

If you define a new access type, edit `schema/access.json` in the same format.

## 3. Provide i18n

Edit `schema/i18n/zh.json` and `schema/i18n/en.json` — flat key-value JSON files. Use fully-qualified dotted keys namespaced under `plugin.<providerType>`:

```
plugin.my-deployer.name                       → display name
plugin.my-deployer.access.apiKey.label        → access field label
plugin.my-deployer.deploy.target.label         → deploy field label
```

To add a new locale, create a new JSON file in `schema/i18n/` (e.g., `ja.json`) — no Go code changes needed. The `plugin.LoadI18n` helper discovers locales at runtime by reading the directory.

The core injects these into the SPA's i18n store under the default namespace at the `plugin.<type>` path, so `t("plugin.<type>.foo")` resolves in pickers and dynamic forms. Missing locale/key falls back to the raw key (graceful degradation).

## 4. Write the manifest

`manifest.json` sits next to the binary. The scaffolding command fills in `provider_type`, `binary`, and `display_name_key` automatically. You must fill in `access_provider_type`:

```json
{
  "version": "0.1.0",
  "provider_type": "my-deployer",
  "access_provider_type": "webhook",
  "display_name_key": "plugin.my-deployer.name",
  "deploy_category": "other",
  "protocol_version": 1,
  "min_core_version": "",
  "max_core_version": "",
  "binary": "my-deployer",
  "icon": "",
  "sha256": "",
  "priority": 0,
  "usages": ["hosting"],
  "description": ""
}
```

- `protocol_version` must match the core's current protocol (hard gate; mismatched plugins are rejected with a readable error and never executed).
- `min_core_version` / `max_core_version` are optional soft gates.
- `binary` is the executable filename (same directory). The core refuses to load if the binary or directory is group/world-writable.
- `icon` optionally references an image file in the same directory; omit for the default placeholder.
- `sha256` is advisory (mismatch warns but does not block).
- `usages` declares the access type usage categories. Valid values: `dns`, `hosting`, `notification`, `ca`.

## 5. Serve the plugin

`main.go` calls `plugin.Serve` with the shared handshake config. The scaffolding generates this file — no changes needed:

```go
githubplugin.Serve(&githubplugin.ServeConfig{
    HandshakeConfig: plugin.HandshakeConfig,
    Plugins: map[string]githubplugin.Plugin{
        plugin.PluginName: &plugin.DeployerGRPCPlugin{Impl: &myDeployer{}},
    },
    GRPCServer: githubplugin.DefaultGRPCServer,
})
```

## 6. Build and install

```bash
make build PLUGIN=my-deployer  # produces dist/my-deployer
```

Place the binary **and** its `manifest.json` into the plugin directory under a per-provider subdirectory:

```
<pluginDir>/my-deployer/
  manifest.json
  my-deployer
  icon.svg          # optional
```

`<pluginDir>` is resolved by the core from the `CERTIMATE_PLUGIN_DIR` env var, or defaults to `./plugins` relative to the directory the core process is launched from (its working directory). Restart the core to load it (no hot reload).

## Verification

After restart: the provider appears in the deploy provider picker (tagged "Plugin"), its dynamic config form renders with translated labels, and a deploy routes through the plugin subprocess. See the core's `internal/pluginhost` E2E test for the full cross-layer path.

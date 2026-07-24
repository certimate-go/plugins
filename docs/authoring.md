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
- **GetConfigSchema** returns the `form/v1` deploy envelope as JSON bytes (`DeploySchemaJSON`) plus an i18n resource bundle (`I18n`). The access envelope (`AccessSchemaJSON`) is **optional** — see "Access types: new vs. reused" below.
- **Deploy** performs the actual deployment. `req` carries `AccessConfigJSON` / `ExtendedConfigJSON` (the two-segment opaque config), `CertificatePEM` / `PrivateKeyPEM`, and `LogLevel`. Deserialize the config segments with your own structs. Return a `DeployResult` with optional `ExtendedDataJSON`.

### Access types: new vs. reused

A deployer does **not** have to invent its own credential type. `AccessProviderType` may reference:

- **A new, plugin-owned access type** — return a non-empty `AccessSchemaJSON`. The core registers the schema and the type appears in the access picker. Use this only if no existing access type fits.
- **An existing access type** (built-in, or one another plugin already declared) — return an **empty** `AccessSchemaJSON` (`nil`). The core reuses the existing access type's schema and picker entry; your `Deploy` must parse that type's config shape. This is how multiple deploy plugins share one credential (e.g. several Aliyun deployers all read an `aliyun` access), and how a deploy plugin shares a credential with `notify`/`apply` (e.g. the `webhook-deployer` pilot reuses the built-in `webhook` access that notifications also use).

When in doubt, prefer **reusing** an existing access type — one credential record then serves every consumer.

Errors you return are transported to the core as gRPC status messages and surfaced to the user — return plain `fmt.Errorf(...)`.

## 2. Build the form/v1 schema

Each envelope is a JSON object of shape:

```json
{
  "schemaVersion": "form/v1",
  "provider": "<type>",
  "category": "access" | "deploy",
  "schema": { "columns": [ { "name": "...", "valueType": "text", "labelKey": "...", ... } ] }
}
```

`valueType` is one of: `text`, `textarea`, `select`, `radio`, `switch`, `number`, `secret`, `code`, `autocomplete`. Fields flagged `secret: true` are masked in the UI. Every `labelKey` / `placeholderKey` / `tooltipKey` must resolve via your i18n bundle (step 3) or the renderer falls back to showing the raw key.

## 3. Provide i18n

Return a `map[locale]map[key]string` covering at least `zh` and `en`. Use fully-qualified dotted keys namespaced under `plugin.<providerType>`:

```
plugin.webhook-deployer.name                       → display name
plugin.webhook-deployer.access.url.label           → access field label
plugin.webhook-deployer.deploy.method.label        → deploy field label
```

The core injects these into the SPA's i18n store under the default namespace at the `plugin.<type>` path, so `t("plugin.<type>.foo")` resolves in pickers and dynamic forms. Missing locale/key falls back to the raw key (graceful degradation).

## 4. Write the manifest

`manifest.json` sits next to the binary in the plugin directory:

```json
{
  "version": "0.1.0",
  "provider_type": "webhook-deployer",
  "access_provider_type": "webhook-deployer-access",
  "display_name_key": "plugin.webhook-deployer.name",
  "deploy_category": "other",
  "protocol_version": 1,
  "min_core_version": "",
  "max_core_version": "",
  "binary": "webhook-deployer",
  "icon": "",
  "sha256": ""
}
```

- `protocol_version` must match the core's current protocol (hard gate; mismatched plugins are rejected with a readable error and never executed).
- `min_core_version` / `max_core_version` are optional soft gates.
- `binary` is the executable filename (same directory). The core refuses to load if the binary or directory is group/world-writable.
- `icon` optionally references an image file in the same directory; omit for the default placeholder.
- `sha256` is advisory (mismatch warns but does not block).

## 5. Serve the plugin

`main.go` calls `plugin.Serve` with the shared handshake config:

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
make build          # produces dist/webhook-deployer
```

Place the binary **and** its `manifest.json` into the plugin directory under a per-provider subdirectory:

```
<pluginDir>/webhook-deployer/
  manifest.json
  webhook-deployer
  icon.svg          # optional
```

`<pluginDir>` is resolved by the core from the `CERTIMATE_PLUGIN_DIR` env var, or defaults to `./plugins` relative to the directory the core process is launched from (its working directory). Restart the core to load it (no hot reload).

## Verification

After restart: the provider appears in the deploy provider picker (tagged "Plugin"), its dynamic config form renders with translated labels, and a deploy routes through the plugin subprocess. See the core's `internal/pluginhost` E2E test for the full cross-layer path.

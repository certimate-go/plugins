# Plugin template

Scaffold for a new plugin: `make init PLUGIN=my-plugin` copies this directory and substitutes `__PLUGIN_NAME__` and `__ACCESS_TYPE__`.

The template ships as a `deploy` plugin. For another capability, change `capability` in `manifest.json` and `Capability` in `server.go` to one of `deploy`, `dns`, `notify`, or `purge` — every capability implements the same `Plugin` interface, and `Process` receives the capability's typed payload (`req.Deploy`, `req.Dns`, `req.Notify`, `req.Purge`).

Reference a plugin of the capability you need before extending the scaffold:

- `deploy`: `../qiniu-cdn/`
- `dns`: `../rfc2136-dns/`
- `notify`: `../mocknotify/`
- `purge`: `../ucloud-ussl-purger/` (smallest vendor surface) or `../baiducloud-cert-purger/`

Purge plugins list expired candidates, never expand the deletion scope beyond the configured threshold, and report the actual deleted count via `req.Purge` results; keep `PostProcess` empty with `plugin.NoopFinalizer`.

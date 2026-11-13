---
name: certimate-plugin-dev
description: Create or modify Certimate external plugins (deploy, dns, notify) in independent Go repositories or the Certimate plugin collection, including protocol integration, credentials, dynamic forms, Chinese/English translations, builds, and verification through the real host. Use for plugin development, not configuring existing plugins or modifying core built-in providers.
---

# Certimate Plugin Development

Deliver a plugin that the Certimate host can load, configure, and execute. For a new independent GitHub project, include runnable CI and a release workflow that automatically publishes new manifest versions on pushes to `main`. If the user explicitly asks for a local-only plugin, omit publishing automation. Establish the target platform, capability, credentials, and observable outcome. Ask only for essential information that cannot be inferred from the request or repository.

## Locate the repository and protocol contract

- Use the current project's repository and module. Read its applicable instructions, Go configuration, and existing plugin code. Preserve the repository and module identity.
- In the official plugin collection, use its Makefile, templates, and author guides when present. In an independent project, follow the bundled [third-party development guide](references/third-party-development.md) for setup, scaffolding, real-host verification, and GitHub Actions. Copy the bundled workflow templates into the target repository’s `.github/workflows/` and adapt them to its Go module and plugin layout. Reuse equivalent workflows when they already exist.
- Pin a core dependency compatible with the target host. Locate the actual protocol with `go list -f '{{.Dir}}' github.com/certimate-go/certimate/pkg/plugin`, then inspect its interfaces, handshake, schema loaders, and manifest validation. Match the target host's protocol and core version.
- Use examples to understand structure. Verify target platform authentication and operation semantics against its official documentation.

| Capability | Starting point | Integration points |
| --- | --- | --- |
| `deploy` | `_template` and the closest existing deploy plugin | `DeployerGRPCPlugin`, `Deploy`, `schema/deploy.json` |
| `dns` | `powerdns-dns` | `DnsGRPCPlugin`, `Present` / `CleanUp`, `schema/config.json` |
| `notify` | `slackbot` and the closest existing notify plugin | `NotifyGRPCPlugin`, `Notify`, `schema/config.json` |

## Create or modify a plugin

When modifying a plugin, preserve its provider identity and compatibility with saved configurations. For a new plugin, choose a unique kebab-case directory name and `provider_type`. Check for conflicts with existing plugins and the target host's built-in types.

In the official collection, scaffold a new deploy plugin with:

```sh
make init PLUGIN=my-deployer
```

`make init` currently generates only a deploy scaffold and uses BSD/macOS `sed -i ''`. On systems that do not support that syntax, copy `_template/` into a destination that does not yet exist and replace `__PLUGIN_NAME__` in its `.go` and `.json` files using available text tools. Do not change repository-wide build tooling just to scaffold a plugin. In either case, resolve `__ACCESS_TYPE__`, remove example fields and placeholder translations, and replace `TestDeploy_ReturnsNotImplemented` with verification of real behavior.

For DNS and notify plugins, create the minimum required files using the corresponding implementation as a reference instead of using the deploy scaffold directly. When copying reference files, update identity, schemas, translations, icons, and business logic. Remove the reference plugin's `release` metadata, checksums, and inapplicable fields.

Ordinary plugin development requires no core `init()` registration, static provider lists, or dedicated frontend forms. If the protocol lacks a required capability, describe the gap and address it within the user's authorized scope.

## Keep identity, credentials, and forms consistent

- Declare exactly one manifest `capability`: `deploy`, `dns`, or `notify`. Register the corresponding gRPC adapter in the entrypoint and use the shared handshake configuration.
- Keep provider and access identities consistent across the manifest, `GetMetadata`, and schemas. Match `protocol_version` to the current `plugin.ProtocolVersion`, `binary` to the build artifact, and any icon path to an actual packaged file.
- `usages` describes credential usage. Select `hosting`, `dns`, `notification`, or `ca` according to the actual consumers; it is not a declaration of multiple capabilities.
- Prefer reusing an access type with a compatible contract and parse its actual JSON fields. Do not declare a separate access schema when reusing a type. If another plugin owns that type, document the credential fields you share. Install the owner first and remove dependents before removing the owner. This applies across markets. For a new type, provide `schema/access.json` and its display name.
- Use the `form/v1` envelope and match field names to JSON tags in Go configuration structs. Put credentials in access configuration and target resources or operation options in provider configuration. Use the `secret` field type and masking flag for secrets.
- Use `LoadDeploySchema` for deploy plugins and, according to the current protocol, `LoadProviderSchema` for dns and notify plugins. Do not infer capability solely from the returned field name `DeploySchemaJSON`.
- Use flat translation keys under `plugin.<provider_type>.*` in `schema/i18n/zh.json` and `en.json`. Check all keys referenced by display names, labels, placeholders, tooltips, nested options, and conditional fields.
- Return diagnostic errors for malformed schema files. Allow an absent access schema only when explicitly reusing an access type.

## Implement platform behavior

Parse and validate configuration at the plugin boundary before calling platform APIs. Reuse suitable helpers in `internal/` and `pkg/` instead of copying large SDK sections. Do not directly modify trimmed or forked third-party SDKs.

- Propagate `context` into network calls and waits, and use bounded timeouts. Errors should identify the operation and target. Check both HTTP status and platform business errors.
- Do not expose credentials, private keys, or full requests or responses that may contain secrets in logs or errors. Deploy plugins should use the logger supplied by the interface. Do not write application logs to protocol stdout.
- For deploy plugins, verify that the certificate was uploaded and bound to the intended resource. Return `ExtendedDataJSON` when subsequent runs need it. Repeated execution should not unconditionally create duplicate resources.
- For dns plugins, cleanup must remove only the TXT value for the current challenge while preserving other values at the same name. Cover retries and recovery during cleanup.
- For notify plugins, handle destination overrides and platform failures. Verify the actual recipient and message content, and account for duplicate delivery when designing retries.
- Format changed files with `gofumpt`. Express intent through names and structure without adding explanatory code comments. Preserve compiler directives such as `go:embed`.

## Build and verify

In the official collection, run the relevant checks below. For an independent repository, use the build and package commands in the bundled guide.

```sh
go test ./my-deployer -count=1
go test . -run '^TestAllPluginManifestsSatisfyCoreContract$' -count=1
make build PLUGIN=my-deployer
```

`make build` requires `jq` and outputs `dist/<plugin>/`. Verify that the manifest, binary, and optional icon are present. The directory and binary must not be group- or world-writable. Place this directory under a separate plugin root for testing and point `CERTIMATE_PLUGIN_DIR` at that root.

Builds and unit tests do not replace end-to-end verification:

1. If the project has an `integration` directory, reuse its runnable fixtures and host startup. Otherwise use the bundled guide. When needed, consult tests from the matching core revision. Check operating system, container, environment variable, and core binary prerequisites. Direct calls to plugin methods or starting only a Manager do not constitute the complete user workflow.
2. Exercise the real entrypoint using a compatible core, a separate temporary database, and the built plugin. For deploy, run a workflow and verify the certificate at the target. For dns, run certificate issuance and verify TXT creation, use, and cleanup. For notify, use the notification entrypoint and verify the message received by the target. External services may use controlled fixtures, but the host-to-plugin subprocess path must actually execute.
3. Cover success, configuration errors, platform failures, and cancellation or recovery paths affected by the change. Check final state and user-visible errors. For new or changed forms, use a browser against the running backend to verify provider selection, dynamic fields, Chinese/English rendering, credential saving, and actual execution.
4. Run the selected integration test when available and inspect its logs. Some existing tests exercise the full workflow only when a core binary environment variable is supplied. A disabled workflow branch, no matching tests, skipped tests, or missing environment does not count as passing verification. Report specific blockers and leave end-to-end verification incomplete until resolved.

Add a small number of unit tests only for complex logic that is difficult to cover end to end. When shared helpers change, extend coverage to affected plugins. Repeated field assertions do not replace behavioral verification.

## Deliver

Report the plugin capability, credential choice, build artifacts, verification scenarios actually executed and their results, and any remaining verification gaps. Explain local installation paths and usage.

For an independent GitHub project, verify that the release workflow creates a GitHub Release, uploads the ZIPs, and only then publishes the catalog on `main`. An index alone cannot make the plugin installable. Check that the release workflow handles a push to `main`, including the first release. Report the trigger and how to retry manually. In the official collection, use its release guide and `make index`. Release tooling generates release metadata; do not invent release checksums. Follow repository rules for version changes. Committing, pushing, and publishing still require an explicit user request. A push to `main` with a new plugin version starts the configured release workflow.

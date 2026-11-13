# Build and distribute an independent plugin

Choose a provider identifier specific to your project, such as `example-widget-deploy`. Keep the identifier stable after release because workflow and credential records refer to it. A repository can hold one plugin or several plugin directories.

## Pin and inspect the protocol

In an empty repository, initialize your own module. In an existing project, use its module.

```sh
go mod init github.com/example/certimate-widget
core_version=v0.4.38-0.20261112233855-b0fca4697cbe
go get "github.com/certimate-go/certimate/pkg/plugin@$core_version"
go list -f '{{.Dir}}' github.com/certimate-go/certimate/pkg/plugin
```

The pin above is an example with deploy, DNS, and notify support. Choose the tag or commit corresponding to the actual host, and use the Go version required by that dependency. Run `go mod tidy` after the imports are in place. The protocol package builds from the published module. Go ignores dependency-module `replace` directives. If an imported SDK needs one, choose a published SDK or add a resolvable replacement in your own module. For local host development, put any checkout overrides in a temporary `go.work` outside the project and set `GOWORK` for that command.

## Scaffold a capability

Create a directory named for the provider. Give it `main.go`, `server.go`, `manifest.json`, and an embedded `schema/` directory. Inspect the selected adapter in the pinned protocol before writing the entrypoint.

| Capability | Adapter and plugin set key | Implement | Form loader |
| --- | --- | --- | --- |
| Deploy | `DeployerGRPCPlugin`, `PluginName` | `Deploy` | `LoadDeploySchema` |
| DNS | `DnsGRPCPlugin`, `DnsPluginName` | `Present`, `CleanUp` | `LoadProviderSchema` |
| Notify | `NotifyGRPCPlugin`, `NotifyPluginName` | `Notify` | `LoadProviderSchema` |

Use the shared handshake and `githubplugin.DefaultGRPCServer`. For example, the deploy entrypoint is:

```go
package main

import (
	"github.com/certimate-go/certimate/pkg/plugin"
	githubplugin "github.com/hashicorp/go-plugin"
)

func main() {
	githubplugin.Serve(&githubplugin.ServeConfig{
		HandshakeConfig: plugin.HandshakeConfig,
		Plugins: plugin.PluginSet{
			plugin.PluginName: &plugin.DeployerGRPCPlugin{Impl: &server{}},
		},
		GRPCServer: githubplugin.DefaultGRPCServer,
	})
}
```

Implement `GetMetadata` and `GetConfigSchema` alongside the capability methods. Embed the schema files using the Go embed directive. Use `schema/deploy.json` for deploy forms and `schema/config.json` for DNS and notify forms. Return `form/v1` and use matching display-name and field keys in `schema/i18n/en.json` and `zh.json`. Read the request and result types in the resolved package source, including the deploy logger. Read `ProtocolVersion` and manifest validation in the pinned package. Set the manifest's provider, access provider, version, capability, binary, and protocol version to match the entrypoint. Resolve every sample name and unused field before building.

For an upstream example, fetch an explicit tag or commit from the public `certimate-go/plugins` repository, inspect its core dependency, and verify it matches your target protocol. Its `_template`, `powerdns-dns`, and `ntfy` directories show deploy, DNS, and notify respectively. Copy only files you need and retain applicable licenses and notices.

When defining an access type, return its access schema once. When reusing a built-in or another plugin's type, omit the access schema and read the existing credential fields. For cross-market reuse, document the owner's provider identifier and field contract. Install the owner before consumers and uninstall consumers before the owner. Removing an owner's market leaves the installed owner available. The host does not install dependencies automatically.

## Verify in a real host

From the project root, build a standalone provider and stage it under a temporary plugin directory. This example targets the local Unix host. Use the manifest’s exact binary name when staging the executable. The GitHub release template uses one manifest across all targets and requires a binary name ending in `.exe` to support Windows. That name also works on Linux and macOS.

```sh
provider=example-widget-deploy
fixture_dir=$(mktemp -d)
mkdir -p "$fixture_dir/plugins/$provider"
go mod tidy
go test ./...
binary=$(jq -r .binary "$provider/manifest.json")
go build -o "$fixture_dir/plugins/$provider/$binary" "./$provider"
cp "$provider/manifest.json" "$fixture_dir/plugins/$provider/"
certimate migrate up --dir "$fixture_dir/data"
certimate superuser upsert fixture@example.test 'Local-fixture-password123' --dir "$fixture_dir/data"
CERTIMATE_PLUGIN_DIR="$fixture_dir/plugins" certimate serve --dir "$fixture_dir/data" --http 127.0.0.1:8090
```

The manifest `binary` must match the staged executable. Keep the binary and directory writable only by their owner. Use a compatible `certimate` binary from a release or your target host build. At the displayed local address, sign in, verify the plugin's credential and provider forms, save fixture credentials, and exercise its normal entrypoint.

- Deploy through a workflow using a test certificate. Check the target service for the uploaded certificate and binding. Repeat with a rejected request and verify the workflow reports the failure.
- Run issuance against a fixture ACME server such as Pebble, with a DNS service the fixture CA can query and that the plugin can mutate. Verify that issuance succeeds, the target TXT value appears and is removed, and unrelated TXT values survive. Use a test zone and test credentials.
- Send a notification test using a saved access record. Verify the target receives the expected message and credentials. Return an error from the target and check that the host reports the failure.

For a reused access type, repeat with the same credential record and the installed owner. Record the actual host version, command, and observed target behavior.

## Add GitHub Actions and release the plugin

For a new independent GitHub plugin repository, copy both bundled templates into the repository root's `.github/workflows/`: [ci.yml](../assets/github-actions/ci.yml) and [release.yml](../assets/github-actions/release.yml). Include them when scaffolding the plugin. Adapt them to the project's layout or extend equivalent existing workflows. Skip the release workflow only when the user explicitly requests a local-only plugin or the project targets another hosting service.

The templates expect `go.mod` at the repository root and each plugin in a top-level directory with a `manifest.json`. CI tests and builds on pull requests and pushes to `main`. Release runs on pushes to `main`. It discovers top-level plugin manifests, publishes the first version or a version bump, and skips versions already recorded for this repository. Its build uses the pinned public Certimate release tools. It builds ZIPs for Linux and macOS on amd64 and arm64, and Windows on amd64. If a supported target needs CGO or a different build matrix, adapt both the build and release metadata to the targets you actually package. The binary in each ZIP must have the exact name in the manifest. Use a manifest binary name ending in `.exe` for all archives with the default matrix. Put any declared icon at the root of the plugin directory.

Land the workflow and source on `main` for the first release. For later releases, bump the plugin’s manifest version before merging to `main`. To retry manually, open **Actions → Release plugin → Run workflow** on `main`. Leave the input blank to scan all plugins or enter one plugin directory. The workflow:

1. Runs tests and builds the platform ZIPs. Each ZIP contains the manifest, executable, and declared icon.
2. Computes ZIP hashes and generates the candidate market files locally on the runner.
3. Creates a GitHub Release tagged `<provider_type>/v<version>` in the current repository and uploads the ZIPs.
4. Updates `main/index.json`, `market/details/`, and the repository manifest after the release succeeds. Previous detail files stay addressable for cached indexes.

The repository must let the workflow token create releases and push the catalog commit to `main`. If GitHub creates the release but rejects the catalog push, the workflow uploads a `market-catalog-patch` artifact. Apply that patch through the repository's normal review process and keep the ZIPs on the original release. A version with an existing release should not be rebuilt and substituted under the same tag.

Verify both workflow files exist and exercise the automatic path locally with no plugin input. Confirm the initial manifest is selected, a published version is skipped, and a version bump is selected again. After an authorized release, check that the release assets are public and that the published catalog refers to their exact names and SHA-256 hashes. In Certimate, open **Plugins → Manage markets**, add the public GitHub repository URL, and install the plugin. The market reads the root `index.json` on `main`; copying an index does not copy its release assets. A compatible host without custom market support can still use the manual installation procedure above.

For a separate catalog repository, keep the same contract. Publish the ZIPs to their release repository first, then publish the generated index, content-addressed details, and icons on the catalog repository's `main` branch. The detail's release repository must identify where the ZIPs were actually uploaded. When using the pinned tools manually, `releaseinfo` expects all five platform ZIPs and accepts `-plugin`, `-repo`, `-dist`, and `-root`. A smaller build matrix can generate a release block using only its real ZIP names and checksums, then run `genindex` with `-root` and `-o`.

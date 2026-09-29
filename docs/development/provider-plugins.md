# External Provider Plugins

Mender supports installable Provider plugin types without recompiling the Mender binary. External plugins run as native subprocesses behind the versioned Provider Plugin Protocol and still enter the canonical Catalog and governed Invocation pipeline.

Architecture is defined by [ADR-0027](../architecture/adr/0027-external-provider-plugin-host.md) and the resilience/governance follow-up [ADR-0028](../architecture/adr/0028-provider-plugin-resilience.md).

## Built-in Provider plugin package

Mender's statically linked first-party plugins are composed through one explicit package:

~~~text
backend/internal/providerplugin/builtin
~~~

That package owns the built-in set and registration seam only. Today it constructs:

- `io.mender.provider.mcp`;
- `io.mender.provider.openapi`;
- `io.mender.provider.model-tools`.

The protocol implementations remain in their existing MCP, OpenAPI, and Model Tools infrastructure packages. The generic `backend/internal/providerplugin` core continues to own only contracts, Registry/Resolver behavior, instance lifecycle, configuration metadata, and external-plugin integration; it does not import first-party implementations.

Runtime composition therefore has one built-in registration dependency instead of one dependency per first-party plugin. Adding another statically linked first-party Provider requires adding it to the built-in package and its focused tests, while external Provider plugins continue to use the installation/package protocol described below.

Built-ins still have `sourceKind=builtin`, no Installation ID, and are enabled by construction. This package boundary does not convert them into filesystem packages or subprocesses and does not change PluginID, ConfigVersion, ProviderID, ToolID, or persisted Provider configuration.

## Trust model

A v1 external plugin is a native executable explicitly trusted by the local user or administrator. The subprocess boundary provides crash/dependency/ABI isolation, bounded RPC/logging, exact executable checksum verification, and authenticated local gRPC transport. It is not an OS sandbox; the executable can still use OS authority available to the user account.

Mender-owned capabilities remain narrow: a plugin does not receive a service locator, Credential inventory, or a general CredentialResolver. Only operation-required Credential values that already passed host governance cross RPC.

## Version and installation identity

These values are independent:

```text
Plugin ID          dev.example.github
Package version    1.4.2
Protocol version   1
Config version     3
Manifest version   1
Installation ID    pinst_...
```

External Providers pin an exact Installation ID. Installing a newer package does not silently retarget existing Providers, and side-by-side package versions may coexist.

## Package layout

```text
mender-provider.json
bin/
  mender-provider-example
```

The manifest declares Plugin ID, package version, supported protocol versions, display metadata, and platform entrypoints. Entrypoints must stay inside the package. Absolute paths, parent escapes, symlinks, unsupported platforms, oversized packages, and non-executable selected entrypoints fail closed.

Installation copies the package into Mender's managed plugin root, records a content digest, launches the package once to verify its protocol/runtime descriptor, and persists that bounded verified descriptor snapshot. Managed content is re-verified before later launches. Normal Provider-plugin catalog reads use the persisted descriptor snapshot and do not start verified plugin processes.

## Public Go SDK

The SDK lives at `sdk/providerplugin/go` and does not import `backend/internal/*`.

A plugin implements the public Plugin interface: Descriptor, ValidateConfig, InspectConfig, Open, ListTools, ExecuteTool, and Close. Start it with `providerplugin.Serve(plugin)`.

A standalone example lives at `examples/provider-plugin-example`. Build it independently:

```bash
cd examples/provider-plugin-example
mkdir -p bin
GOWORK=off go build -o bin/mender-provider-example ./cmd/mender-provider-example
```

The Runtime end-to-end test builds and installs this example, so it verifies the public package/SDK contract rather than a private test-only implementation.

Non-Go plugins may implement `contracts/providerplugin/v1/provider_plugin.proto` directly. Additional language SDKs are deferred.

## Configuration schema and UI

External plugins may publish JSON Schema Draft 2020-12 configuration metadata. Mender owns rendering; plugins cannot inject React, JavaScript, HTML, CSS, or arbitrary frontend bundles.

The generic v1 UI handles common object/string/number/integer/boolean/enum/array configuration, required/default/additionalProperties, basic bounds, URI format, and bounded nesting. Unsupported composition/dynamic features such as `$ref`, `oneOf`, `anyOf`, `allOf`, conditionals, pattern properties, and unevaluated constructs fail closed rather than being ignored.

The reviewed annotation `x-mender-control: credential` renders an inventory Credential selector. The host also enforces this annotation: configured values must be canonical `credential://<namespace>/<name>` references before plugin validation or persistence. Schema fields marked `writeOnly: true` or `format: password` receive the same host treatment.

Provider configuration persistence additionally rejects common secret-bearing field names when their values are literal secret material; Credential References and explicit environment references remain allowed. Management projections omit unsafe sensitive values from historical rows. Historical rows can therefore still be inspected and repaired, but they cannot be saved again or activated until the sensitive values are replaced by governed references.

Built-in MCP/OpenAPI/Model Tools retain first-party Mender-owned renderers. External installations are always identified by exact Installation ID and use the generic renderer.

## Credential boundary

`InspectConfig` reports Credential References plus a bounded, management-safe connection summary. Mender validates that external references occur as exact string values in persisted config, normalizes the references, and persists the normalized metadata with the exact Provider config revision. Credential inventory usage/deletion checks therefore remain available when a plugin is disabled, crashed, missing, or otherwise unavailable; they do not start plugin processes. Legacy rows without normalized metadata are inspected conservatively from their persisted JSON only.

Each Tool may narrow the Provider-level Credential set further. During invocation: Profile ToolBinding Credential scope -> scoped host resolver -> Tool-required refs -> ephemeral RPC values -> plugin execution. A reference outside Profile scope fails before secret bytes cross RPC. Resolved secret values are never persisted in the normalized metadata.

## CLI workflow

```bash
mender plugin install ./my-provider
mender plugin list --output json
mender plugin inspect pinst_...
mender plugin disable pinst_...
mender plugin enable pinst_...
mender plugin remove pinst_...
```

Removal is blocked while a Provider references the exact installation. Disabling persists fail-closed state, closes affected Provider sessions/routes, and terminates the package process.

Create a Provider from a pinned installation:

```bash
mender provider add external \
  --installation pinst_... \
  --namespace example \
  --name "Example" \
  --config-json '{"prefix":"example: "}'
```

Use `--config-file` for larger plugin-owned JSON.

Move an existing Provider to another exact installation of the same PluginID without changing ProviderID:

```bash
mender provider rebind example \
  --installation pinst_new... \
  --config-file ./provider-config.json
```

Rebind validates the target package/configuration and synchronizes its Tool snapshot before retiring the old pin. SourceKey-stable Tools retain their canonical ToolIDs. The failed invocation or failed rebind is never replayed automatically.

## Web and Desktop

Web and Desktop can list, enable/disable, remove installations, configure unknown external Provider types from persisted descriptor schema, and explicitly rebind an external Provider when another enabled installation of the same PluginID is available. Provider rows display the persisted connection summary when the plugin supplies one. External Provider details use the exact installation's persisted verified descriptor, so package version/display metadata remain available even while that installation is disabled or its process is unavailable.

Legacy installations without a verified descriptor may be backfilled during plugin catalog listing. Backfill persistence and process-cleanup failures are reported as installation-scoped catalog issues; one degraded installation does not hide unrelated healthy plugins. Once a verified descriptor is persisted, normal catalog reads remain process-free.

Browser management deliberately does not accept an arbitrary server filesystem path for installation. Desktop provides one scoped native directory picker and passes that local path only through the Wails host-local binding. Headless/local installation uses the CLI.

## Process lifecycle

The host uses one process per exact installed package per Runtime and may host multiple Provider sessions. It launches without a shell, minimizes inherited environment, negotiates protocol versions, enforces package/descriptor identity, applies host-owned upper bounds to startup, descriptor, ValidateConfig, InspectConfig, Open, ListTools, and Close operations, enforces 16 MiB gRPC message limits both directions, bounds stderr, and reaps processes on Runtime shutdown. ExecuteTool continues to use the governed Invocation deadline.

Plugin crashes fail the affected operation/package but do not terminate Mender. Mender does not replay the failed invocation. On a later normal call, the Provider Instance Manager detects that the stale external Instance is no longer reusable; the Supervisor then restarts the package process and the plugin opens a fresh Provider session. This recovery is request-driven—there is no background polling or restart loop.

## Upgrade behavior

Plugin upgrades are explicit: install the new package, verify its new Installation ID, rebind the existing Provider to that target installation/configuration, verify synchronization/execution, then remove the old installation after references are gone. Successful rebind preserves ProviderID, namespace, Profile/Policy references, and ToolIDs whose SourceKeys remain stable.

Mender never automatically follows a newer or “latest” installation. Package update discovery and automatic retargeting remain out of scope.

## Deferred work

OCI distribution, Sigstore/Cosign verification, registry policy/revocation, update discovery, enterprise allowlists, and a WASM Component/WIT runtime require separate phases.

## Quality gates

```bash
pnpm provider-plugin:proto:check
pnpm provider-plugin:sdk:lint
pnpm provider-plugin:sdk:test
pnpm provider-plugin:sdk:test:race
pnpm provider-plugin:example:lint
pnpm provider-plugin:example:test
pnpm provider-plugin:example:test:race
pnpm provider-plugin:example:build
pnpm check
pnpm security:check
```

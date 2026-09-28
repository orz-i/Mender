# External Provider Plugins

Mender supports installable Provider plugin types without recompiling the Mender binary. External plugins run as native subprocesses behind the versioned Provider Plugin Protocol and still enter the canonical Catalog and governed Invocation pipeline.

Architecture is defined by [ADR-0027](../architecture/adr/0027-external-provider-plugin-host.md).

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

Installation copies the package into Mender's managed plugin root, records a content digest, and re-verifies managed content before launch.

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

The reviewed annotation `x-mender-control: credential` renders an inventory Credential selector. Persisted config stores the Credential Reference, not the secret.

Built-in MCP/OpenAPI/Model Tools retain first-party Mender-owned renderers. External installations are always identified by exact Installation ID and use the generic renderer.

## Credential boundary

`InspectConfig` reports Credential References. Mender validates that external references occur as exact string values in persisted config. Each Tool may narrow that set further.

During invocation: Profile ToolBinding Credential scope -> scoped host resolver -> Tool-required refs -> ephemeral RPC values -> plugin execution. A reference outside Profile scope fails before secret bytes cross RPC.

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

## Web and Desktop

Web and Desktop can list, enable/disable, remove installations, and configure unknown external Provider types from descriptor schema.

Browser management deliberately does not accept an arbitrary server filesystem path for installation. Desktop provides one scoped native directory picker and passes that local path only through the Wails host-local binding. Headless/local installation uses the CLI.

## Process lifecycle

The host uses one process per exact installed package per Runtime and may host multiple Provider sessions. It launches without a shell, minimizes inherited environment, negotiates protocol versions, enforces package/descriptor identity, applies startup/RPC deadlines, enforces 16 MiB gRPC message limits both directions, bounds stderr, and reaps processes on Runtime shutdown.

Plugin crashes fail the affected operation/package but do not terminate Mender.

## Upgrade behavior

Plugin upgrades are explicit: install the new package, verify its new Installation ID, explicitly move/recreate Provider configuration when supported, verify synchronization/execution, then remove the old installation after references are gone.

Phase 16 does not include automatic update/retargeting.

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

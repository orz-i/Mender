# ADR-0037: Make Mender plugins a platform with typed Provider, Sandbox, and Tunnel extensions

- Status: Proposed
- Date: 2026-10-10
- Scope: architecture research and decision proposal; this document does not authorize implementation

## Context

Mender is a governed Tool and Resource runtime for LLM/Agent clients, not an Agent loop. The current installable plugin infrastructure is narrowly Provider-specific:

- `backend/internal/providerplugin/{plugin,registry,manager}.go` owns Provider descriptors, configuration validation, resolution and live sessions.
- `backend/internal/providerplugin/{manifest,installation,external}/` owns `mender-provider.json`, installation identity, package checksums, verified descriptor snapshots and process supervision.
- `contracts/providerplugin/v1/provider_plugin.proto` and `sdk/providerplugin/go/` expose Provider discovery, Tool execution and Provider session operations.
- `backend/internal/sandbox/application/ports.go` already defines an independent Sandbox Backend / Session port, but `sandbox/domain.BackendKind` is a closed vendor enum. Docker/Daytona provisioning, immutable templates and lifecycle are still wired in `bootstrap` separately from the execution router.
- `backend/internal/tunnel/application/ports.go` already defines a separate process boundary, but the domain only accepts `openai`, and create/start are OpenAI-specific.
- `packages/app/src/domains/plugins/marketplace.ts` has a multi-kind UI projection/adapter interface, but registers only `providerMarketplaceAdapter`. HTTP, Wails and SDK plugin installation actions are Provider-specific.

Current architectural decisions explicitly prohibit representing Sandbox or Tunnel as a Provider: ADR-0035 separates tool production from execution placement, and ADR-0031 separates downstream reachability from upstream Tool Providers. ADR-0032/0036 require real Principal/Policy/Sandbox admission, and ADR-0034 centralizes runtime-owned process governance. The solution must retain these invariants.

## Proposed decision

Establish a **Mender Plugin Platform** with one host-owned installation and process infrastructure, plus *typed* extension points:

| Extension kind | Responsibility | Owned bounded context |
| --- | --- | --- |
| `provider.tools` | Discover and execute canonical Tools through the existing Invocation pipeline | Provider / Catalog / Invocation |
| `sandbox.backend` | Manage and operate a pre-authorized Sandbox execution environment | Sandbox |
| `tunnel.transport` | Connect a managed, loopback-only downstream MCP Profile endpoint to a named reachability service | Tunnel / managed MCP Server |

A **Plugin Package** is executable/distributable software. An **Extension** is a versioned, typed contract supplied by that package. A **Resource** is a configured Provider, Sandbox connection/Sandbox, or Tunnel owned by a Mender bounded context. Package identity must not be confused with configured resource identity.

The host owns governance, state transition decisions, identity, normalized Credential References, resource binding, admission, leases, quotas, audit, and resource lifecycle orchestration. A plugin supplies implementation-specific behavior and observations only. No plugin receives authority to author Mender policies, mint trusted Principals, grant its own Sandbox leases or bypass managed MCP security boundaries.

## Package identity and metadata

Retain stable PluginIDs, InstallationIDs and immutable exact-version pinning. A generic installation record owns:

```text
InstallationID
PluginID
PackageVersion
SourceKind / SourceReference
ContentDigest
NormalizedManifest + VerifiedExtensionDescriptors
AdministrativeStatus
InstalledAt
```

Descriptors distinguish the extension contract from the package:

```text
ExtensionID (stable within PluginID)
Kind (provider.tools | sandbox.backend | tunnel.transport)
ExtensionProtocolVersion
ConfigVersion
ConfigSchema / reviewed UI metadata
Capabilities / host-validated security assertions
```

A configured resource persists an exact binding:

```text
ResourceID
PluginID
ExtensionID
InstallationID?   // absent for built-ins
ConfigVersion + immutable config revision
ConfigJSON + host-owned normalized Credential References
```

No resource may resolve an arbitrary 'latest' package. Installing a new version must never silently redirect an existing Provider, Sandbox, or Tunnel.

New external packages use `mender-plugin.json` (proposed manifest version 2). A possible *illustrative* format:

```json
{
  "manifestVersion": 2,
  "pluginId": "dev.example.sandbox",
  "version": "1.0.0",
  "displayName": "Example Sandbox",
  "entrypoints": {
    "darwin/arm64": "bin/mender-example-sandbox"
  },
  "extensions": [{
    "id": "main",
    "kind": "sandbox.backend",
    "protocolVersion": 1
  }]
}
```

Manifest and descriptor validation remain bounded, side-effect-free and host-owned. A plugin cannot inject executable management UI. The schema allows an `extensions` array for future composition; the first external implementation supports **exactly one typed extension per native package**, rejecting multi-extension packages until dispatch/process-lifecycle semantics are specified and tested. Do not build a generic cross-domain service locator prematurely.

The existing `provider.tools` wire contract remains a first-class, versioned Provider extension protocol. The plugin platform does not require a single oversized RPC service: the common runtime owns package verification/launch/health, and typed adapters own distinct gRPC contracts. Avoid new compatibility bridges around Provider and avoid changing canonical Tool/Provider IDs.

## Sandbox extension contract

The Sandbox bounded context remains the authority for `SandboxID`, Owner/Scope, immutable security configuration, `Run.Generation`, state machine, durable execution leases, Profile delegation grants, admissions, and audit.

External Sandbox Backend extension operations should be expressed as small ports (transport methods may differ):

- Static descriptor: supported file/read/search/edit/process operations, lifecycle, snapshots/streams, isolation guarantees and platform requirements. Merely advertising a capability is **not** proof of enforcement.
- Connection configuration: validate and inspect non-secret JSON, report exact Credential References and safe metadata; optional availability probe only after authorization.
- Lifecycle: provision, inspect, start, stop, destroy, reconcile with idempotency operation key and Run generation fencing; model uncertain remote results explicitly.
- Session: acquire/open and close a backend session, with file and process APIs addressing the **same** Sandbox filesystem. No host-side file/shell fallback.

Do not delegate `sandbox.provision`, `sandbox.acquire`, `sandbox.start`, `sandbox.stop`, `sandbox.destroy` Policy decisions to the plugin. Host must authorize before resolving secrets, opening connections or performing chargeable lifecycle operations. Plugins return observations; Mender persists transitions atomically. Cancellation/timeout does not mean a remote destroy/provision failed or succeeded: surface `unknown`/`orphaned` and reconcile without automatic replay of side effects.

Replace hardcoded BackendKind routing with a typed backend binding, preserving legacy metadata and isolation labeling via a one-time migration. Local Workspace and Docker Engine remain **trusted built-ins** in the first stage because local host/process and Docker socket operations are high-authority. Daytona remains experimental until its real remote acceptance succeeds. E2B remains unsupported until an implementation passes full capabilities/lifecycle checks. A resource does not populate the Catalog by itself: the existing Workspace Provider remains the Tool facade with stable `workspace.*` SourceKeys.

## Tunnel extension contract

Tunnel is downstream transport infrastructure; it does not produce Tools. Retain `TunnelID`, Profile's optional TunnelID binding, Credential inventory references, and the coupled managed HTTP MCP Server lifetime.

Typed Tunnel adapters should support:

- Configuration validation/inspection and safe Credential Reference discovery.
- Start/connect to the **host-selected** managed loopback MCP endpoint, returning a bounded session handle and evidence-based health (starting/ready/degraded/failed).
- Stop/close, exit/crash observation, bounded retry/backoff policy where safe, and managed Server startup rollback.
- Provider-specific reachability claims, identity and authentication prerequisites. OpenAI Secure MCP Tunnel is not a generic public URL; other providers must not be advertised as equivalent without proof.

The host controls endpoint selection, process supervision and credential delivery after authorization. An external Tunnel plugin cannot unilaterally widen the MCP bind address, redefine trusted Profile scope or expose an arbitrary public listener. OpenAI becomes `io.mender.tunnel.openai` as a built-in extension; additional tunnel kinds require their own tested adapters.

## Shared runtime, security and lifecycle

The common plugin platform owns: Registry/Resolver, local package install, digest and verified descriptor snapshot, explicit enable/disable/remove, process supervisor, exact installation pin, config schema validation, platform-owned Credential Reference extraction, safe diagnostics and typed management projections.

External native binaries remain *trusted code running with the OS authority of their process*. go-plugin/gRPC, magic cookies, process separation and signed artifacts do not make them a filesystem/network/container security sandbox. Any requested permissions/capabilities in metadata are disclosures and policy inputs, not OS-enforced grants unless a real isolation mechanism proves otherwise. Docker daemon/socket access and local host filesystem/process access must not be handed to untrusted code through convenience APIs. Keep high-authority local backends built in until an enforceable host-mediated or OS sandbox design exists.

Disable/remove must consult **all resource domains** before tearing down an installation. Providers may remain configured but become unavailable as currently defined. A Tunnel with an active managed server must be drained before disable. A Sandbox driver with nonterminal resources, running leases, uncertain runs or outstanding cleanup obligations must **not** be disabled/removed in a way that strands remote resources. Fail closed with actionable usage diagnostics; do not automatically destroy remote resources, retry side-effecting calls or fall back to other drivers.

Plugin catalogue and management reads must not start processes, make remote calls, provision resources or resolve credentials. Use verified snapshots and explicitly separate static capability from observed health. Existing owner-only CLI/Desktop/loopback management protections remain; do not make install or resource-admin controls available as MCP Tools.

## Migration and compatibility policy

Implement a **forward-only hard cut**, consistent with ADR-0014, without permanent dual-read tables or generic compatibility shim packages.

1. Migrate existing external Provider installations into the generic installation inventory **preserving InstallationID, PluginID, package version, exact package bytes and digest**. Convert persisted metadata to a generic typed descriptor in one migration, without silently rewriting installed executable bytes. Direct Provider Plugin Protocol v1 remains the current Provider typed extension protocol; retaining this published service is not a legacy adapter.
2. Existing configured Providers retain ProviderID, ToolID/SourceKey, Profile bindings, policy references, Credential references and exact external installation pins. Built-in Provider IDs remain unchanged.
3. Rebind Sandbox backend kinds (`local_workspace`, `docker`, `daytona`) to stable typed extension identities in one atomic/idempotent migration. Preserve SandboxID, immutable template settings, run generation/external ID/state, durable leases, existing Profile capability grants and lifecycle audit. Never provision a resource while migrating.
4. Rebind existing `openai` Tunnel rows to the built-in OpenAI extension; preserve TunnelID, external ID, Credential Reference, Profile tunnel binding and managed server rollback semantics.
5. After migration, the runtime reads the **new authoritative model only**. Fresh installations require the generic manifest. Unsupported/corrupt references and missing/incompatible extensions remain visible but fail closed; startup must not delete, recreate or silently downgrade a live resource. Backups, rollback-on-error and downgrade refusal are mandatory.

The migration implementation must specify how a previously installed immutable Provider package is resolved from its migrated verified entrypoint metadata and checked against its *unchanged* package digest without consulting legacy manifest parsers on every Runtime start.

## Management and product boundaries

The existing Plugin Marketplace becomes the common catalogue of built-in and installed typed extensions. Keep card-grid discovery, same-row type/search/filter/actions, cursor pagination, dedicated details, and technical IDs only within details. The backend provides a unified catalogue; Web and Desktop consume it through typed SDK/Wails parity. A plugin detail's **Add** action opens the owning resource workflow (`Providers`, `Sandboxes`, `Tunnels`) with that extension pinned and only the selected extension's form visible. Lists are previews; edit/create/delete happen through focused follow-up surfaces. Never inject plugin React/HTML/JS into management.

## Non-goals

No Mender Agent loop or hosted sandbox orchestrator. No arbitrary runtime-generated React/HTML. No automatic plugin marketplace downloads, OCI distribution or dynamic frontend modules in the first cut. No promise of native plugin confinement. No speculative proliferation of extension kinds. No cross-domain generic resource lifecycle abstraction erasing different ownership and failure semantics.

## Alternatives considered

- **Add Sandbox and Tunnel to `providerplugin.Plugin`**: rejected; incorrectly mixes upstream tools, execution infrastructure and downstream reachability, and makes a large interface.
- **Keep three completely separate install/process managers**: rejected; duplicates packaging, trust decisions, lifecycle and Marketplace projections.
- **Replace existing Provider IPC with a universal gRPC RPC immediately**: rejected; breaks working public SDK/binaries and erases typed contracts without a benefit.
- **Enable arbitrary multi-extension native packages in the first cut**: deferred; shared process/version/partial-disable semantics need demonstrated use cases.
- **Use WASM Component/WIT for every extension now**: deferred; WASI 0.3 supports async but native host integration, secrets and lifecycle still need explicit capability design.

## Acceptance before implementation

This ADR must be accepted explicitly before Phase 27 implementation. Verify install identity/byte integrity, no startup process side effects, per-kind contract gating, mandatory host admissions before credentials/charges, Sandbox fencing/reconcile/no fallback, tunnel bind/start/stop/rollback, native trust disclosures, forward-only upgrades preserving all existing resource IDs and references, full CLI/HTTP/SDK/Wails parity, focused bilingual UI, race tests and the repository quality gates. Never modify real user Sandboxes, Tunnel credentials or live remote resources during acceptance tests.

## References

- [ADR-0027: external Provider plugins](0027-external-provider-plugin-host.md)
- [ADR-0031: managed Profile tunnels](0031-managed-profile-tunnels.md)
- [ADR-0034: runtime-owned processes](0034-runtime-owned-process-governance.md)
- [ADR-0035: Sandbox execution router](0035-governed-sandbox-execution-router.md)
- [ADR-0036: Profile Sandbox delegation](0036-sandbox-profile-delegation.md)
- https://developer.hashicorp.com/terraform/plugin/terraform-plugin-protocol
- https://github.com/hashicorp/go-plugin
- https://backstage.io/docs/backend-system/architecture/extension-points/
- https://code.visualstudio.com/api/references/extension-manifest
- https://opentelemetry.io/docs/collector/components/
- https://github.com/containernetworking/cni/blob/main/SPEC.md
- https://docs.docker.com/engine/security/protect-access/
- https://wasi.dev/docs/roadmap

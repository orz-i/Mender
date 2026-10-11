# Phase 27 proposal — Multi-kind Mender Plugin Platform

Status: In progress; ADR-0037 accepted. Work packages are separately implemented, verified and committed.

Governing proposal: [ADR-0037](../architecture/adr/0037-multi-kind-plugin-platform.md). This phase generalizes the existing Provider-only plugin installation/host to support Provider Tool Sources, Sandbox Backends and Tunnel Transports. Mender stays a governed Tool/Resource runtime, not an Agent loop. The deliverable is real backend extension support, not merely three labels in the Marketplace.

## Audited baseline (2026-10-10, HEAD f8b35cb)

| Current implementation | Evidence | Missing for multi-kind platform |
| --- | --- | --- |
| Provider extension contract, registry, instance manager | `backend/internal/providerplugin/{plugin,registry,manager}.go` | Types import Provider/Catalog/Invocation, not a platform-wide kernel |
| External Provider host | `backend/internal/providerplugin/{manifest,installation,external}/`; `mender-provider.json` | Installation, manifest, package trust and process supervision bound to Providers |
| Public Provider SDK | `contracts/providerplugin/v1/provider_plugin.proto`; `sdk/providerplugin/go/` | RPCs only cover Tool discovery/execution, not Sandbox/Tunnel contracts |
| First-party provider package | `backend/internal/providerplugin/builtin/builtin.go` | Only MCP, OpenAPI, Model Tools and Workspace registered |
| Governed Sandbox Router | `backend/internal/sandbox/application/ports.go`; `sandbox/infrastructure/router/router.go` | Closed `BackendKind`; Docker/Daytona create/lifecycle and immutable templates are special-cased in Bootstrap |
| Current Sandbox backends | `sandbox/infrastructure/{localworkspace,dockerengine,daytona}/` | Docker is opt-in; Daytona experimental/disabled by default; no live-accepted E2B adapter |
| Tunnel bounded context | `backend/internal/tunnel/{domain,application,infrastructure/process}/` | Domain only supports OpenAI; process args and Create usecase are vendor-specific |
| Marketplace | `packages/app/src/domains/plugins/marketplace.ts` | Generic adapter shape, only `providerMarketplaceAdapter` is installed |
| Management transports | `contracts/api/openapi.yaml`; `packages/sdk/`; `backend/internal/desktop/binding/`; `backend/internal/cli/plugin.go` | Installation endpoints/actions and generated types are Provider-specific |

Existing plugin trust hardening and exact pinned installations are valuable assets. Existing Sandbox Profile grants and leases (Phase 26) are **not** rewritten or simplified by pluginization.

## Required behavior and boundaries

```text
                  Mender Core
      Plugin Inventory / Package Trust / Process Host
               /             |             \
     provider.tools    sandbox.backend    tunnel.transport
           |                 |                 |
    Catalog + Invoke   Sandbox Router      Managed MCP Server
    Policy + Audit     Admission+Lease     Profile Tunnel binding
           |                 |                 |
       Tool Source      Sandbox Resource     Tunnel Resource
```

- Package/Installation and Extension identities are different from resource identities. A package may be installed without creating a resource; a built-in needs no synthetic Installation row.
- Provider Tool IDs, `io.mender.provider.*` PluginIDs and Workspace `workspace.*` SourceKeys stay stable.
- Sandbox is not a Tool Provider. Profile or Tool arguments never choose the Sandbox backend or directly resolve credentials.
- Tunnel is downstream reachability, not a Tool Provider; managed Profile MCP stays loopback-only.
- Native third-party plugins remain fully trusted OS processes absent real OS isolation; manifest capability declarations are not a security boundary.
- All resource-specific authority remains in the owning bounded context, and all side-effecting operations observe its Policy/Approval/lease/budget gates.
- The generic catalogue is process-free/read-only; management lists never launch plugins or provision remote resources.
- Never auto-fallback to host on unsupported Sandbox operations; never auto-replay uncertain provisioning, shell/edit or remote destroy.
- Do not add compatibility bridges or permanent dual-read database paths. Keep the published Provider gRPC v1 contract as a typed, supported protocol; migrate persisted packages and bindings once.
- Do not edit `.agents/`, `.pi/`, `.intent-loop/` or any skill-related source.

## Work packages and separate commits

### A. Decision and migration design (complete)

Deliverables: Proposed ADR, source-backed inventory, explicit plugin/extension/resource concepts, manifest v2 shape, trust boundaries, forward-only migration strategy, staged acceptance. No production code or live resource changes.

Gate: architecture review agrees with preserving existing Provider protocol as its typed extension service and rejecting generic Sandbox/Tunnel-as-Provider.

### B. Generic package and extension infrastructure

- Extract `backend/internal/plugin` as an installable **package** bounded context: installation identity, repository/store, manifest validation, exact package pin, verified per-extension descriptors, package integrity, health and supervised native process boundaries.
- Keep `backend/internal/providerplugin` for Provider behavior and instances. Its Resolver uses the new platform installation store; do not insert a second Provider-specific package installation registry.
- Introduce three explicit typed Extension kinds and a registry/factory integration boundary. Allow exactly one extension per external package in the first version, while manifest represents an array for future safe growth.
- Introduce a generic `mender-plugin.json` for new packages. Migrate existing Provider installations, persisted descriptor snapshots and package references without modifying existing installed bytes/digests or reassigning InstallationIDs. Runtime does not retain a legacy manifest parser after the migration.
- Preserve per-Provider exact installation pinning, fail-closed status and process-crash semantics; reuse the current Provider Plugin Protocol v1 directly as the Provider extension.
- Add generic plugin CLI/management API/Wails SDK projections in the same staged commit family, without breaking typed Provider resource management.

Gate: existing external example Provider installs/loads/executes/rebinds/disables under the generic package store; backup/current-schema/future-schema migration tests; digest tamper, invalid manifest, wrong extension kind, missing plugin and package crash fail closed. Provider identity/ToolIDs and Credentials remain unchanged.

### C. Sandbox Backend extension

- Add typed `sandbox.backend` registry, descriptor, config validation and versioned external protocol/SDK fixture. First-party Local Workspace/Docker/experimental Daytona register as typed built-ins, preserving their actual capability and isolation behavior.
- Replace vendor-closed `BackendKind` lookup with immutable typed binding to PluginID/ExtensionID/InstallationID. Keep trusted host-facing Docker Engine endpoint and local host launcher behind explicit built-in adapters in V1.
- Move backend-specific create/provision/start/stop/destroy/reconcile dispatch out of Bootstrap switch branches into capability-checked backend controller adapters. Mender continues to own lifecycle state, immutable templates/connection revision, lease fencing, Profile grants and Policy/Approval.
- Return truthful per-driver capabilities. External fixture must demonstrate connection validation, provision/inspect/open/execute/destroy and uncertain-operation reconciliation, with no local-host fallback.
- Migrate existing Sandbox IDs, runs, Docker/Daytona templates, generation/external IDs and grants without recreation; maintain opt-in Daytona quarantine until remote live validation.

Gate: fake driver end-to-end, cross-Runtime lease contention, grant/revoke fencing, failure during provision/destroy, remote orphan reconciliation, disabled driver cleanup blocking, restart and no-side-effect catalogue reads. Existing local/Docker/Daytona focused tests and Sandbox integration remain green.

### D. Tunnel Transport extension

- Add typed `tunnel.transport` registry/protocol/SDK fixture. Register OpenAI as a built-in transport with stable extension identity; move OpenAI-specific creation validation and execution details out of the generic Tunnel usecase.
- Preserve `TunnelID`, Profile reference, Credential Reference and managed MCP startup/rollback/stop ownership. Tunnel plugin only receives the host-selected loopback MCP endpoint and allowed ephemeral credentials after admission.
- Surface meaningful process exit/readiness/health evidence separately from a mere child-started state. Different transport types may advertise different reachability semantics; no generic promise of public access.
- Migrate existing OpenAI Tunnel inventory and Profile bindings without any network call or tunnel startup.

Gate: fake external Tunnel starts against actual loopback test MCP, returns health transitions, crashes, restores managed server cleanup, and cannot alter public listen binding or expose Credential values; referenced Tunnel deletion and Credential usage checks remain enforced.

### E. Unified experience and final acceptance

- Backend serves one Plugin Catalogue with typed extensions/installations and resource usage/error projection. Web, Desktop, CLI and TypeScript SDK use one canonical management schema rather than separate frontend-only Provider installation aggregation.
- Keep existing Marketplace responsive cards, cursor pagination, search/type/status/source controls, detail page, safe technical metadata and package-level enable/disable/remove; make local package install installer generic.
- From plugin detail, `Add provider`, `Add sandbox`, `Add tunnel` enter a focused owner-domain flow with the selected extension fixed. No unrelated plugin forms, dense forms in list views, or arbitrary plugin-injected UI.
- Config UI reads bounded host-validated JSON Schema and uses host-owned Credential selectors. No raw filesystem path input from Web.
- Destructive actions preview active usage/leases/billable/unknown resources before confirmation; block disabling/removing drivers that would strand cleanup.
- English/Chinese parity, no redundant page descriptions, compact title/toolbar, one information class per view.

Gate: HTTP/OpenAPI/TS/Wails consistency, end-to-end Marketplace -> detail -> resource create, exact package pin, fake native Provider/Sandbox/Tunnel, migration on a copy of existing DB, lifecycle and trust negative cases, browser Desktop/mobile QA, `pnpm check`, `pnpm security:check`, focused `go test -race` and desktop production build. No real Sandbox, Tunnel or API key touched by tests.

## Migration and safety checklist

| Historical object | Must preserve |
| --- | --- |
| Provider | ProviderID, PluginID, SourceKey-stable ToolID, exact InstallationID pin, configuration and policy/profile links |
| External Provider package | InstallationID, package version, unchanged package bytes and SHA-256 digest, trust and descriptor verification |
| Sandbox | SandboxID, owner, declared isolation and immutable spec, templates, run generation/state/external ID, durable leases, delegation grants and activity history |
| Tunnel | TunnelID, external tunnel ID, normalized Credential Reference, Profile tunnel link, managed server ownership |
| Credentials | Inventory/secret-backend split; no secrets copied into plugin manifest, config or diagnostics |
| Audit | Stable referents, immutable historical records, explicit migration evidence |

DB startup migrates atomically/idempotently with a pre-upgrade backup path and rejects unknown future schemas. If a package, backend or resource binding is missing or inconsistent, report an actionable unavailable state rather than silently ignoring the reference. Migration itself performs no Docker/Daytona/OpenAI I/O.

### B-next: one native process supervisor, typed protocol dialers

Package process ownership must move to `backend/internal/plugin/process/`, separate from Provider RPC semantics. The generic supervisor owns per-InstallationID synchronization, exact pinned package resolution, descriptor kind/protocol guards, at-most-one live process per exact installation, crash recovery, disable/close/shutdown, and process-free health queries. A typed dialer is selected by `extension.kind` only **after** host validates the persisted installation, exact package digest and normalized manifest/extension identity. The dialer negotiates its own typed wire protocol and verifies the runtime descriptor before a process is cached.

- Native Provider Plugin Protocol v1 remains in `providerplugin/external` as the `provider.tools` dialer; it does not become a universal RPC service.
- Shared process host must not import Provider/Catalog, Sandbox, Tunnel or generated gRPC interfaces.
- Execution remains request-driven; registry/catalogue reads never launch the plugin.
- A returned process is not reusable when package identity/digest/extension changes, and unsupported kinds fail before launching anything.
- `CloseInstallation` and `CloseAll` must preserve current bounded cleanup and race semantics. Quarantined side-effecting Sandbox drivers will later block disable/remove until cleanup obligations are reconciled.
- The Provider adapter may expose typed `Ensure`/timeout handling around the host. Such an adapter is an extension implementation, **not** a second independently owned process registry or a compatibility bridge.

Acceptance for this cluster: concurrent Ensure, cross-installation concurrency, nil/missing/wrong-kind dialer, exact digest/version mismatch, process crash/restart, disabled/closed/no-process-start catalogue, close/reuse races; existing Provider external integration and full repository gates.

### C1: trusted Sandbox Backend extension registry and Router cutover

Before introducing external Sandbox RPC or a new persisted backend binding, the existing local/managed backend implementations must become **registered typed** `sandbox.backend` extensions. The initial cut is deliberately scoped to runtime routing and trust metadata:

- Every registered backend is paired with one immutable, validated `plugin.Descriptor` of kind `sandbox.backend`, exact ExtensionID `main`, a stable namespaced PluginID, and an exact sandbox backend kind. Identity collisions and wrong-kind registration fail closed.
- The typed descriptor capabilities are derived from the **actual** Backend operations: `file.read`, `file.search`, `file.edit`, `process.shell`; an external/vendor claim alone cannot elevate permissions.
- The Sandbox Execution Router receives one typed `BackendResolver`, not an unreviewed varargs list or a parallel kind-to-backend map. `Resolve` remains read-only; `Acquire` continues owner/Principal/Policy/Approval, state-generation and durable lease admission **before** resolving/opening an extension backend. Unknown/offline backends remain unavailable and never fall back to local host.
- Local Workspace/Docker/experimental Daytona retain existing immutable templates and lifecycle controllers during C1. Docker is opt-in and Daytona remains quarantined when the operator has not enabled experimental execution. Offline drivers do not become runnable merely because their descriptor is discoverable.
- Persisted `BackendKind` remains unchanged **during C1** to preserve resources; the C2 forward-only migration to explicit `PluginID + ExtensionID + InstallationID?` and C3 external protocol cannot be called complete yet. No compatibility shim is introduced: C1 replaces the Router dispatch directly.

Acceptance: typed registration invalid/duplicate cases, real Backend/descriptor parity, denied Acquire no side effects, unavailable backend and unsupported capability fail-closed, unchanged lease/revoke and concurrent acquisition tests, no Docker/Daytona live operations and repository gates.

## Verified implementation progress

- **A (complete):** ADR-0037 accepted before implementation; no user resource changes.
- **B — foundation (complete):** shared PluginID/InstallationID and typed registry;
  strict `mender-plugin.json` v2 manifest; generic `plugin/installation`
  domain/application/SQLite/local package inventory; Provider extension descriptor
  adaptation; one-time schema migration with WAL-consistent backup and original
  v1 package digest integrity; Provider RPC v1 stays unchanged. Provider-only
  admin operations reject wrong extension kinds. Previously installed native
  Provider binaries remain byte-for-byte pinned.
- **B — native host complete:** the Provider extension now delegates package
  verification, per-installation concurrency, crash/restart and teardown to
  one generic typed process supervisor. Provider Protocol v1 remains its typed
  dialer. `pnpm check` and `pnpm security:check` pass; verified by
  concurrent, wrong-kind, tamper, cancellation and ProcessHost Race tests.
  Unified Plugin Catalogue remains outstanding.
- **C1 — complete:** Local Workspace, Docker and opted-in Daytona are registered
  as host-reviewed `sandbox.backend` extension descriptors and Router
  acquires implementations only through the typed BackendResolver after Policy,
  generation and lease preflight. Unknown/offline drivers and unsupported
  capabilities fail closed. Tests include mutable backend capability expansion,
  invalid/wrong-kind descriptors, duplicate registration and existing governed
  Sandbox lifecycle/lease tests. Full repository and security checks pass.
- **C2/C3 remain:** persisted exact extension/installation binding, generic
  Sandbox lifecycle backend ports and installable external Sandbox RPC.
- **D and E:** not yet implemented; typed Sandbox descriptors alone do not
  imply external Sandbox/Tunnel driver support or Marketplace coverage.

Foundation verification: `pnpm check` (Go lint/test/race, SDK/protocol/example,
HTTP/TS/Wails contract drift, i18n/132 frontend tests, Web/Desktop builds) and
`pnpm security:check` passed. Isolated migration tests cover v1 inventory with
descriptor and original backup, restart idempotence, incompatible legacy
protocol rollback, and old software tree checksum/tamper protection.

## Explicit deferrals

OCI/Sigstore distribution and registries; WASI Component runtime; arbitrary combinations of several extension kinds in one native binary; unrestricted install from Web; generic dynamically loaded UI; unproven OS isolation of native plugins; E2B/Modal/Cloudflare adapters without acceptance tests; agent loops or new model-facing privilege escalation.

## External sources reviewed

- Terraform plugin protocol/version separation: https://developer.hashicorp.com/terraform/plugin/terraform-plugin-protocol
- HashiCorp subprocess plugin model: https://github.com/hashicorp/go-plugin
- Backstage typed extension points: https://backstage.io/docs/backend-system/architecture/extension-points/
- VS Code declarative extension manifest: https://code.visualstudio.com/api/references/extension-manifest
- OpenTelemetry typed component roles: https://opentelemetry.io/docs/collector/components/
- CNI execution and cleanup contract: https://github.com/containernetworking/cni/blob/main/SPEC.md
- Docker daemon privilege caveats: https://docs.docker.com/engine/security/protect-access/
- WASI 0.3 state (2026): https://wasi.dev/docs/roadmap

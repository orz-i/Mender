# ADR-0035: Separate governed Sandbox resources from Provider execution routing

- Status: Proposed
- Date: 2026-10-08

## Context

Mender is a Tool runtime and governance gateway, not an Agent/LLM loop. ADR-0030 introduced a built-in Workspace Provider exposing `workspace.read`, `workspace.search`, `workspace.edit`, and `workspace.shell` against an absolute host path. ADR-0032 admitted Provider activation and Tool invocation under Principals. ADR-0033/0034 introduced Runtime Budget and host-owned process isolation for local child processes.

The current `invocation/infrastructure/router` resolves a canonical ProviderID to a ToolExecutor. This is a *Provider executor router*, not a target execution-environment router. Workspace file tools use `os.Root` on the host; Workspace shell uses a selected host/restricted-host process launcher. Their storage and execution cannot simply be pointed at a remote container without introducing inconsistent semantics. `restricted_host` is not a VM/container sandbox and has no generic network egress policy.

We need multiple local and remote sandbox backends while keeping canonical Tool identity, Profile/Policy authority, credential isolation, audit, and adapter consistency.

## Decision (proposed)

### 1. Distinct domain concepts

- **Provider** produces canonical Tools and remains the unit of Provider activation.
- **Sandbox Backend** is a host-owned, explicitly trusted adapter with a documented capability set and truthful isolation guarantees. Examples: Local Workspace, Docker Engine, Daytona, E2B.
- **Sandbox Connection** contains driver-specific non-secret endpoint/configuration plus Credential References. It is not a Provider plugin installation.
- **Sandbox** is a stable, persisted desired-state execution resource: backend connection reference, immutable security-relevant execution configuration revision, owner/scope, resource limits, filesystem mapping, lifecycle strategy, and current status.
- **Sandbox Run** identifies one actual host/container/remote instance with an external ID, monotonic generation, observed state, and bounded metadata.
- **Sandbox Lease** grants a bounded, Principal/scope-attributed use of one admitted run and is released/reconciled independently of the Tool executor lease.
- **Execution Environment Router** resolves a *preconfigured* execution target for a Tool/Provider binding and admits access to a Sandbox. It is distinct from, and composed with, the existing Provider executor router.

Do not silently represent an unisolated host process as a strongly isolated Sandbox. UI and APIs must distinguish `host`, `restricted_host`, container, and stronger backend-specific isolation claims from verified runtime properties.

### 2. Stable placement, never from untrusted Tool arguments

V1 Workspace Provider configuration becomes `{"sandboxId":"<stable-sandbox-id>"}` (ConfigVersion 2). Tool arguments still contain only their operation fields and never choose a SandboxID, backend URL, host mount, or executable. The Sandbox binding is host-validated and persisted outside Tool schemas.

Resolve routing from persisted configuration and explicit trusted Invocation Context. Profile ToolBinding and Policy may *tighten* the permissible Sandbox/capabilities but cannot silently elevate or substitute a more permissive target. V1 binds one Workspace Provider to one Sandbox and does not migrate arbitrary MCP/OpenAPI/Model Tools to a Sandbox. Future Provider process placement must be explicitly implemented per compatible Provider type.

Retain the existing `io.mender.provider.workspace` PluginID and stable `workspace.*` SourceKeys so ProviderID and SourceKey-stable ToolIDs continue to identify the same tools. Change the display label to **Sandbox Workspace Tools**. The Sandbox itself is not a Provider and does not automatically populate the Catalog.

### 3. Capability-first backend contracts

Define small consumer-owned ports under a new `sandbox` bounded context:

- registry/discovery: `Capabilities` and `Probe` with backend availability, isolation class, platform, file/exec/search/lifecycle/stream/PTY/snapshot/mount support;
- lifecycle: `Provision`, `Inspect`, `Start`, `Stop`, `Destroy`, `Reconcile` as supported;
- operations: `Exec` (bounded argv/command, cwd, timeouts, stdout/stderr), `ReadFile`, `WriteFile/Edit`, `Search`, optionally `Transfer`/streaming;
- implementation-specific details, secrets, and credentials remain in infrastructure.

A capability must be both advertised and actually supported before any operation; unsupported operations reject explicitly without host fallback or shell emulation. `Exec` and file operations must address the *same Sandbox filesystem*. If a backend lacks safe file semantics, disable affected Workspace Tools or reject instance provisioning for the complete four-tool profile; do not secretly read/edit the host.

Prefer an argument-vector command transport where supported. Preserve the intentional shell tool contract only through an explicitly selected shell interpreter inside the admitted environment. No driver accepts arbitrary extra host privileges, image flags, Docker socket mounts, or raw credential values from Tool arguments.

### 4. Governance order and authority

The deterministic placement summary (SandboxID/BackendKind/security class/Scope) is derived from persisted configuration **before** Policy and without provisioning/opening a Sandbox.

Canonical Tool execution remains:

```text
Principal + Scope -> Profile ToolBinding -> Tool -> Validate arguments
-> Reviewed Governance Facts + persisted Sandbox placement metadata
-> Policy(tool.invoke) / Approval
-> Runtime Budget + ToolBinding guard
-> Sandbox admission policy / resource quotas
-> Execution Router: verify binding/capabilities, acquire/start lease
-> Provider Executor -> Sandbox operations
-> Result Guard -> bounded Invocation/Sandbox Audit -> release/reconcile
```

Introduce explicit `sandbox.provision`, `sandbox.acquire`, `sandbox.start`, `sandbox.stop`, and `sandbox.destroy` actions with trusted Principal attribution. Do not simulate them as arbitrary Tool invocations. A denial must prevent chargeable provisioning, network calls, credential resolution, mount access, and execution. Management operations must not bypass governance.

Existing `tool.invoke` and `provider.activate` semantics stay authoritative. A Provider instance may publish static Workspace Tools without starting or discovering an external Sandbox. Provider activation admission remains required for Plugin.Open and executor registration.

Credential secrets are resolved at the Sandbox backend connection boundary after admission; only Credential References persist. Logs, API projections, Audit, schemas, and error messages must not contain secret bytes, raw command output, unrestricted file content, or provider tokens.

### 5. Runtime, identity, and failure invariants

- Sandbox state transitions are typed and persisted: `defined -> provisioning -> ready -> stopping -> stopped -> destroying -> destroyed`, with explicit `failed`, `unknown`, and `orphaned` recovery states. Not every backend supports pause/resume.
- Store external IDs, operation idempotency keys, a generation/fencing token, and observed timestamps; use compare-and-swap state transitions. Reconcile orphaned/unknown instances after restart; never blindly recreate a possibly-provisioned remote Sandbox.
- Leases are bounded by Principal, scope, SandboxID, run generation, and expiry. Never share a stateful Sandbox between untrusted Principals without explicit ownership semantics.
- No automatic retry of side-effecting `shell/edit` after transport timeout, cancellation, or ambiguous backend response. Report an explicit uncertain outcome and reconcile.
- No automatic fallback from an unavailable Sandbox to another backend, `restricted_host`, or `host`.
- Existing Runtime Budget limits apply to Invocation, plus independently governed Sandbox run/provisioning concurrency, TTL, CPU, memory, disk, network policy, and remote cost/usage limits where enforceable.
- Driver-level cgroup/container/VM limits are not implied by current in-process Runtime Budget. Unsupported hard guarantees must fail capability checks rather than be shown as enforced.
- Network egress is denied by default when a backend can actually enforce it; otherwise the capability/trust warning is explicit and strict policies reject the target.
- Host mounts are deny-by-default and read-only unless explicitly granted through a trusted management operation. No privileged containers, Docker daemon socket mount, or implicit host HOME/credential mount.
- All administration uses the existing typed HTTP/OpenAPI/SDK/Wails/Desktop contract chain, with error and isolation-class parity.

### 6. First-backend implementation order

1. **Local Workspace**: preserve current functionality, with explicit `host` or `restricted_host` security labeling and no claim of VM isolation.
2. **Docker Engine**: first true managed local container backend, after engine/socket trust preflight; enforce resource/namespace/capability/mount limits and lifecycle cleanup. Docker daemon access is high authority and must remain host-owned.
3. **Daytona**: first remote backend candidate; Go SDK supports Sandbox lifecycle, files, commands, snapshots, and volumes.
4. **E2B**: second remote candidate after version-specific API/lifecycle and Go integration review.
5. **Modal/Cloudflare**: later adapters according to actual capability and deployment constraints; do not assume SDK shapes or file semantics are identical.

External sandbox backends are host-owned adapters in V1 rather than arbitrary untrusted Provider plugins. A separately versioned external Sandbox Driver protocol may be considered later.

### 7. Workspace ConfigVersion 1 hard-cut migration

Migrate existing Workspace Provider `{"root":"<absolute-path>"}` configurations forward to a local Workspace Sandbox resource. Derive the original process isolation choice from the current Runtime setting; preserve existing Provider IDs, SourceKeys, Tool IDs, Profile bindings, and Policy refs. Mark any `host` execution explicitly as unisolated.

Migration must be transactional and idempotent, with a forward-only schema/version bump, a pre-upgrade backup path, and **no dual-read, legacy parser, or silent fallback** after the cut. Invalid roots and unavailable restricted-host backends become actionable unavailable states, not auto-downgraded host access. Upgrading must not provision chargeable remote instances.

Do not alter `.agents/`, `.pi/`, or `.intent-loop/`.

## Alternatives considered

- **Extend the `IsolationMode` enum with Docker/E2B/Daytona**: rejected. An isolation toggle cannot represent environment ownership, persistent files, remote costs, running state, or backend capabilities.
- **Reuse the Provider executor router as Sandbox router**: rejected. Provider source and execution placement have different cardinality, authority, and lifecycle.
- **Make every Sandbox a Provider plugin**: rejected. A Sandbox does not inherently publish canonical Tools; it is execution infrastructure. Reuse a governed Workspace Provider as a Tool facade.
- **Give Tools a `sandboxId` argument**: rejected. This delegates placement authority to model-controlled input, undermining Policy and secret/host boundaries.
- **Launch a shell via each backend and implement all file operations through shell text**: rejected as a universal design because of consistency, escaping, capability, and audit risks.
- **Add a hosted Sandbox orchestrator to Mender**: rejected. Mender routes and governs existing environments and does not become a general compute provider.

## Consequences

Advantages: one reusable Tool governance pipeline, explicit runtime placement, multiple backend choices, truthful security isolation, and stable Tool identity across the Workspace migration.

Costs: new stateful lifecycle, migration, backend health reconciliation, cross-environment file APIs, separate Sandbox budgets, and more complex integration tests. V1 must scope to one environment per Workspace Provider and must not promise strong multi-tenant security until a suitable backend proves it.

## References

- [ADR-0030 Workspace native tools](0030-workspace-native-tools-provider.md)
- [ADR-0032 Provider activation and Principal](0032-provider-activation-principal-governance.md)
- [ADR-0033 Runtime Budget and restricted host](0033-runtime-budget-restricted-host-isolation.md)
- [ADR-0034 Runtime process governance](0034-runtime-owned-process-governance.md)
- Docker Engine security: https://docs.docker.com/engine/security/
- Daytona Go SDK: https://www.daytona.io/docs/en/go-sdk/
- E2B Sandbox SDK: https://e2b.dev/docs/sdk-reference/js-sdk/v2.6.2/sandbox
- Modal Sandbox Go API: https://modal.com/docs/sdk/go/latest/Sandbox
- Cloudflare Sandbox SDK 1.0 announcement: https://developers.cloudflare.com/changelog/post/2026-09-30-sandbox-sdk-1-0/
- gVisor security model: https://gvisor.dev/docs/architecture_guide/intro/

# Phase 25 Plan — Sandbox Execution Router and Sandbox Workspace Tools

Status: In Progress (documentation accepted; implementation in segments)

> Architecture decision: [ADR-0035](../architecture/adr/0035-governed-sandbox-execution-router.md) (Accepted on commencement of Phase 25). Each implementation cluster requires focused automated checks before its separate commit.

## Objective

Add a first-class Sandbox management surface and a governable, backend-neutral execution-environment routing mechanism. Convert the existing built-in Workspace Provider from a direct host-root implementation to a Sandbox-bound Tool facade, preserving Mender as a Tool/Resource runtime rather than an Agent loop.

The first useful product experience must support local Workspace and a managed local container adapter, while making remote Sandbox adapters possible without changing Tool contracts or recreating a separate Invocation pipeline.

## Existing source evidence (2026-10-08, HEAD 9a1c264)

- `backend/internal/invocation/infrastructure/router/router.go`: maps ProviderID to ToolExecutor, not execution target.
- `backend/internal/invocation/application/invoke_tool.go`: central Tool preflight, Policy/Approval, Runtime budget, binding guard, executor routing, result guard, and Audit. Sandbox admission belongs after governance and before side effects.
- `backend/internal/workspace/infrastructure/plugin/plugin.go`: Workspace plugin exposes four stable SourceKeys, `workspace.read/search/edit/shell`.
- `backend/internal/workspace/infrastructure/plugin/config.go`: ConfigVersion 1 accepts only absolute local root.
- `backend/internal/workspace/infrastructure/plugin/read.go`, `edit.go`, `search.go`: operate through a host `os.Root` and cannot be lifted into remote execution merely by changing Shell.
- `backend/internal/workspace/infrastructure/plugin/shell.go`: tool exposes intentional shell commands and bounded timeout/output/environment.
- `backend/internal/runtimegovernance/domain/isolation.go`: host/restricted_host are host process modes, not Sandbox backends.
- `backend/internal/runtimegovernance/infrastructure/processisolation/launcher.go`: macOS Seatbelt/Linux bubblewrap, no VM guarantee.
- `backend/internal/runtimegovernance/infrastructure/budget/controller.go`: per-Runtime in-memory Invocation/Provider activation concurrency quotas, not host/container CPU/memory/disk or remote cost.
- `packages/app/src/shell/management-shell.tsx`: sidebar grouping; Sandbox belongs in Runtime group.
- `packages/app/src/mender-app.tsx`: centralized routes.
- `contracts/api/openapi.yaml`, `packages/sdk/src/generated/schema.ts`, `backend/internal/desktop/binding`: required typed transport parity.
- Local scoped baseline: `go test ./backend/internal/runtimegovernance/... ./backend/internal/workspace/... ./backend/internal/invocation/infrastructure/router` succeeded. Full `pnpm check` was not run as part of planning.

### External backend evidence

| Backend | Capability and integration facts | V1 recommendation |
| --- | --- | --- |
| Local Workspace host/restricted_host | Existing paths and host process launcher; OS-level restricted-host only, not VM/container isolation | Preserve functionality; explicitly label security |
| Docker Engine | Local container lifecycle and resource controls; high-authority daemon socket; container isolation depends on privileges, daemon, mounts, and kernel | First managed local sandbox |
| Daytona | Maintained Go SDK (current docs require Go 1.25+), create/stop/start, process/file operations, snapshots and volumes | First remote integration |
| E2B | Remote Sandbox create/connect, commands, filesystem, lifecycle via SDK/APIs; verify exact version and Go client approach | Remote integration second |
| Modal | Go Sandbox API with exec, resources, volumes, tunnel and snapshot options, some APIs experimental | Candidate |
| Cloudflare Sandbox SDK 1.0 | As of 2026-09-30, application-owned Durable Objects manage containers and commands; deployment differs materially from generic remote sandbox APIs | Later dedicated Cloudflare adapter |
| gVisor/Kata/microVM | Isolation backends rather than drop-in Mender Tool Providers | Potential Docker/Kubernetes implementation options, separate threat-model review |

Official sources:
- https://docs.docker.com/engine/security/
- https://docs.docker.com/engine/containers/resource_constraints/
- https://www.daytona.io/docs/en/go-sdk/
- https://www.daytona.io/docs/en/sandboxes/
- https://e2b.dev/docs/sdk-reference/js-sdk/v2.6.2/sandbox
- https://modal.com/docs/sdk/go/latest/Sandbox
- https://developers.cloudflare.com/changelog/post/2026-09-30-sandbox-sdk-1-0/
- https://gvisor.dev/docs/architecture_guide/intro/

## Desired architecture and authority boundaries

```text
CLI / MCP / Skill / Web / Desktop
                 |
      Canonical Invocation
       Principal / Scope
       Schema / Governance Facts
       Policy / Approval
       Runtime Budget / Binding Guard
                 |
       Placement preflight (no side effects)
                 |
     Sandbox admission / hard limits
                 |
      Execution Environment Router
                 |
       Sandbox Lease / Session
                 |
     +-----------+------------+-----------+
     |           |            |           |
 Local Workspace Docker       Daytona     E2B
 (low/host)     (container)   (remote)    (remote)
     +-----------+------------+-----------+
                 |
  Sandbox Workspace Tool Facade: read / search / edit / shell
                 |
         Result Guard / Audit
```

Retain `ProviderExecutorRouter` for Provider identity. Only compatible provider executors (initially Sandbox Workspace Tools) acquire Sandbox execution sessions. Remote MCP/OpenAPI HTTP tools continue using their own Provider execution paths, unless a future explicit sandbox-compatible process adapter is designed.

### Persisted concepts

- SandboxBackendDescriptor: driver kind, version, available operations, truthful isolation labels, host requirements.
- SandboxConnection: backend endpoint/transport and Credential Reference; no raw tokens.
- Sandbox: ID, name, owner/scope, connection ref, desired state, template/image or local root/mount declaration, immutable security config revision, strategy (`persistent` initially), TTL, CPU/memory/disk limits and network policy when enforceable.
- SandboxRun: SandboxID, generation/fencing token, opaque backend external ID, observed state, timestamps, last bounded error.
- SandboxLease: operation ref, Principal, Scope, SandboxID, generation, admission deadline, release/recovery state.
- SandboxCapabilities: file reads/edits/search, command exec, streams, PTY, mounts, snapshot, network restrictions, CPU/memory/disk enforcement, lifecycle actions.

No Solver, Agent, Prompt, Memory or autonomous Planner entities.

### Proposed Go port shape (illustrative, not an implementation contract)

```go
type ExecutionEnvironmentRouter interface {
    Resolve(ctx context.Context, req PlacementRequest) (Placement, error)
    Acquire(ctx context.Context, placement Placement, principal PrincipalRef) (SandboxSession, Lease, error)
}
type SandboxBackend interface {
    Probe(ctx context.Context, connection Connection) (Capabilities, error)
    Provision(ctx context.Context, req ProvisionRequest) (RunHandle, error)
    Inspect(ctx context.Context, handle RunHandle) (ObservedState, error)
    Stop(ctx context.Context, handle RunHandle) error
    Destroy(ctx context.Context, handle RunHandle) error
}
type SandboxSession interface {
    Files() SandboxFilesystem
    Processes() SandboxProcesses
    Close() error
}
```

Actual interfaces live near their consumers and split further where a backend does not offer a capability. Driver implementation stays outside `domain/`; no vendor SDK types flow into the application/domain model. The provisional Go shapes must be reconciled with existing `ExecutionRequest` and lease/release ownership before implementation.

### Placement rules

1. Selection is persisted in trusted Sandbox/Provider configuration, never controlled by Tool arguments.
2. Resolve Sandbox identity and reviewed placement Governance Facts before Policy without opening/provisioning a sandbox.
3. Existing global Policy and Profile ToolBinding restrictions remain monotone; Sandbox admission adds a separate tightening decision.
4. V1: one Sandbox per Workspace Provider, fixed target across Profile bindings; cross-Profile overrides not supported.
5. A stopped Sandbox may be started only after explicit `sandbox.start` authorization; automatic start policy is opt-in and resource-bounded.
6. No alternative Sandbox, weakened security class or host execution fallback when the bound Sandbox is unavailable.
7. A per-Principal lease prevents unsafe sharing between unrelated Principals. Non-owner access is denied unless explicitly delegated.
8. Managed remote or local Sandbox lifecycle must not be spawned during Provider catalog discovery.
9. Invocation failures/cancel/timeouts preserve canonical outcome and Audit semantics, including an explicit uncertain execution state where the remote result is unknown.

## Workspace Provider hard cut

### Provider contract

Keep:
- PluginID `io.mender.provider.workspace`;
- stable `workspace.read`, `workspace.search`, `workspace.edit`, `workspace.shell` SourceKeys;
- existing canonical argument/result schemas whenever equivalent semantics can be preserved;
- Policy reviewed Governance Facts and static discovery without environmental side effects.

Change:
- display name from `Workspace` to **Sandbox Workspace Tools**;
- ConfigVersion 1 `{"root":"/host/root"}` becomes ConfigVersion 2 `{"sandboxId":"sbx_..."}`;
- direct filesystem access is removed from Workspace Plugin and implemented by selected Sandbox backend;
- Shell uses the Sandbox process capability, while `read/search/edit` operate in the same Sandbox filesystem;
- no implicit escape to host on unsupported backend operations.

### Migration

1. Create forward-only Sandbox tables/constraints and deterministic stable Sandbox IDs from existing Workspace Provider identity.
2. For each v1 Workspace Provider, create Local Workspace Sandbox config using existing canonical root and the active Runtime WorkspaceShell isolation snapshot.
3. Rewrite provider config to v2 in the same idempotent upgrade operation (or with a safe journaled cross-store migration if storage ownership requires separate stores). Preserve ProviderID, SourceKey-derived ToolIDs, Profile bindings, and audit history.
4. Keep a marked, non-executable unavailable state if a root is invalid or a restricted-host backend cannot be enforced; no host fallback.
5. Remove v1 runtime parsing and UI `root` forms. Test old data -> current schema, restart, error rollback and downgrade refusal.
6. Warn that `host` mode is not isolated and requires explicit trusted local use; don't silently relabel it as secure.

## Sandbox management UI / Product design information architecture

### Navigation

Add **Sandboxes / 沙盒** in the existing Runtime group, directly beneath Runtime and before Profiles.

Routes:
- `/sandboxes`: list, filter, creation entry, health states.
- `/sandboxes/new`: choose backend then backend-specific configuration.
- `/sandboxes/:sandboxId`: stable resource detail, state transitions, permitted actions, usage/relationships.
- Avoid a dense single dialog containing every driver configuration.

### Sandbox list (one-screen overview)

Header: `Sandboxes` / `沙盒`; description: manage where Sandbox Workspace Tools run. Primary action: `Create sandbox`.

A compact metrics strip: total; running; stopped; unhealthy. Filter by backend, state, isolation strength, name, linked Provider. Paginated list/table (cursor where backend supports it) with columns:

`Name | Backend | Isolation | State | Workspace | Last activity | Actions`

Show accurate unsupported/unknown status instead of `Running`/secure guesses. Row actions: View, Start, Stop, Delete with accessibility tooltips. Disable or explain prohibited operations. Avoid embedding full instance logs or resource configuration in the list.

### Create flow

1. **Choose backend** — cards: Local Workspace (host/restricted_host), Docker, Daytona; E2B/Modal/Cloudflare labeled planned until implemented, never clickable as working integration. Each card shows availability, security boundary, local/remote, supported operations.
2. **Configure selected backend** — focused form only for that driver:
   - Local Workspace: name, select local directory, requested host isolation with explicit warning if unisolated.
   - Docker: name, pinned image digest/tag with warning, CPU/RAM/disk, read-only/writable mounts, network egress, TTL, trusted Engine status.
   - Daytona: name, Connection/Credential selector, image/snapshot, environment size, lifecycle/TTL, file transfer, network and cost/limits if supported.
3. **Review and create** — show effective authority, mount changes, network exposure, persistent-vs-ephemeral cost warning, creation preflight, Policy/Approval outcome. Only then provision.
4. **Attach to Workspace Provider** — choose existing or create new Workspace Provider; hide the Sandbox ID behind a selector. No handwritten opaque IDs.

### Detail page — one concern per screen

Header: Name, backend, current state, last inspection, supported actions.

Tabs:
- **Overview**: status, run generation/ID (copy), backend health, linked Workspace Provider/Profiles, active leases, resource limits.
- **Files & Execution**: sandbox root and capabilities, safe read-only browser preview; terminal/exec only when supported and authorized. Do not render an interactive terminal by default.
- **Security**: effective isolation, mounts and write scope, egress, Credential References (never values), Principal/scope owners, policy decisions.
- **Activity**: sandbox lifecycle events and related Invocation audits (IDs/status only, redacted summaries).

Always label `host` as high-authority, non-sandbox execution. Explain `restricted_host` is not a container/VM. Prefer exact security properties over blanket `secure` badges. Expose failed state reason and remediation without leaking credentials or command output.

### Existing UI touchpoints

- Providers: Workspace plugin form becomes Sandbox resource picker + supported tool summary; show deep link to Sandbox details.
- Profiles: show Sandbox association through bound Workspace tools, not a duplicated editable Sandbox assignment in V1.
- Runtime: retain host-process `WorkspaceShell / ExternalProvider / TunnelClient` isolation, add navigation/help to managed Sandboxes; do not add Docker/E2B to existing Isolation enum.
- Catalog: optionally display a Tool's Sandbox-bound Provider origin.
- Audit: add SandboxID/BackendKind/RunGeneration/terminal status as safe, queryable metadata.
- i18n: complete English and Simplified Chinese strings; keyboard navigation, labels, truncation and responsive behavior.

## Safety / acceptance tests

### Governance & isolation

- denied `tool.invoke` cannot trigger `sandbox.acquire/start/provision`, credential resolution, network call or filesystem operation;
- denied `sandbox.start/provision` prevents the side effect even if `tool.invoke` was allowed;
- unverified remote Principal cannot assume local Runtime owner authority;
- Tool arguments cannot change Sandbox, mount, runtime, credentials, backend endpoint, image or isolation;
- stale binding/invalid generation, backend unavailable, capabilities missing and runtime mode mismatch all fail closed;
- sandboxed read/search/edit/shell operate on exactly one backend filesystem, never host filesystem in remote mode;
- root traversal, symlinks, races, file size, UTF-8, edit replacement and search budget remain bounded;
- namespace/capability/user/resource limits are enforced by selected backend or reported unavailable;
- no inherited blanket HOME/host environment, credential/log leak, Docker socket mount, privileged container or unrestricted host bind;
- lease cleanup is idempotent under cancellation, errors, concurrent stop and restart.

### Lifecycle / correctness

- typed state transitions, generation CAS, external ID persistence, idempotent provision and destroy, crash reconciliation;
- concurrent invocation vs stop/delete yields deterministic outcomes;
- unknown remote exec response never auto-retries a side-effecting command;
- remote timeout and cancelled command do not produce a false success;
- no orphaned containers after test teardown; leaked remote resources are enumerated for human cleanup if deletion fails;
- Sandbox run caps and per-Principal/Backend quota are enforced separately from existing Invocation budget;
- backend unsupported operations never fall back to host shell;
- observability redacts commands, file payloads and secret bytes as dictated by existing audit policy.

### Compatibility and UX

- migrate pre-Phase-25 DB snapshots while preserving ProviderID/ToolIDs/Profiles and explicit host-isolation classification;
- reject future schema versions and unsupported downgrade, no legacy v1 config decoder post-cut;
- CLI/MCP/Skill/Web/Desktop invoke the same workspace tool into the same Sandbox when bound;
- typed OpenAPI/SDK/Wails generated artifacts drift checks pass;
- Sandbox selector is type-safe and no typed-ID text field appears in normal user flow;
- cards show correct capability/availability and never present planned backends as working;
- empty, error, loading, policy-denied, engine-unavailable, restart-recovery states have tested UI;
- pagination, long path truncation, mobile layout, keyboard/tooltip and bilingual content are tested.

## Implementation sequence — document first, segmented commits

**Cluster A — Governance decision (current planning stage)**
- ADR-0035 proposed, Phase 25 plan, ADR index, roadmap.
- Source-backed backend matrix and UX contract.
- No runtime implementation until architecture accepted.

**Cluster B — Domain / contracts / persistence**

First landed slice: stable Sandbox identity, strict backend/isolation/capability classification, typed transition and run-generation model, persistence with forward/future-version migration checks, side-effect-free placement resolver and explicit lease contract. This slice must not register a remote backend or claim that Workspace was already migrated.
- `backend/internal/sandbox/{domain,application,infrastructure}` contexts, backend registry, lifecycle state machine, connection/resources/mount schema.
- Host-owned policy actions, Principal/Scope, migration, resource/cost budget, typed DTOs and deterministic test fakes.
- Adapters expose backend capabilities and safe list/get operations.

**Cluster C — Local Workspace hard cut**
- LocalWorkspace backend and Sandbox workspace session.
- Workspace Plugin ConfigVersion 2 `sandboxId`, existing Tool IDs and schemas preserved.
- Side-effect-free discovery, runtime binding verification, forward-only v1 provider migration.
- OS restricted-host mode remains a local execution implementation, not remote Sandbox fallback.

**Cluster D — First managed local Sandbox**
- Docker Engine driver with preflight, resource limits, mount/network policies, lifecycle cleanup, no privileged/socket escape.
- Safe file/exec operation parity and integration tests gated on available Docker daemon.
- Maintain all host safety invariants.

**Cluster E — Management UI and client parity**
- `/sandboxes` and detail/create flow; backend-focused forms and Workspace Provider selector.
- Management HTTP/OpenAPI/generated SDK/Wails binding parity.
- Chinese/English i18n; normal tool/profile/audit navigation preserved.

**Cluster F — Remote first adapter and closeout**
- Daytona adapter with credential-bound Go SDK, authoritative lifecycle, run observation, output bounds, file operations, idempotency and cost guard.
- E2B adapter separately scoped if desired after Daytona contract proves portability.
- Focused, race, lint, TS checks, API/bindings drift, security and full `pnpm check` before completion.

Suggested commit categories: `docs(architecture)`, `feat(sandbox): domain-and-persistence`, `feat(workspace): route-through-sandbox`, `feat(sandbox): docker-backend`, `feat(app): sandbox-management`, `feat(sandbox): daytona-backend`, `docs(governance): complete-phase-25`.

## Exclusions

- LLM calls, Agent planning/loop/memory.
- General-purpose cloud compute marketplace or Kubernetes orchestration.
- Relocating arbitrary MCP/OpenAPI remote providers into Sandbox.
- Cross-tenant strong-isolation guarantees on Local Workspace or generic Docker.
- Transparent migration of active Sandbox instances across different backends.
- Automatically replaying non-idempotent Tool invocations.
- Unreviewed external Sandbox driver RPC protocol in V1.
- Broad public inbound Sandbox exposure or implicit Port/Tunnel publication.

## Completion / review gates

Before declaring Phase 25 complete:

1. ADR accepted and contracts reviewed.
2. Real backends prove read/search/edit/shell parity over one selected filesystem.
3. Full governance/security tests prove denial-before-side-effects and strict no-fallback behavior.
4. Workspace v1 migration demonstrably preserves stable Tool identities and Profile bindings.
5. UI navigates to Sandbox list, backend-specific create, detail/security/activity and Workspace Provider binding across Web/Desktop.
6. All entry points use canonical Invocation and auditable Sandbox lifecycle actions.
7. Docker and at least one remote Sandbox adapter have documented availability, resources and cleanup behavior, or the phase is explicitly split with honest partial status.
8. `pnpm check` and `pnpm security:check` are recorded; no test passes claimed from documentation alone.
9. `.agents/`, `.pi/` and `.intent-loop/` remain untouched and uncommitted.

## Key research unknowns to resolve during implementation

- Trust requirement for Docker Engine socket on supported macOS/Linux deployment modes.
- Whether a remote backend offers all four Workspace operations with required path, symlink and atomic edit safety.
- Backend-specific hard guarantees for egress policies, resource caps, mounts and TTL.
- Remote operation idempotency, cancel semantics and reconciliation after power loss.
- Usage/billing observation granularity and whether quotas can actually be enforced by each vendor.
- Tenant/Principal ownership when a Profile is remotely exposed through MCP/Tunnel.

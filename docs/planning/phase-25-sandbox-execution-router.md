# Phase 25 Plan — Sandbox Execution Router and Sandbox Workspace Tools

Status: Complete (bounded core; Daytona experimental and disabled by default; real remote acceptance deferred)

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

## Implementation checkpoint — Foundation B1 (2026-10-08)

This is **not** Phase 25 completion. The first implementation slice is intentionally isolated from active Invocation and does not create Sandbox instances.

Committed in separate segments:

- `fcbd033` — accept ADR-0035, start Phase 25 governance scope and roadmap.
- `4614c44` — add `backend/internal/sandbox` domain model, SQLite resource/run persistence, CAS run transitions, explicit Backend/AdmissionGate ports, and side-effect-free Resolve / gated Acquire with bounded and idempotently released leases.

Verified properties for this slice:

- deterministic Sandbox IDs; typed backend and truthful isolation classification;
- invalid local roots / remote host roots rejected by domain validation;
- lifecycle transitions typed and persisted, stale transition CAS rejected;
- future-version SQLite schema rejected; restart preserves run generation/state;
- placement Resolve performs no backend call;
- Acquire denies unauthorized Principal or missing gate, missing backend/capability, stale generation and concurrent saturation without host fallback;
- acquired lease release closes the session at most once.

Verification evidence on the current local workspace:

- `go test -race ./backend/internal/sandbox/...`: passed.
- `golangci-lint run ./backend/internal/sandbox/...`: 0 issues.
- `pnpm check`: exit code 0 (includes Go lint/test/race, SDK/proto/bindings checks, frontend lint/typecheck/test/build). Bundler reports existing >500kB chunk warnings.
- `pnpm security:js`: no known high-severity production dependency vulnerabilities.
- **`pnpm security:check`: failed**. `govulncheck` reports 12 reachable Go standard-library vulnerabilities in the installed Go 1.27.1 toolchain (fixed in Go 1.27.2) and vulnerable `golang.org/x/net@v0.58.0` entries (fixed in v0.60.0), with some package/module findings marked non-reachable. This is an independent dependency/toolchain remediation gate; do not represent the security scan as green. Review affected module versions and upgrade/re-run the scan in a separate evidence-backed hardening segment.

Still pending before this is end-to-end:

1. Production Sandbox authorization actions (`sandbox.provision/acquire/start/stop/destroy`) and binding to the existing Policy/Approval/Audit chain; currently the gate is a required interface only and has no production implementation.
2. Wire the Sandbox resource store and execution router into Runtime composition with health/reconciliation and lifetime ownership. No actual backend is registered yet.
3. Sandbox backend Session operations for one filesystem and shell, plus verified local Workspace backend.
4. Forward-only Workspace Provider ConfigVersion 1→2 migration, maintaining PluginID/ToolIDs/Profiles and removing direct host file routing in one hard cut.
5. Managed Docker backend; remote Daytona/E2B backend and cost/resource governance.
6. Real Sandboxes navigation, list/create/detail, typed OpenAPI/SDK/Wails/Desktop, English/Chinese parity, and end-to-end tests.

Do not confuse Foundation B1 test green with Sandbox runtime integration. No `.agents/`, `.pi/`, or `.intent-loop/` paths were added to commits.

## Follow-up slice B2 — governed Sandbox admission and Runtime registration

Status: Planned for implementation in separate commits; foundation B1 is already complete.

Contract refinements before code:

1. Extend the existing action-aware Policy vocabulary with `sandbox.acquire`, `sandbox.provision`, `sandbox.start`, `sandbox.stop`, `sandbox.destroy`, rather than introducing a second implicit Policy engine. Sandbox operations carry explicit Principal/Origin/Scope and validated `sandbox_id`/`sandbox_backend`/`sandbox_isolation` **reviewed, bounded resource facts**, never raw Tool arguments, root paths or credentials. ProviderID/ToolID are empty for these actions. Existing Provider/Tool selectors must not accidentally match Sandbox-only actions.
2. Persist an isolated data-minimized `SandboxActionAudit` for Policy/approval/admission outcomes; denied calls must not reach backend Open, filesystem, external requests, or credential resolution. Approvals must receive the typed SandboxID and Action, not a dummy ProviderID.
3. Keep the existing `Invocation -> ProviderExecutorRouter` pipeline unchanged until Workspace ConfigVersion 2 is migrated. A new Sandbox Router cannot be reachable from user-controlled Tool arguments or silently bypass the canonical Tool Policy. Parent tool.invoke admission remains the prerequisite for `sandbox.acquire` during the future Workspace hard cut.
4. Runtime owns the Sandbox SQLite store and backend Registry lifetime. A production AdmissionGate must be present even when no backend is installed; no null/permissive stub is allowed. Add the Local Workspace backend only when both read/edit/search and shell operate within one backend identity. Do not describe `restricted_host` as container/VM isolation.
5. Existing Global Policy default decision applies to Sandbox actions, *in addition to* immutable owner equality, capability checking and explicit sandbox action audit. Persistent Policy rules support Sandbox-only action + reviewed `sandbox_id` matching and cannot loosen a parent denied tool.invoke.
6. Segmented commits: B2 governance/domain/tests; B3 Runtime wiring and tests; C1 Local Workspace Session operations/tests; C2 explicit hard cut/migration only after complete cross-entrypoint checks. No UI claims before typed API/client parity.

Remaining gates: introduction of a Sandbox backend cannot start a process or read/write any file prior to Policy authorization; Provider discovery must remain side-effect-free; a rejected Sandbox cannot fall back to Host.

## Follow-up checkpoint — B2/B3 completed (2026-10-08)

Status: Foundation B2/B3 implemented and committed; Phase 25 remains **In Progress**. No Workspace Provider hard cut, Docker/remote backend or Sandbox UI was shipped by these commits.

Segregated commits:

- `b962e12` — document Sandbox actions, reviewed placement facts, audit invariants and Runtime wiring order *before* changing the code.
- `c2ad1d7` — add `sandbox.acquire/provision/start/stop/destroy` as distinct Policy actions; allow bounded `sandbox_id`, backend and isolation facts for Sandbox action evaluation while preventing mixing Sandbox and Tool/Provider selectors. Add Sandbox-specific fail-closed acquisition approval/admission with an explicit `SandboxID` approval request, bounded durable admission audit (SQLite schema v2), and updated OpenAPI/SDK and bilingual Policy UI. At present the production admission adapter implements only `sandbox.acquire`; other action names are reserved for later admitted lifecycle operations.
- `7594b49` — compose the Sandbox SQLite inventory and governed, fail-closed Execution Router into the shared Runtime, with close ownership and a restart/persistence test. No sandbox backend is registered by this slice and the existing Workspace Provider still executes through the previous path.

Validation evidence:

- `go test -race ./backend/internal/sandbox/...` passed after B2.
- `go test -race ./backend/internal/bootstrap ./backend/internal/sandbox/...` passed after B3.
- `pnpm check` exit code **0** after B2 and again after B3; Go lint/test/race, generated SDK/desktop bindings, frontend lint/typecheck/test/build passed. The frontend continues to emit non-fatal large-bundle warnings.
- The known `pnpm security:check` blocker (Go 1.27.1 and vulnerable `golang.org/x/net` versions from the earlier Phase 25 scan) has **not** been remediated here. Its status must not be described as green.
- The only unrelated untracked directories remain `.agents/`, `.pi/` and `.intent-loop/`; none are committed.

Critical limitations and next mandatory gates:

1. Runtime currently registers **zero** actual Sandbox backends; `Acquire` rejects unavailable drivers with no fallback. The new Router is not yet wired into the existing Workspace Plugin and no user-facing Sandbox resource creation/update operation is exposed.
2. A concrete portable Workspace Session contract must implement safe `read/search/edit/shell` on **one backend filesystem**, with bounded input/output, symlink containment and host-owned shell isolation; only then may Local Workspace be registered.
3. `sandbox.provision/start/stop/destroy` require their own admitted use cases and lifecycle audit; their Policy action enumerations alone are not implementation of those operations.
4. Workspace ConfigVersion 1→2 migration must be forward-only, rollback-safe and preserve Provider/Tool identity, existing Profile bindings and governance. No dual parser, default host fallback or silent privilege escalation.
5. Sandbox management UI/HTTP/OpenAPI/Wails parity is still pending, as are Docker, Daytona/E2B and remote resource/cost controls.

## Follow-up cluster C1 — Local Workspace backend extraction (implementation contract)

Status: Ready for implementation; document-first boundary before touching runtime code.

- Introduce the first concrete Sandbox backend at `sandbox/infrastructure/localworkspace`, with a narrow `WorkspaceSession` application port exposing all four existing canonical operations. Its backend must open **one** validated, symlink-resolved local root and bind both structured files and shell execution to this same root.
- Reuse the existing bounded read/search/edit/shell algorithms by extracting them, not by teaching remote drivers to use a local `os.Root`, and preserve canonical SourceKeys, JSON argument/output schemas, symlink denial, exact edit semantics and bounded results.
- `Backend.Open` must validate `local_workspace` identity, `host/restricted_host` isolation, `ready`/matching run generation, and the configured root before returning an `os.Root`-owned session. It must not spawn a subprocess or silently create host directories.
- Shell command launcher is **host-owned** and explicitly receives the Sandbox's selected isolation mode. No use of the mutable Runtime-wide default to override the persisted Sandbox isolation; restricted-host must fail closed when unavailable. Shell remains high authority for `host` and must not be represented as a VM/container sandbox.
- A Local Workspace adapter must not read/write an unrelated root or execute outside the selected Sandbox. All operations require explicit capability selection; unknown capabilities fail closed. Full read/search/edit/shell parity tests must exercise one common root and confirm the reduced environment, bounded command output/timeouts, path traversal and symlink protections.
- Keep the existing Workspace Provider ConfigVersion 1 live only until C2 migration is fully implemented and verified. The C1 commit may temporarily retain the current tool implementation while introducing the tested backend in isolation; C2 will remove the duplicated legacy execution path in a **single hard cut**, with no persistent compatibility bridge.
- Acceptance: `go test -race ./backend/internal/sandbox/...`, focused bootstrap/process isolation checks, `golangci-lint`, and a clean, scoped commit; C1 does not claim that Invocation already invokes the new backend.

## Follow-up cluster C2 — Workspace plugin hard cut prerequisites

Status: Pending C1.

- Provider ConfigVersion 2 only accepts `{ "sandboxId": "sbx_..." }`; reject root, legacy version and unknown fields at runtime, and make descriptor/management/CLI contract match.
- Workspace Tool execution receives validated Invocation's trusted `GovernanceContext` (never from Tool arguments), resolves the bound Sandbox, acquires the governed lease, delegates to the backend `WorkspaceSession`, then releases it on every outcome. Profile Tool and global Policy must remain authoritative before any sandbox acquisition.
- Before switching the parser, perform one **transactional forward migration** of Workspace Provider v1 records to local Sandbox rows/run records in the same SQLite file; derive stable Sandbox IDs from Provider IDs, preserve Tool/Provider IDs and bindings, record original roots/selected isolation without weakening it, and fail closed on invalid legacy data. Never auto-provision remote chargeable resources.
- Hard-cut test matrix must prove migration rollback and restart, catalog/Profile identity stability, shell/files backend consistency, absence of v1 runtime decoding, and CLI/HTTP/Wails/Web parity. No automatic host fallback, even if a bound Sandbox backend is unavailable.

## Implementation checkpoint — Local Workspace backend C1 and migration preparation (2026-10-09)

Status: Local Workspace backend is available as a **governed backend within the Sandbox Router**. Workspace Provider hard-cut is **not yet applied**; Phase 25 remains In Progress.

Commits, in order:

- `649830b` — document C1 extraction and C2 migration invariants before implementation.
- `e313a4a` — add the Local Workspace backend and WorkspaceSession with root-contained, bounded `read/search/edit/shell` operations, explicit isolation-mode handoff and backend lifecycle checks; preserve the existing provider execution path until hard cut.
- `2da240a` — introduce a consumer-owned Sandbox Lease/ExecutionRouter port, register Local Workspace in Runtime composition, ensure the persisted selected isolation is sent to the host-owned process-isolation launcher, verify governed access against the real persistent store and explicit no-fallback when a remote backend is not registered.
- `dd9840b` — add an **unwired**, transactional, idempotent migration helper for legacy Workspace ConfigVersion 1 to stable Local Workspace Sandbox resources and ConfigVersion 2 references; add rollback/retry/restart validation, and deny *all* restricted_host Sandbox operations before opening the filesystem when the required host isolation backend is unavailable.

Validation:

- `go test -race ./backend/internal/sandbox/...`: passed after backend extraction.
- Focused backend/bootstrap migration, isolation and no-fallback tests: passed.
- `golangci-lint run ./backend/internal/sandbox/...` and bootstrap-focused lint: zero issues.
- `pnpm check`: exit code 0 after backend extraction, again after Runtime registration, and after migration helper security preflight (includes Go lint/test/race, TS checks, Wails/OpenAPI generated contract checks, frontend tests/build). Bundler continues to report non-fatal large-chunk warnings.
- `pnpm security:check`: the previously identified Go 1.27.1 / x/net vulnerability blocker remains open and was not reclassified as passing.

Important honesty and safety boundaries:

1. The migration helper is tested but **not invoked during Runtime startup**. This is deliberate; activating it before converting the Workspace Provider runtime parser, CLI, and frontend contracts would break deployed v1 configurations.
2. Current Workspace Provider still executes its old v1 tools. The new backend is accessed only through explicitly governed Sandbox Router leases; duplication is temporary and must be removed by the C2 cut, not maintained as a compatibility fallback.
3. The Local Workspace backend's `host` mode is not a Sandbox security guarantee. `restricted_host` remains OS policy isolation, not a container or VM. A missing required isolation backend fails closed at Sandbox Open, including for structured file operations.
4. C2 must atomically wire migration + ConfigVersion 2-only Provider + trusted Invocation Context propagation + CLI/Web/Desktop contract support and remove legacy execution files. It must preserve existing stable ProviderID and ToolID identities and reject missing/stale Sandbox bindings.
5. Docker/Daytona/E2B backend implementations and Sandbox list/create/detail UX remain pending; do not represent those as working features.
6. `.agents/`, `.pi/`, `.intent-loop/` remain untracked and untouched.

## C2 execution handoff — hard-cut checklist (2026-10-09)

Implementation gate for this segment:

1. Add `GovernanceContext` to the already policy-admitted canonical `ExecutionRequest`; set it exactly once in `InvokeTool.execute` from trusted invocation context. It is not extracted from JSON Tool arguments or arbitrary Provider metadata.
2. Workspace Provider ConfigVersion 2 accepts only `{ "sandboxId": "sbx_..." }`. Its activation/discovery remains side-effect free: `Plugin.Open` parses and retains the persistent SandboxID but never acquires a Sandbox, opens a shell, resolves credentials, or touches a filesystem.
3. Each Workspace tool invocation maps the existing stable SourceKey to a **fixed** Sandbox capability, calls `SandboxRouter.Resolve` and `SandboxRouter.Acquire` with trusted `ExecutionRequest.Context`, executes the selected Sandbox WorkspaceSession, and closes the lease exactly once. The canonical `tool.invoke` Policy/approval, Runtime Budget, Result Guard and Audit stay outside this facade. Missing binding, wrong capability, unavailable backend and denied `sandbox.acquire` fail closed.
4. Provider ConfigVersion 1 is decoded **only** by the forward-only SQLite startup migration. Creation/update paths and Plugin.Open reject v1 immediately after the hard cut. Existing ProviderIDs, SourceKey-stable ToolIDs, Profile bindings, Policy and historical Audit are not regenerated. The isolation mode is snapshotted from the pre-cut Workspace Runtime setting for existing providers; subsequent global-isolation changes cannot silently elevate a persisted Sandbox.
5. The end-user Provider form and CLI must stop writing arbitrary `root` into Workspace Provider configuration. Workspace Provider receives an inventory-selected sandbox identifier instead. A governed Sandbox creation/list API and client workflow is a separate management cluster that must not be disguised by a free-form opaque-ID text field.
6. Remove duplicate host read/edit/search/shell execution from the Workspace Provider package after the cut; keep only reviewed Governance Fact parsing and canonical tool schemas. File operation algorithms live inside Sandbox backends.
7. Tests must cover v1 persisted DB upgrade on Runtime start, idempotent restart, strict v2 rejection of `root`, denied `tool.invoke` and `sandbox.acquire` (zero backend effects), `read/search/edit/shell` on the same Sandbox root, and stale/missing sandbox bindings. Existing tests must be adapted to the **new** source of authority rather than retaining v1 helpers.

Non-goals: no new compatibility bridge, no Agent loop, no auto provisioning, no remote fallback, no unreviewed executable/mount flags from Tool input. Remain In Progress until management adapter parity and tests are green.

## Implementation checkpoint — C2 hard cut and local Sandbox management (2026-10-09)

Phase 25 is **In Progress**, not complete. The post-C1 milestone implements the Workspace Provider hard cut and the first local Sandbox management experience; it does **not** implement Docker, Daytona, E2B, lifecycle stop/destroy, remote budget enforcement or a general multi-tenant sandbox security boundary.

Document-first and segmented commits:

- `81c0932`: establish explicit C2 execution/authority/migration requirements before the hard cut.
- `b5fcfe7`: admitted local `sandbox.provision` with durable Policy decision and isolated Sandbox/Run persistence, including denied-before-filesystem-observation tests.
- `a769885`: hard-cut Workspace Provider ConfigVersion 2 to bound `{ "sandboxId": "sbx_..." }`; remove the v1 host-based read/search/edit/shell execution implementations; preserve stable `workspace.*` SourceKeys; attach validated Invocation GovernanceContext to the delegated Executor request; transact the v1→v2 startup migration in a single SQLite DB while preserving ProviderID/ToolID/Profile bindings. CLI now supports `mender sandbox add local`, `mender sandbox list`, and `mender provider add workspace --sandbox`. An integration test constructs v1 persisted configuration, upgrades/restarts the runtime and checks real tool invocation plus identity preservation. Existing isolation settings become a migration-time **snapshot**, not mutable Sandbox authority.
- `a332d7e`: add `GET /api/v1/sandboxes`, `GET /api/v1/sandboxes/{sandboxId}`, `POST /api/v1/sandboxes/local`; typed OpenAPI, SDK and Wails binding/client parity; HTTP and Desktop governance/validation tests. Management operations are local-only and use explicit Principal/Policy; remote/container backends are not advertised as operational.
- `df35e4d`: add Runtime sidebar `Sandboxes` and `/sandboxes`, `/sandboxes/new`, `/sandboxes/:sandboxId` pages with local-backend-first creation, explicit host/non-VM warning, state and capability metadata, search/filter/pagination, and planned-backend disabled cards. The built-in Workspace Provider's configuration editor uses a filtered *existing Sandbox selector* (ready local only), not a free-text opaque ID. Added regression test and updated provider UI test fixtures.

Verification on the local Mender checkout (commit `df35e4d`):

- `pnpm check` **exit code 0**, including Go full/race/lint, Provider plugin SDK/proto, generated OpenAPI/Wails bindings, TypeScript/ESLint, i18n, Vitest and Web/Desktop production builds.
- Vitest **11 test files, 72 tests passed**; focused Workspace Sandbox selector test ensures only ready local backends are selectable and no manual ID field exists.
- Startup migration proves persisted ProviderID/ToolID/Profile identity and real workspace read after restart. HTTP/Wails governance tests assert a denied Sandbox provision has no persisted instance.
- Frontend production builds emit pre-existing non-fatal large-chunk warnings (>500 kB). Performance/codesplitting remains an independent backlog item.
- `pnpm security:check` was **re-run and failed**: local Go 1.27.1 standard-library reachable advisories (12, patched by Go 1.27.2) and `golang.org/x/net` v0.58.0 vulnerability metadata (patched in v0.60.0); scan remains a release blocker. `pnpm check` green is **not** a security clearance.
- `.agents/`, `.pi/`, `.intent-loop/` remain untouched and untracked. Commits are local and have not been pushed.

Next gates, in priority order:

1. Harden local Sandbox resource integrity: prevent orphaned Provider → Sandbox links and unsafe sharing of the same local root across independently governed Sandbox IDs, and verify access to non-existent Sandbox IDs cannot be configured through normal Provider flows.
2. Design and implement governed `sandbox.start/stop/destroy` lifecycle, typed state transitions, durable audit and active-lease-aware cleanup (only where the backend supports the action). Preserve safe immutable host/restricted-host decisions for local Sandbox resources.
3. Design Docker's backend-specific security configuration (image digest, UID/GID, read-only mount and explicit write grants, no privilege or daemon socket binding, network default, cgroup resource limits, timeouts), run IDs, cancellation/reconciliation and integration tests, then expose a working Docker create card only when verified.
4. Integrate Daytona as first remote adapter after strong credential/cost/TTL and uncertain-outcome safeguards; do not auto-retry side-effecting commands or silently fail over to host.
5. Address the separate Go toolchain/dependency vulnerability scan failure and rerun `pnpm security:check` before any Phase 25 completion claim.

The new management pages display last *persisted* run state; `ready` does not mean a fresh backend health probe. Local Workspace `host` is high authority and **not isolated**; `restricted_host` is OS process isolation, **not** a VM/container. Profiles using a different Principal from the owning local user require explicit future delegation; do not loosen the ownership check as an incidental compatibility change.

### Workspace binding integrity hardening (2026-10-09)

- `bd39430` — Workspace Provider `ValidateConfig` now performs a **metadata-only** `SandboxRouter.Resolve` after strict ConfigVersion 2 parsing. If the Sandbox does not exist, Provider registration/update fails before activation and no Provider configuration is persisted. `ValidateConfig` never calls `Acquire`, probes a backend filesystem, resolves credentials, or starts a Sandbox; existing `Plugin.Open` and Catalog discovery stay side-effect-free.
- Added unit and Runtime integration tests for unknown SandboxID rejection and absence of persisted failed Provider configuration.
- `pnpm check` was re-run after the change: **exit code 0**, Go lint/test/race and contracts/TypeScript checks passed, Vitest **11 files / 72 tests** passed, and frontend production builds completed. One first-pass unrelated Skills-page 5-second test timeout passed both as an isolated re-run and during full re-run; document this as a potential flaky timing test, not a new Workspace regression.
- `pnpm security:check` is still **failing** (Go 1.27.1 standard-library advisory set and `golang.org/x/net` v0.58.0). This is not fixed by the Sandbox binding hardening commit. The Phase 25 milestone remains **In Progress**.
- Future lifecycle changes must block deleting a Sandbox while it is referenced by any Provider and must reconcile stale/unknown runs before permitting destruction. The current UI does not expose deletion or stop/start and should not imply they work.

## Cluster D0 — Durable leases and governed local lifecycle (2026-10-09)

Status: implementation scoped for the next segmented commits. This section is **the contract**, not a completion claim.

**Threat model and decision:** the existing Sandbox Router uses an in-memory `active` counter. Different CLI, desktop, and management processes may open the same Mender SQLite DB simultaneously, so in-memory counts alone cannot block `sandbox.stop/destroy` while a different process still holds a Workspace shell or file session. Before exposing lifecycle buttons, add a SQLite-backed **single active execution lease per Sandbox** reservation, uniquely keyed by SandboxID, with cryptographic token and pinned Run Generation/Principal/Scope. The reservation must atomically read the persisted Run's `ready` state and generation. Never auto-expire leases on restart: an uncertain process may still be running; orphaned leases block lifecycle changes and new executions until explicit reviewed recovery exists. A successful sandbox Session close is a prerequisite for releasing a durable lease. Any cleanup failure leaves a lease for human reconciliation (fail closed).

**Local Backend semantic contract:** Local Workspace manages a directory and a selected host/restricted-host process-isolation policy. Its `Stop` means make the Sandbox logically unavailable for new Workspace executions, **not** kill unrelated host processes or delete files. Its `Start` revalidates the pinned canonical root and the original isolation capability, then transitions `stopped -> provisioning -> ready` with a generation increment. Its `Destroy` tombstones the Sandbox Run (`ready/stopped -> destroying -> destroyed`) and retains resource metadata and audit history; it **must never delete the actual host workspace**. Existing/unknown/failed backends must reject control actions without switching to Host.

**Authority and consistency:** `sandbox.start`, `sandbox.stop`, `sandbox.destroy` require an explicit, audited `Principal+Scope` Policy/Approval decision before backend root checks or lifecycle mutations; local Sandbox owner must match. A state transition and a bounded lifecycle-event audit row commit in one SQLite transaction with generation compare-and-swap; any active durable lease yields a typed conflict. A destroy must fail when any Workspace Provider ConfigVersion 2 references the Sandbox; the Provider binding lookup and run transition are part of the same SQLite transaction, so no time-of-check/time-of-use split. The ordinary Provider registration path must continue to reject missing/destroyed Sandbox bindings. Do not remove or recreate Sandbox IDs by default.

**Post-cut API parity:** implement typed `POST /sandboxes/{sandboxId}/start`, `POST /sandboxes/{sandboxId}/stop`, `DELETE /sandboxes/{sandboxId}` (tombstone only) on HTTP, OpenAPI, TypeScript SDK, Wails/Desktop and CLI. The UI must only offer applicable actions, confirm irreversible tombstoning, explain that local files remain untouched, and show non-ready/destroyed state. All entry points call the same Runtime management methods. Do not label `host` as isolated and do not enable Docker/Daytona/E2B controls before those drivers implement the lifecycle.

**Acceptance tests:** separate runtimes against the same DB serialize Sandbox leases, refuse a concurrent lease and stop/destroy until release, preserve unresolved leases across restart, fence stale-generation acquisitions, ensure a policy-denied stop doesn't write a lifecycle row, ensure Provider-bound destroy is rejected, ensure unsupported backend attempts fail closed, verify repeated stop/start generation and full audit on restart, and verify HTTP/CLI/Desktop and UI behavior including error states.

**Commit boundaries:** (D0a) SQLite durable lease contract + test; (D0b) local lifecycle use case and transactional state/audit/reference validation; (D0c) typed management transports and UI; (D0d) evidence/verification. The existing project skills and `.agents/`, `.pi/`, `.intent-loop/` remain untouched.

## Cluster D0 checkpoint — durable leases and local lifecycle (2026-10-09)

**Implemented in local `main`, Phase 25 remains In Progress:**

- `c6b0ebb` — document-first durable lease / local lifecycle invariants and rollout order.
- `b149081` — SQLite schema v4 persisted, principal/scope/generation-fenced single-Sandbox leases across Runtime connections. On crash, an uncertain active lease is NOT automatically expired. Token-guarded release occurs only after successful Sandbox Session cleanup, and denied/stale/duplicate execution is fail-closed.
- `3d77a61` — SQLite schema v5 transactional local `start/stop/destroy` with CAS run generation and successful lifecycle events; admission via existing `sandbox.*` Policy/Approval/Audit. `Stop` blocks new execution but does not forcibly kill unrelated local processes; `Start` revalidates canonical root and pinned isolation, increments generation; `Destroy` only tombstones the run and never deletes host files. Cross-Runtime active leases and Provider references block transitions. SQLite triggers guard new Workspace Provider ConfigVersion 2 references to missing/destroyed Sandbox IDs atomically, including concurrent direct DB writers.
- `7df4eb3` — typed HTTP/OpenAPI/SDK/Wails/Desktop and CLI Start/Stop/Destroy, with explicit CLI `destroy --confirm`, UI lifecycle actions gated by state/active leases, confirmation and host-file-preservation messaging, and tests across transports. Docker/Daytona/E2B operations remain hidden/unsupported.

**Verification:** final `pnpm check` exit code 0 (provider SDK/proto drift, Go lint/test/race, TypeScript, i18n, Vitest and Web/Desktop builds); Vitest **12 files, 76 tests passed**; targeted Go race and integration tests include multi-connection persistence/restart, fencing, Policy denials, Provider-bound destroy rejection, host file retention, and HTTP/Wails/CLI state parity. The existing frontend >500kB bundle warnings remain. `pnpm security:check` is still a separate known blocker (Go 1.27.1 standard-library advisories and golang.org/x/net v0.58.0); do not claim green before re-running on patched toolchain/dependencies.

**Important limitations:** a leaked durable lease deliberately blocks all new executions until reviewed recovery; the present UI displays active lease count but does not offer arbitrary force-unlock. Local `host` is **not** isolated, and local `restricted_host` is not a VM/container. A stopped Local Workspace is a logical unavailable state, not a managed compute process kill. Destroyed Sandbox rows remain as audit tombstones. Lifecycle event history is currently persisted and testable, but a dedicated Activity detail tab with filtering is not yet shipped. Strong cross-Principal sharing and recovery remain explicitly out of scope until designed.

## Cluster D1 — Docker execution backend, document-first security gate (2026-10-09)

Status: design/probe scope, NOT registered or visible as a usable backend. On this developer machine, Docker Desktop Engine **29.8.0** responded, using **cgroup v2** and default seccomp. Active Docker context: `desktop-linux` via a user-local UNIX socket. This is environment-specific readiness, **not** a security clearance or proof of rootless mode; current `docker info` does not advertise rootless. Only database/postgres/Redis/telemetry images are cached here, so no trusted general-purpose Workspace image is assumed or auto-pulled.

### Security invariants (hard fail, no fallback)

1. Docker Engine is a high-authority host-owned service, not an untrusted Provider plugin or Tool argument. Mender must require an explicitly configured local UNIX socket with strict path/symlink checks, forbid unauthenticated remote TCP and reject any user/model-supplied Docker daemon path, raw daemon options or socket mounts. Prefer the modern supported Go SDK modules `github.com/moby/moby/client` and `github.com/moby/moby/api`; **do not** use deprecated `github.com/docker/docker` for Engine 29+.
2. Image must be an immutable, locally verified `repository@sha256:<64 lowercase hex>` digest, with a fixed non-root numeric UID/GID. No `latest` tag, implicit registry pull, arbitrary Dockerfile/build, privileged container, host PID/user/network namespace, host bind mount, Docker daemon socket inside Sandbox, port publishing or added capabilities.
3. Trusted creation fixes `ReadonlyRootfs=true`, `CapDrop=ALL`, `SecurityOpt=no-new-privileges`, default seccomp, no network egress (`none`), no host mounts, bounded CPU/memory/PIDs and bounded writable scratch/workspace volume. If enforceable disk limits are unavailable, **do not** claim an enforced disk budget. Validate actual Engine/kernel capabilities and effective HostConfig on Inspect; reject unmet hard requirements, never soften limits.
4. File operations use the **same running container filesystem** as Shell, via Docker Engine `archive` transfer/read plus bounded validated TAR parsing, strict containment/symlink checks and atomic write semantics; do not emulate generic read/search/edit by host path access or by interpolated shell commands. Docker `exec` applies a fixed interpreter and command argv only inside the validated container, under bounded time/output/cancel, no privileged exec. If four Workspace tools cannot be implemented with equivalent safety, backend stays unavailable.
5. Run lifecycle uses persisted Engine container identity, labels and generation, explicit policy/provisioning admission, and conservative reconciliation after crashes/cancellation/timeouts. Never auto-retry potentially side-effecting commands or recreate an unconfirmed container. Stop/Destroy must verify no live durable lease and no Provider references; cleanup errors retain `unknown/orphaned` evidence for operator review.
6. Keep the Docker backend disabled in catalog/create UI until an end-to-end integration matrix proves read/search/edit/shell parity plus start/stop/destroy, resource/no-egress/privilege restrictions, cancellation, leaked resources, restart reconciliation, and generated HTTP/SDK/Wails/Desktop parity.

**D1 segmented implementation:**

- **D1a**: pure, strict local Engine endpoint and immutable Docker template validation (no Docker SDK/daemon mutations; unit/race tests). No backend registration.
- **D1b**: modern Moby Go SDK Engine capability probe, concrete typed container creation and persisted run reconciliation (no UI exposure until safe).
- **D1c**: bounded archive-backed files/search/edit plus non-privileged in-container Exec, real daemon integration tests with a pinned trusted test image and cleanup.
- **D1d**: admitted create/lifecycle use cases, resource/cost governance, UI/backend selection parity and security/test closeout. Remain fail-closed if Engine or test image is unavailable.

Official references: [Docker Engine Security](https://docs.docker.com/engine/security/), [Protect the Daemon Socket](https://docs.docker.com/engine/security/protect-access/), [Engine API v1.51](https://docs.docker.com/reference/api/engine/version/v1.51/), [Resource Constraints](https://docs.docker.com/engine/containers/resource_constraints), [Moby supported public Go SDK modules](https://github.com/moby/moby#go-modules).

### D1a implementation checkpoint — strict Docker inputs (2026-10-09)

Commit `eec811a` (`feat(sandbox): validate secure local Docker backend inputs`) establishes the first **non-operational** Docker security-contract slice under `backend/internal/sandbox/infrastructure/dockerengine`:

- `ParseEngineEndpoint` accepts only explicit canonical `unix:///absolute/socket` addresses, never Docker contexts, SSH/TCP/HTTP, encoded path aliases, query strings, or Tool-selected daemon references. `PreflightSocket` requires an actual local UNIX socket and rejects path symlink components, group/world writable ancestors and group/world writable sockets. It is **a local path preflight, not a race-free daemon socket authentication or an Engine capability proof**.
- `Template.Validate` requires lowercase fully qualified immutable `registry/team/image@sha256:<64 lowercase hex>` digests, a fixed numeric non-root UID/GID range, bounded CPU/memory/workspace scratch and PIDs, and a scratch limit not exceeding the memory budget. This validation deliberately rejects mutable image tags and container privilege flags; image presence/digest verification and effective Engine enforcement still belong to D1b/D1c.
- A fixed `SecurityRequirements` projection expresses read-only root FS, network mode `none`, dropped Linux capabilities, `no-new-privileges`, no host mounts/port publishing/privileged mode, and `/workspace` as the only intended writable workspace. A future Docker adapter **must verify** these effective settings against Docker's observed HostConfig; this pure struct does not enforce Docker runtime isolation yet.
- Unit and UNIX-socket tests cover allowed and denied endpoints, socket symlink/ordinary file/writable-parent rejection, mutable and unqualified images, root/system user IDs and invalid resource budgets. Docker daemon has not been contacted by code in this D1a slice, and no image was pulled, container created, Docker Go SDK dependency added or UI backend enabled.

Verification: `go test -race ./backend/internal/sandbox/infrastructure/dockerengine -count=1` passed; `golangci-lint run ./backend/internal/sandbox/infrastructure/dockerengine/...` reported 0 issues. Full `pnpm check` exit code **0** after the D1a commit, with Go lint/test/race and contract parity, Vitest **12 files / 76 tests**, and Web/Desktop production builds. Large JS bundles remain a warning. This does **not** clear the independent `pnpm security:check` blocker (Go 1.27.1 and `x/net` v0.58.0), which was already observed in prior scans.

Next substantive implementation: **D1b** modern Moby Go SDK, explicit read-only image/Engine capability probe and observed security configuration; only then implement bounded container provisioning and persistent run recovery. Docker must remain unavailable for user creation/Workspace Provider binding until all four tool capabilities, cancellation, cleanup and no-fallback end-to-end integration are proven.

## Cluster D1b — Modern Moby SDK, verified Engine planning and crash-safe provisioning (2026-10-09)

Status: **Implementation contract; do not represent D1b as completed until all tests and commits land.** The supported external API is `github.com/moby/moby/client` **v0.6.2** and `github.com/moby/moby/api` **v1.56.1**, whose public modules are independently versioned and Go 1.24+ compatible. Verified against Docker Desktop 29.8.0 / Engine API 1.56; version values are developer-host facts, not deployment requirements. The deprecated `github.com/docker/docker` SDK must not be imported. Explicit `client.New(client.WithHost(endpoint.URL()))` is required; do **not** use `client.FromEnv`, Docker contexts, or alternate hosts. Errors, secrets and URI details must be bounded/redacted in user-facing contracts.

**D1b1: Probe and immutable image availability (read-only).** Engine calls are limited to Ping, Info, ImageInspect and ContainerInspect/ContainerList scoped to Mender labels. Probe must fail closed for missing UNIX socket, non-Linux daemon, lack of CPU/Memory/PID limit support, absent seccomp or missing cgroup version, unsupported API, and image Digest not present locally. Never `ImagePull` or build an image. A local image inspect by immutable reference must match the exact `RepoDigests` and be Linux-based; network access is denied by default, regardless of daemon network settings. The path check currently implemented in D1a is not race-free socket authentication, so D1b must not overstate transport guarantees.

**D1b2: Typed no-privilege container plan.** Template/ImageDigest is a trusted *management-side* input and never a Tool argument. `ContainerCreateOptions` use fixed `Config` and `HostConfig`: pinned digest, numeric non-root user, `/workspace` working dir, fixed long-running command with no Tool-derived executable, no inherited host environment, Mender SandboxID/run generation labels, read-only image root, tmpfs `/workspace` with bounded size/UID/GID and no host bind/volume/daemon socket, no network or published ports, capabilities dropped, `no-new-privileges`, bounded CPU/Memory/PIDs, no restart policy, and resource limits validated against Engine observed support. Reject image Entrypoint/volume/healthcheck surprises if overrides cannot be safely enforced. Inspect response must match immutable plan including labels, User, ImageID/digest, rootfs, network, mounts, effective resource limits and running/stopped status; a mismatch is an explicit governance failure, never an automatic host fallback.

**D1b3: Durable run reservation and reconciliation.** Before a chargeable/side-effecting `ContainerCreate`, require a Principal-attributed `sandbox.provision` admission and persist the Sandbox + `provisioning` run with a deterministic resource name from `(SandboxID, generation)`; only then call Engine. Network/HTTP/cancellation failure after an attempted create marks the run `unknown` without automatically retrying `ContainerCreate`. Reconciliation searches only the exact, Mender-labeled container; a missing/mismatched container is left `unknown/orphaned` for review, not silently recreated or removed. Store the actual external container ID and generation before reporting `ready`. No unsupervised reaping of existing daemon resources; cleanup must check matching ownership labels and admit the lifecycle action.

**D1b implementation segments and evidence:**

1. Commit Docker Go SDK and read-only Engine probe with fake API tests, including no environment overrides, deny-on-missing daemon capabilities, and real Engine read-only smoke probe. No image pull and no container creation as part of this segment.
2. Commit strictly constructed container options and inspection verification with fake daemon tests. An image reference mismatch or weakened effective HostConfig fails closed.
3. Commit durable Sandbox run staging and explicit uncertain-outcome reconciliation, with test-only fake Engine injection. Do not register Docker as a functional Workspace backend until D1c implements all four operations on the same container filesystem and D1d wires admitted management/UI flows.

**Deployment caveat:** at present no purpose-built pinned Workspace image is cached. A read-only daemon probe can be exercised safely, but real provisioning/in-container Workspace Tool integration is intentionally gated on a reviewed immutable image and end-to-end resource cleanup test; never auto-pull an unrelated database/Redis image as a substitute.

### D1b implementation checkpoint — Moby SDK, immutable container plan and durable uncertain-outcome handling (2026-10-09)

**Status:** D1b internal execution-control foundations are implemented and tested, but Docker is **NOT registered as a usable Sandbox backend**, no Docker UI card was enabled, and no live container was created by this phase. D1c must still prove file/exec parity using a reviewed pinned image before any end-user release.

**Segmented commits:**

- `2c7eaca` — documentation-first D1b policy, immutable image, daemon connection, in-container limits, idempotent naming, and no-auto-retry/reconciliation contracts.
- `0e7302f` — adopt officially supported `github.com/moby/moby/client@v0.6.2` and `github.com/moby/moby/api@v1.56.1` in the Go backend, updating separately built Wails Desktop module dependencies as required by isolated `GOWORK=off` module validation. No deprecated `github.com/docker/docker` import. Add an **SDK read-only** Inspector, explicit `unix://` socket host (never `client.FromEnv`), Engine Ping/Info capability checks (Linux, Engine API >=1.51, cgroup v2, builtin seccomp, CPU/memory/PID hard-limit support), and local immutable ImageInspect requiring exact cached `RepoDigests`/Linux/ImageID/architecture. SDK errors are sanitized. A real read-only smoke against Docker Desktop 29.8.0 / API 1.56 succeeded despite a malicious test `DOCKER_HOST` override. No image pull, container create/start, or daemon resource changes.
- `0ab34cd` — immutable, generated `ContainerCreateOptions` with fixed numeric non-root user, immutable verified image reference, fixed long-running command, disabled image healthcheck, limited environment, no network/ports, readonly root, all capability drops, no-new-privileges, private namespaces, bounded memory/swap/CPU/PIDs, memory-backed `/workspace` and `/tmp` with bounded tmpfs, no host binds, privileged daemon socket access, restart policy or Docker log driver. `VerifyInspect` rejects deviations in ID/name/labels/image/config/HostConfig/mounts/resource limits or unexpected state; tests mutate privileges, namespace, mounts, network, resources, user, healthcheck and labels and confirm fail-closed behavior. **This is a reviewed plan, not proof that a real daemon has enforced its options.**
- `a8b6f7e` — non-registered internal Moby Provisioner with required `sandbox.provision` Principal/Policy/Approval/Audit gate, immutable TemplateLookup and persisted Sandbox Run CAS; records `provisioning` before the first Docker create call, then checks returned container identity and effective inspected security configuration before reporting `ready`. A failed/ambiguous Create/Start/Inspect leaves the run in `unknown` with any known external container ID (even after caller cancellation via bounded cleanup context); subsequent `Provision` rejects unknown states rather than retrying. `Reconcile` ONLY inspects an exact deterministic container name, checks immutable labels/template/image/previously recorded external ID and moves an already-running instance to `ready` without creating/starting/deleting it. Schema v6 adds an immutable SQLite Docker template store (digest, UID/GID, CPU/Memory/Workspace/PIDs; no socket/secret); direct SQL UPDATE/DELETE is denied by triggers. Tests inject a fake daemon to simulate lost responses, mismatched HostConfig, stale IDs, pre-admission denial, full restart with reopened run/template SQLite stores, and verify zero unexpected mutating calls.

**Verification:** the final full `pnpm check` returned **exit code 0** after D1b3; Go package/race/lint and Module Isolation checks passed, plus typecheck, i18n, contracts/SDK/Wails drift checks, and Web/Desktop builds. Vitest: **12 files, 76 passing tests**. Targeted Docker D1b tests and `golangci-lint run ./backend/internal/sandbox/...` also passed. Real daemon smoke read only: Docker 29.8.0, API 1.56, Linux cgroup v2, builtin seccomp and resource-limit support confirmed. No Docker images were pulled and no test containers were created.

**Security blocker:** re-run of `pnpm security:check` returned exit **1**: twelve reachable advisories in the installed Go 1.27.1 standard library (fix in Go 1.27.2); Go dependency metadata also identifies `golang.org/x/net@v0.58.0` findings fixed by v0.60.0, with one imported-package and two module-only additional findings reported as not reachable. The new Moby SDK is not being represented as a security clearance; do not mark Phase 25 complete until the scan is re-run on remediated toolchain/dependencies.

**Next gate D1c:** use a deliberately reviewed immutable, locally available general-purpose execution image (no auto-pull of Redis/Postgres or random image); test real Engine ContainerCreate/Start/Inspect with exact resource settings, labels, healthcheck, network, tmpfs and mounts on the supported Docker Desktop/Linux hosts. Build bounded tar-based read/search/edit and in-container shell execution against the *same* container filesystem. Verify path/symlink containment, atomic edits, stdout/stderr/cancellation, no network, CPU/memory/PID/scratch enforcement, timeout unknown outcomes, cleanup, restart reconciliation and orphan review. D1d then wires admitted user-facing Docker creation/lifecycle into HTTP/CLI/Wails/UI only after all tests and a secure image are available.

**Known limitations:** the internal Provisioner requires a previously created Docker Sandbox `defined` Run and reviewed immutable Template; user-facing Docker provisioning is intentionally not wired. The D1a UNIX socket path preflight remains non-race-free and should not be presented as high-assurance daemon authentication. Docker is a container boundary, not a microVM; strong tenant isolation claims require a separate threat-model review.

## Phase 25 — Stage D final closure contract (2026-10-09)

**Scope frozen; no D1e/D2/D100Z subdivisions.** This stage is ONE bounded delivery: complete Docker Sandbox local execution or document concrete blockers and stop rather than extending the numbered stage. The scope is limited to (a) Docker Engine configuration and admission, (b) deterministic secure container lifecycle, (c) Workspace read/search/edit/shell on one container filesystem, (d) UI/API/SDK/Wails/CLI parity, and (e) verification and stage status. Remote Daytona/E2B is Stage E only. Do not build a new agent loop, image marketplace, container orchestrator, or provider compatibility bridge.

**Integrated minimal Docker contract:** a management-authorized immutable locally cached `registry/namespace/image@sha256:<digest>` is required. Engine socket must come from explicit host-owned `MENDER_DOCKER_SOCKET=unix:///absolute/socket`, never Tool arguments, Docker context or `DOCKER_HOST`; missing/untrusted socket keeps Docker unavailable. Creating a Docker Sandbox admits `sandbox.provision` first, persists one stable immutable template, stages a `defined` run and invokes the existing D1b no-retry Provisioner; failed/uncertain responses stay unknown for operator reconciliation. Docker is selected only through the ordinary Workspace Provider's existing Sandbox picker after all four capabilities are tested. Host/restricted-host isolation remains separate.

**File-operation safety:** source and destination paths are relative to `/workspace` (one isolated memory-backed tmpfs). Reject NUL, `..`, absolute paths, symlinks, non-directory parents, unsupported tar entries, oversized files and traversal. Use a fixed, bounded, non-root in-container Exec stream protocol for file reads, directory listing and writes (Engine archive APIs cannot access writable tmpfs under the required readonly rootfs), never access host Workspace file paths. For atomic writes, stage a bounded tar file and use fixed in-container non-privileged command argv to link/rename into place; untrusted user text is never interpolated into the staging command itself. Sandbox shell intentionally passes command input only to a fixed in-container `/bin/sh -c` with explicit UID and CWD. Timeouts/ambiguous errors retain a durable lease, not false success or automatic retry; an explicit error is surfaced. Backend supports no network, host mounts, daemon socket, privileged exec or port publishing.

**Lifecycle:** provision, start, stop, destroy require dedicated Policy/Principal/Owner checks, verified external ID/labels/template, cross-Runtime lease check and conservative CAS state transition. Stop/destroy are NOT callable while a Workspace Provider references the Sandbox (destroy) or a lease is active. Destroy removes only the sandbox container and tmpfs, not local host files; remote resources or other containers are not touched. Missing/uncertain resources remain inspectable as unknown/orphaned.

**Acceptance before marking Stage D complete:** (1) a reviewed pinned test image available by digest, never pulled implicitly during normal Runtime execution; (2) live Docker Engine tests exercise create/read/search/edit/shell/stop/start/destroy and attest isolation, network and CPU/memory/PID/tmpfs policy; (3) no host fallbacks or Tool-controlled placement; (4) denied admission before container or filesystem side effects; (5) HTTP/OpenAPI/SDK/Wails/Web/Desktop and CLI parity, i18n and smoke tests; (6) full `pnpm check` green and security gate status stated truthfully. If security scanner stays red due Go toolchain, Phase 25 release remains blocked while Stage D implementation can be declared functionally complete with that explicit qualification.

**Commit policy:** docs first, cohesive backend commit, cohesive client/UI commit, final verification/doc commit. No more stage-letter suffixes introduced in roadmap/ADR. Keep `.agents/`, `.pi/`, `.intent-loop/` untouched/uncommitted.

**Stage D implementation finding — Docker readonly filesystem:** a real Engine 29.8.0 run with Alpine 3.22, pinned by its verified immutable digest, confirmed `ReadonlyRootfs=true`, non-root uid=10001, and `NoNewPrivs=1`. Docker's `CopyToContainer` REST archive endpoint refuses writing to this container with `container rootfs is marked read-only` even for its bounded writable `/workspace` tmpfs. Keep rootfs readonly as a hard requirement. Both read/search and edit use bounded non-privileged in-container Exec streams to the tmpfs; write/edit stage file content over ExecAttach stdin with a declared length and post-transfer digest verification, followed by fixed argv-only atomic link/rename. The file payload is never interpolated into shell commands or routed through host file operations. This is a deliberate correction to the earlier archive-only write assumption; do not weaken isolation or add a fallback to host execution.

**Engine tmpfs correction confirmed on real Docker Desktop (2026-10-09):** not only `PUT /containers/{id}/archive` but also `GET/HEAD /containers/{id}/archive?path=/workspace/...` fails to access a verified existing ordinary file in the writable container tmpfs (GET reports nonexistent; HEAD returns 404 route). Therefore Stage D structured file operations must use a fixed, bounded, non-root **container-local process protocol for both reads and writes**, not Docker Archive APIs for files on tmpfs. Paths are verified component-by-component inside the container, no tool-supplied shell script is constructed for read/search/edit, and data is streamed only over attached Docker Exec stdio with byte limits. This correction keeps read-only rootfs, no network, no host bind mounts, and no host fallback intact. An integrated deterministic test must cover file create/read/search/replace/delete and shell equivalence under these exact security settings before Docker is user-visible.

## Stage D — Functionally Complete (2026-10-09)

**Final outcome:** Stage D has been CLOSED as one bounded local Docker Sandbox delivery; **no further D subdivisions are planned**. Phase 25 as a whole stays **In Progress** because the explicitly separate Stage E remote integration remains pending and the repository-wide security release gate is still red. Older D1a/D1b/D1c/D1d checkpoint sections above are historical implementation records, superseded by this final status.

### Shipped functionality

- Docker Engine is an opt-in, high-authority host-owned backend, activated only when the Runtime has an explicit `MENDER_DOCKER_SOCKET=unix:///canonical/local/socket` and its Engine passes the Linux/cgroup-v2/seccomp/CPU/memory/PID probe. No `DOCKER_HOST` fallback, uncontrolled image pull or privileged daemon socket exposed to an Agent or Provider Tool.
- Governed `sandbox.provision/acquire/start/stop/destroy` use the existing Principal, Policy, Approval, immutable Docker Template, durable audits, generation CAS and SQLite cross-Runtime lease reservation. A denied/uncertain operation does not fall back to local host execution or automatically recreate a container.
- A Docker Sandbox is pinned to an immutable, *already cached* OCI image digest and fixed non-root execution identity. Containers have a read-only root filesystem, no network, ports or host binds, no added capabilities, `no-new-privileges`, private namespaces, and a bounded memory-backed `/workspace` plus CPU/memory/PID resource limits. Run generation, image identity, labels and observed effective Docker settings are revalidated before execution.
- `workspace.read/search/edit/shell` now execute in **one and the same Docker container filesystem**, preserving existing Tool SourceKeys, Workspace Provider ConfigVersion 2 and Profile tool bindings. Structured operations use fixed, path-validated, size-bounded non-root in-container command/stream protocols. User-supplied shell commands are executed only inside the Docker container, never via a host shell; no Tool argument may supply a Docker daemon URL, image, mount or host execution fallback.
- Stop, Start and Destroy perform admitted, lease- and Provider-reference-fenced transitions. Docker Start creates a new Run generation and a fresh ephemeral `/workspace` (its old scratch data is not preserved); Destroy removes only the verified Mender-labeled container and leaves host files untouched. Missing/uncertain Engine outcomes remain reviewable, and `unknown` recovery inspects the exact persisted Mender identity without creating or restarting resources.
- HTTP (`/api/v1/sandboxes` and Docker create/lifecycle/reconcile), OpenAPI, TypeScript SDK, Wails/Desktop bindings, Web UI and CLI have parity. The UI exposes Docker only when the current Runtime's Engine passes the host-owned readiness probe, lets users choose an immutable image and bounded limits, and then bind a ready Sandbox from the normal Workspace Provider selector rather than entering opaque IDs. A Docker create whose durable state exists but Engine result is uncertain returns HTTP `202` with observed status rather than concealing the resource or attempting duplicate creation.

### Final acceptance evidence

- Real Docker Desktop Engine **29.8.0**, API **1.56**, Linux/arm64, cgroup v2, built-in seccomp. Opt-in test image `docker.io/library/alpine@sha256:5291449c3df73caf6ed85e649dec1b9e818b39a5d8c871e97afc13e9cd5e8fa8` (Alpine 3.22) was explicitly fetched for the test host, and not pulled by Mender Runtime. It is an integration fixture, not a universal recommended image or security attestation for arbitrary agents.
- All FOUR **real Docker `go test -race` flows passed**, executed sequentially: (1) Docker Backend read/search/edit/shell + isolation + lifecycle; (2) shared Runtime/Workspace Provider/Policy/lease pipeline; (3) real HTTP Docker management creation, Stop, Start, Destroy and persistent status; (4) real CLI Docker creation, Stop, Start and Destroy across separately opened Runtime sessions. No Mender test containers remained after cleanup.
- Final `pnpm check` **exit code 0**, covering Go package tests/Race Test/Lint, Go module isolation (including `GOWORK=off` desktop module), generated OpenAPI SDK/Wails bindings, TypeScript/ESLint/i18n, **13 Vitest files / 79 tests passing**, and Web/Desktop production builds. Existing >500kB JS bundle warnings are non-fatal and outside this frozen stage.
- Fail-closed tests also cover no trusted Engine endpoint (HTTP/Desktop creation unavailable), denied/unknown Sandbox and Provider bindings, immutable template/update constraints, Docker creation with unknown outcome, idempotent read-only reconciliation, wrong labels/images/namespace/network/UID/resource limits, and active leases/Provider references blocking destructive changes.
- **Independent release security scan remains FAILED**: `pnpm security:check` returned exit **1** due to 12 reachable Go 1.27.1 standard-library advisories (fix in Go 1.27.2). Dependency scanning also reports `golang.org/x/net@v0.58.0` findings (fix in v0.60.0) with separate reachability distinctions. This is an explicit **repository release blocker**, not evidence that Stage D feature/tests failed; a later security maintenance change must upgrade and rescan before an unqualified release claim.

### Boundaries that remain outside Stage D

- Docker UNIX-socket filesystem preflight does not provide race-free socket authentication. Linux container isolation is not a microVM or untrusted multi-tenant isolation guarantee. Resource-limits are verified through Engine capabilities and observed HostConfig; destructive load/stress enforcement is not claimed as performed. Durable unknown/uncertain leases deliberately require operator review rather than force-unlock or auto-retry.
- Remote Daytona/E2B integration belongs exclusively to **Stage E**, not another Stage D suffix. No generic Agent loop, workflow scheduler, image marketplace, implicit compatibility bridge or remote failover has been introduced.
- Repository `.agents/`, `.pi/`, `.intent-loop/` skill and agent state directories were not edited or committed. All work remains on local `main`, **not pushed**.

**Final Stage D commits:** `e0f0d80` (frozen contract); `29fc7cf`, `87b9fc2` (readonly/tmpfs corrections); `6122378` (all four Docker Workspace tools); `96480eb` (governed Docker Runtime/CLI/HTTP/Desktop services); `c155b4f` (typed SDK/Wails/Web UI parity); this governance closeout (documentation).

## Stage E — Daytona remote Sandbox (single bounded delivery, 2026-10-09)

Status: **Implementation delivered (experimental/quarantined); live vendor acceptance deferred outside Phase 25.** This is a historical design checkpoint; the final support decision below supersedes its original completion gate. Stage E has exactly one implementation target: Daytona. E2B and other cloud adapters are deferred and do not justify E1a/E2/E100-style phases. Completion requires a true Daytona credential-backed remote integration test; a fake SDK passing does **not** imply remotely tested or a completed Phase 25.

Current provider reference: [official Daytona Go SDK 0.223 documentation](https://www.daytona.io/docs/en/go-sdk/), module `github.com/daytona/clients/sdk-go@v0.223.0`. The SDK supports `daytona.NewClientWithConfig`, `Client.Create/Get/List/Close`, `Sandbox.Start/Stop/DeleteAndWait`, `FileSystem.DownloadFile/UploadFile/MoveFiles/GetFileInfo/ListFiles`, `Process.ExecuteCommand`, and `types.ImageParams` with CPU/Memory/Disk bounds, network block and wall-clock `TtlMinutes`. This is a managed third-party service with an external billing account, not a locally enforceable cost meter. As of kickoff, **no Daytona credential is available in the developer environment**; never synthesize availability, billable calls or a passing live test.

### Security/ownership invariants

1. Keep all existing ToolIDs/SourceKeys and the canonical `tool.invoke -> sandbox.acquire` pipeline. Trusted Sandbox metadata, not Tool JSON, selects a Daytona run. The backend must be **fail-closed** if the template/credential/remote run is absent, expired, mismatched or unknown; no fallback to Docker/Host.
2. The `daytona` backend has only `remote` isolation class. A per-Sandbox immutable template stores a **Mender Credential Reference**, a canonical Daytona region/host (allowlisted HTTPS), a fixed immutable `registry/namespace/image@sha256:<64 hex>` reference, bounded CPU/RAM/Disk, **server-side wall-clock TTL** (15–120 minutes), `NetworkBlockAll=true`, and first-party labels for SandboxID/Run Generation. Never store plaintext keys in SQLite, URLs, Tool args, Audit or client metadata. Resolve the Credential Reference *after* Policy/Approval admission immediately before remote operations and discard secret bytes promptly. Construct the SDK explicitly; never use SDK `NewClient()` or implicit `DAYTONA_*` API URL/image inputs. Protect against unsafe toolbox-host redirects and SSRF.
3. Bound local allocation attempts to at most **two live/defined Daytona Sandbox resources per Principal** in a SQLite transaction. This is a conservative Mender allocation guard, **not** a provider billing ceiling; owner controls remain subject to Daytona subscription charges. Do not claim a USD spending cap unless provider billing APIs can enforce one. Require API provider-enforced TTL, reject disabled TTL and network egress and do not expose public previews/linked sandboxes/volumes/GPU/KVM.
4. Admit `sandbox.provision` before remote create and persist `defined -> provisioning` with a Mender deterministic name and labels. On network errors/timeouts after Create, retain `unknown` and do NOT automatically retry. Reconcile *read-only* using server-issued ID or exact Mender labels, verify Owner/region/image/resource bounds, and move `unknown -> ready` only when truly observed running. Credential/permission errors stay non-secret, bounded and auditable.
5. Four Workspace capabilities must use the **same observed Daytona SDK Sandbox instance and fixed service-provided working directory**. Structured read/search/edit validates relative paths and all ancestor symlink checks, bounds bytes/files/output, and implements create-only/replace with staging + exact-count/atomic moves; the explicit `shell` command runs only in the remote Sandbox (never host). The SDK's toolbox file metadata has limited symlink details: do not claim race-free multi-tenant path confinement or stronger isolation than the remote service provides; fail closed if pathname type cannot be validated.
6. Lifecycle Start/Stop/Destroy is admitted and fenced by durable execution lease, Provider-reference check and Run Generation. Remote failures remain `unknown`, never optimistic `ready/destroyed`. Destroy must prevent new lease and never delete a local host root. SDK client connections are closed when use completes; no unbounded resident socket pool.

### Final acceptance for Stage E

- Single bounded set of commits: **specification**, **remote backend + immutable persistence/tests**, **Runtime/CLI/HTTP/OpenAPI/SDK/Wails/Web UI parity/tests**, **verification/closeout**. No extra stage numbering.
- Unit/fake-SDK tests prove Principal/Policy/Credential ordering, exact labels/region/TTL/network/capacity inputs, path/symlink/size bounds, shell/file parity, no create replay on uncertain outcome, lease/reference conflicts, idempotent reconciliation, and no unintended host invocation.
- A live credential-backed Daytona integration must create a short-TTL sandbox, exercise read/search/edit/shell, stop/start/destroy, and verify observed provider state/cleanup and costs using a dedicated reviewed image/snapshot. The live test is opt-in and never silently spends money.
- All repository Go/TS/contract/Wails/i18n checks and `pnpm check` pass. The existing Go 1.27.1/x/net security scan blocker is a separate release gate; record it accurately and do not recursively extend Stage E to fix unrelated issues.
- Only when the above are evidenced, update Stage E and Phase 25 status to Complete. No Agent loop, no remote auto-retry, no E2B in this stage. `.agents/`, `.pi/`, `.intent-loop/` remain untouched.

### Stage E — Real Daytona credential check and fail-closed provisioning evidence (2026-10-09)

**Historical acceptance checkpoint: real remote integration NOT accepted.** An API Key was made available **ephemerally by the user** for this bounded verification attempt; its plaintext was never added to repository source, tests, docs, Git commits or persisted Mender Credential secrets.

- **Real SDK read-only authentication passed.** Daytona Go SDK v0.223.0, with an explicit allowlisted HTTPS vendor endpoint and ephemeral environment Credential Resolver, successfully performed an authorized label-filtered inventory query.
- **One real chargeable provisioning attempt did not succeed.** A resource with CPU=1, RAM=2 GiB, disk=3 GiB and service-side TTL=15 minutes, targeting an immutable Alpine image digest and outbound network blocking, was attempted. The Daytona SDK returned a remote error; Mender preserved the durable result as `unknown` without automatically attempting another Create. Because the initial production error mapper intentionally stripped vendor responses, the actual Daytona HTTP status/code and whether remote allocation occurred **were not observable** from the first attempt. Therefore **do not** describe remote Create, four Workspace tools or Start/Stop/Destroy as live-tested or working on Daytona.
- Best-effort exact-label cleanup was attempted inside the opt-in test but did not provide independent evidence that no remote resource remained. A subsequent safe read-only recheck could not complete; the user was explicitly instructed to check and remove the test resource by its deterministic `mender-sbx-...-g1` name in their Daytona console, **then revoke the supplied API Key**. The tool was not asked to keep using the secret after this point. Daytona-side cleanup remains **unverified** until the user confirms it.
- Commit `7af6d90`: add an opt-in read-only/live Runtime test harness gated by explicit environment variables (without any credential literals), typed sanitized Daytona SDK HTTP status and machine-code diagnostics that cannot leak provider message bodies or Authorization headers, and read-only `unknown` reconciliation by deterministic Mender name **only after validating exact ownership labels, generation, TTL, network and resources**. A wrong-label resource remains rejected and never triggers mutating recovery calls.
- Local evidence after the hardening: Daytona/backend focused Go Race Test passed, the full `pnpm check` **exit code 0** (Go lint/test/race/module isolation, generated SDK/Wails, TS/i18n and Web/Desktop builds), **14 Vitest files / 82 tests passed**. Repo scans found **no literal Daytona API Key pattern** in tracked code or added test files. This evidence is distinct from the failed live provisioning check.
- The independent Go toolchain and dependency `pnpm security:check` release blocker from previous Phase 25 checkpoints has **not** been represented as fixed in this update.

**Only remaining Stage E acceptance gate:** with a newly issued time-limited account credential and the service-side error identified through sanitized numeric status diagnostics, obtain one verified provider-created and provider-deleted Daytona Sandbox, demonstrate real read/search/edit/shell through the bound Workspace Provider, and verify remote Start/Stop/Destroy, observed limits and clean final inventory. Do **not** add further E subdivisions, automate retries of unknown Creates, or mark Phase 25 Complete while this gate is unsatisfied.

## Phase 25 stabilization — Sandbox Activity and security release gate (2026-10-09)

Status: **Implementation contract**, not a claim Stage E has passed real Daytona testing. Stage D stays closed; do not introduce new Stage D/E suffixes. No remote billable calls are permitted with the already revoked Daytona API key.

**Sandbox Activity:** the existing durable `sandbox_admissions` and `sandbox_lifecycle_events` SQLite records must power one bounded, typed read-only Activity view per Sandbox. No duplicate audit writes, no new Audit/Policy DSL, no daemon/credential side effects and no new compatibility bridge. Server returns the most recent 100 events at most, merging admission attempts (admitted/denied/failed) and **successful** lifecycle transitions in descending actual timestamp order; use one `SandboxActivity` response and identical HTTP/OpenAPI/SDK/Wails typed contracts. Project only action, event type, outcome/status, non-secret policy decision, principal *kind*, origin, generation, prior/next state and timestamp. Do **not** render PrincipalRef, reason strings, tool arguments, remote URLs, API keys or provider error bodies. A missing Sandbox yields 404; future/invalid limit fails validation. A destroyed Sandbox retains its activity history. UI gets a dedicated `/sandboxes/:sandboxId/activity` route, navigated from detail and linking back, with pagination, clear empty/failure states and bilingual wording. Unit/HTTP/Desktop/React tests must assert ordering, denial visibility, bounded pages and absence of sensitive fields.

**Independent security maintenance:** evaluate upgrading the installed Go toolchain from 1.27.1 to the patched release and upgrade affected `golang.org/x/net` dependency to the documented fixed range across independently buildable Backend/Desktop modules. Run `pnpm security:check` as a **separate** release gate and full `pnpm check`; never imply a passing feature test is equivalent to passing vulnerability scanning. If toolchain or dependency installation is blocked, retain explicit failure evidence and avoid unrelated major upgrades. Do not change Daytona credentials or contact the live remote service.

Implementation commits must be segmented: architecture acceptance, Activity API/typed clients, Activity UI/tests, security maintenance only if tested, and factual verification record. Protect `.agents/`, `.pi/`, `.intent-loop/` and do not push without an explicit instruction.

### Stabilization checkpoint — Activity delivered and security gate green (2026-10-09)

**Feature status:** Sandbox Activity is shipped. The dedicated `/sandboxes/:sandboxId/activity` read-only page and `GET /api/v1/sandboxes/{sandboxId}/activity?limit=1..100` are backed by existing SQLite `sandbox_admissions` and `sandbox_lifecycle_events`. A single bounded response merges admitted/denied/failed Policy operations with **completed** lifecycle transitions in timestamp-descending order, retaining Activity even after Sandbox tombstoning. Only non-secret action, kind, status, UTC timestamp, Principal *kind*, origin, Policy decision and lifecycle generation/state are exposed: no PrincipalRef, policy reason, Tool arguments, credential refs, remote endpoints or provider error messages. HTTP, OpenAPI, SDK, Wails/Desktop and bilingual Web routes have parity, with filtering/pagination/empty/error UX. Read-only Activity does not connect to external Sandbox backends or resolve credentials.

**Commits:** `203b1b9` (documentation-first stabilization contract), `b1b788e` (bounded typed Activity backend, HTTP, SDK and Wails), `4e43aed` (Web Activity details and tests), `51a5604` (security and toolchain maintenance), followed by this checkpoint.

**Security release gate: RESOLVED.** Independently installed Go **1.27.2** (without changing the user's system-wide default Go installation), raised the repository's `go.work` and all four independently buildable Go Module floors to 1.27.2, and updated all four `golang.org/x/net` references from v0.58.0 to **v0.60.0** along with the Go module solver's bounded indirect dependency changes. Rebuilt the repository-pinned `golangci-lint v2.14.0` against the patched toolchain in the local developer environment; its old build was incompatible with Go 1.27.2's export data. Documented two intentional operator-selected local database directory creations for the newer gosec taint analyzer; neither operation is derived from an Agent Tool argument or HTTP request. A backup of the previous local lint binary was stored outside the repository.

**Verification (after code changes):** ordinary, unmodified `pnpm security:check` **exit 0**; govulncheck returned **no vulnerabilities** in Backend, Wails Desktop, Go Provider Plugin SDK and example plugin, and JavaScript production dependency audit returned no known high-severity vulnerabilities. Ordinary `pnpm check` **exit 0**, covering Go lint/test/race/module isolation, contract generation, TypeScript, i18n, **15 Vitest files / 85 tests passed**, and Web/Desktop production builds. Activity Go integration tests verify denied admission visibility, lifecycle persistence after destruction, time ordering, strict 1–100 bounds, invalid Sandbox IDs and absence of sensitive fields. Existing frontend large-chunk warnings (>500 kB) remain nonblocking performance backlog.

**Remote Stage E is NOT closed.** The previously supplied Daytona API Key was reported revoked by the user; this work neither reuses it nor makes any new billable remote calls. Real SDK read-only authentication was verified in the earlier checkpoint, but remote provisioning failed with an uncertain outcome; a Daytona Console check of the matching resource was requested, and provider-side resource deletion has **not** been independently confirmed. A future fresh-account, explicitly authorized live acceptance must verify remote provisioning, `read/search/edit/shell`, Stop/Start/Destroy and the final empty inventory before Stage E/Phase 25 can be marked Complete. E2B remains deferred; Stage D remains closed.

**Change hygiene:** Documentation-first, local segmented commits on `main`; no push. `.agents/`, `.pi/` and `.intent-loop/` remain unmodified and untracked.

## Phase 25 closure decision — bounded acceptance and explicit remote quarantine (2026-10-09)

**Decision (accepted by direction to close Phase 25):** No more Stage D/E suffixes or expanded Sandbox implementation scope. The Phase 25 **core release acceptance** is Local Workspace + Docker execution, stable four Workspace tools, canonical Provider/Profile/Policy/Approval/Audit authority, durable Sandbox lifecycle/leasing and UI/SDK/CLI parity, Sandbox Activity, and clean full CI/security gates. All those capabilities have automated or live Docker evidence in prior checkpoints.

**Daytona acceptance is intentionally narrower:** the remote adapter implementation, persistent quotas/TTL/network restrictions, fake SDK tests and typed management integration are retained as **experimental, not live-accepted**. Its attempted real remote create returned an uncertain result, a real Daytona four-tool/lifecycle test has **not** passed, and final remote resource cleanup was not independently confirmed. Do not call Daytona production-ready or claim provider billing safety. The revoked test credential must not be reused.

**Fail-closed product gate for closure:** Daytona remote creation/SDK connections are disabled by default. Only a local operator who explicitly sets `MENDER_DAYTONA_EXPERIMENTAL=enabled` at Runtime startup may opt in; this is a host-owned deployment decision, never a Tool/HTTP/Provider-controlled flag. When disabled, Daytona is not registered as an executable Sandbox Backend, no new remote Sandbox can be created through CLI, HTTP, SDK, Wails or Web UI, and previously persisted remote Sandbox inventory/audit remains visible but cannot be acquired or mutated. The credential-usage check must **continue** recognizing durable Daytona references while remote execution is disabled, so disablement does not allow deleting a credential that might still protect a remote resource. The opt-in does NOT prove live service availability or a monetary spending cap.

**Deferred operational acceptance (NOT a reopened Phase 25):** An explicitly authorized, credential-backed short-TTL Daytona live smoke must eventually verify successful create/inspect/read/search/edit/shell/stop/start/destroy, provider-side deletion and empty final inventory, and resolve any ambiguous existing `unknown` allocation. The host operator decides if/when to enable experimental Daytona before this check. This is a release-readiness/support decision for that experimental backend, not a Phase 25 functional-completion claim.

**Completion rule:** mark Phase 25 `Complete (core; Daytona experimental and disabled by default)` only after fail-closed gating and default-denial tests, `pnpm check` and `pnpm security:check` all pass. Preserve the historical Stage E checkpoints as evidence of attempted and incomplete remote acceptance, not as live-success claims. Keep E2B and stronger remote isolation out of scope; no new compatibility bridges, no Agent loop, no remote fallback.


## Phase 25 final closeout — core implementation accepted; remote experiment quarantined (2026-10-09)

**Final phase status: COMPLETE within the explicitly frozen core acceptance.** This section supersedes the older, time-ordered implementation and experimental Stage E checkpoint statuses above. The user requested closure without creating further D/E subphases. The final scope is **Local Workspace, Docker, Sandbox governance/router/leases/audit, stable four Workspace Tools, forward-only Workspace Provider hard cut, Activity, HTTP/CLI/OpenAPI/SDK/Wails/Web management parity, and all quality/security checks**. This is not a claim that Daytona's remote service has passed acceptance.

### Release support matrix

| Backend or surface | Phase 25 acceptance | Operational behavior / boundary |
| --- | --- | --- |
| Local Workspace | **Complete** | `host` and `restricted_host` are honestly labeled; restricted host is not a microVM. File and shell share the pinned Workspace path and all accesses traverse Principal/Policy/Approval/Lease. |
| Docker | **Complete (local Engine acceptance)** | Read/search/edit/shell and Create/Stop/Start/Destroy exercised against a real Docker Engine with pinned image and observed non-root/readonly/no-network constraints; requires an explicitly configured local UNIX socket and a cached immutable image. No VM-grade or hostile multi-tenant isolation claim. |
| Sandbox governance/router/management | **Complete** | Durable generation-fenced leases and admissions, deny-before-side-effects, typed lifecycle and unknown-state behavior, Provider/Tool identity preservation, a bounded non-secret Activity page, and HTTP/CLI/OpenAPI/SDK/Wails/Web parity. |
| Daytona | **Experimental / NOT remotely accepted** | Go SDK, immutable Template/credential references, quota and lifecycle implementation plus fake-SDK tests are retained. A real authenticated read-only call succeeded; a real Create was ambiguous/unsuccessful and live tools/lifecycle/cleanup were not proven. **No default creation or execution.** |
| E2B / other remote backends | **Not delivered; outside this Phase** | Remain deferred. No fallback, no compatibility bridge, no automatic remote work. |

### Default-deny quarantine for unaccepted Daytona

- The Runtime constructs only the trusted **read-only inventory/configuration** portion of the Daytona integration by default so persistent Sandbox metadata, Audit and Credential Reference deletion protections remain available.
- **No Daytona execution Backend is registered, and all remote create/stop/start/destroy/reconcile and Workspace acquire attempts reject**, unless an operator explicitly starts Mender with the *exact* host environment setting `MENDER_DAYTONA_EXPERIMENTAL=enabled`. Other values do not enable the driver.
- Opt-in is a **local deployment authority**, not a Tool argument, Profile setting, Provider config, HTTP flag, or proof of successful live testing. The UI displays the experimental label only when opted in, and otherwise shows how the local operator can enable it. The remote provider may incur charges after opt-in; its TTL does **not** enforce a dollar budget.
- Disabled existing Daytona resources remain visible as non-executable inventory. Their active Credential References remain protected against deletion. An operator who later chooses to reconcile or decommission remote allocations must follow separate explicit, reviewed remote procedures.
- No previously supplied revoked Daytona key is reused. The uncertain earlier allocation and provider-side cleanup **remain unverified**. Never claim those resources were deleted simply because a credential was revoked.

### Objective exit-gate evidence

- **Real local Docker**: four sequential real-Engine Race Test end-to-end paths verified four Workspace capabilities, the normal Mender Runtime+Provider pipeline, HTTP lifecycle and CLI lifecycle; only matching test resource labels were eligible for cleanup.
- **Default-deny Daytona**: Go Race Tests validate no operator opt-in means no registration, Create, Tool acquire, or lifecycle/remote reconciliation; immutable persisted inventory remains accessible, active leases are not allocated, and Credential Reference usage still blocks deletion. An explicit host opt-in is tested without making network requests. HTTP/Desktop/UI denial and opt-in intent are tested.
- **Workspace/API/UI**: final plain `pnpm check` exit **0** with Go lint/test/race/module isolation, SDK/proto/Wails generation parity, TypeScript/ESLint/i18n, **15 Vitest files and 85 tests passing**, and Web/Desktop production builds.
- **Security**: final plain `pnpm security:check` exit **0**; govulncheck reports no findings for Backend, Desktop, Go Provider Plugin SDK and its example; the production JavaScript dependency audit reports no known findings. Minimum Go 1.27.2 and `golang.org/x/net@v0.60.0` apply to the independent Go modules.
- **Evidence limitations**: a passing local security scan is not an assurance against future disclosures. Nonblocking JS chunk-size warnings remain outside this scope. The unverified Daytona live acceptance is **deliberately excluded** from Phase 25 core completion; it is an **operational release gate for the experimental provider only**.

### Deferred, non-phase operational acceptance for Daytona

If the user later authorizes a **fresh, short-lived credential** and a potentially billable test, review the previous unknown instance in the Daytona console, verify no stranded resources, confirm the actual remote error/status without logging tokens, create one short-TTL pinned-image Sandbox, prove real remote read/search/edit/shell/Stop/Start/Destroy and ownership labels, and independently confirm final remote deletion/charges. Only after that evidence should an operator reconsider removing the default quarantine through a separate, explicitly accepted maintenance change. **Do not re-open Phase 25, create Stage E suffixes, assume missing API credentials are user consent, or auto-retry ambiguous Creates.**

### Source-control closeout

Documentation-first, segmented local commits; no push. Project code and docs committed to `main`. `.agents/`, `.pi/`, and `.intent-loop/` remain untouched and untracked. This completed Phase 25 is an architecture/runtime delivery, not a new Agent orchestration layer.

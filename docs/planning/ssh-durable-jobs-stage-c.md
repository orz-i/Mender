# SSH Durable Jobs — Stage C: governed optional Tool Suite

- Status: **Closed (2026-10-13)** — staged implementation and automated validation complete; real Rocky/systemd execution remains operator-only acceptance
- Predecessors: [Stage A](plugin-capability-contract-stage-a.md), [Stage B](ssh-durable-jobs-stage-b.md), [ADR-0039](../architecture/adr/0039-offerings-resource-tool-suites.md), [ADR-0040](../architecture/adr/0040-optional-plugin-capability-contracts.md)

## Intent and invariants

**Outcome:** A locally approved SSH Sandbox can opt in to a distinct `sandbox.jobs` suite exposing `jobs.submit`, `jobs.status`, `jobs.logs`, `jobs.result`, `jobs.cancel` through stable canonical Tools, Profile ToolBindings, Policy/Audit and ordinary MCP/SDK/CLI. All authoritative TaskID/record/runner/recovery/cancel state remains **inside sshremote**; Mender Core does not schedule/reconcile/own RemoteTask. A suite is not a privilege merely by appearing in the catalogue.

**Success conditions:**

1. Host-owned reviewed suite requires verified SSH `v1.0` contracts: `job.submit` detached with idempotent submit, reconnect observation and persisted results; `job.observe` inline; `job.cancel` inline. No other backend or mere Descriptor strings can meet the suite. Current root-only systemd restriction remains; no silent fallback to Workspace Shell.
2. Explicit owner `Ensure` creates one canonical, exact Sandbox-bound `io.mender.provider.sandbox-jobs` facade and persists a `sandbox.jobs` binding, without exposing it to any Profile. Ordinary Provider CRUD cannot hijack or delete owned facades. Stable SourceKeys/ToolIDs survive restart. `Remove` requires no remaining Profile/Policy references and must not remove the plugin's remote TaskIDs or lifecycle marker; no silent task cancellation.
3. Profile grants distinguish `job.submit`, `job.observe`, `job.cancel` from `process.shell`; no default Jobs grant. Any remotely accessible Profile must also contain an explicit ToolBinding. `sandbox.acquire` Policy/audit runs before remote SSH credential resolution; canonical `tool.invoke` Policy/audit and reviewed Governance Facts apply to each tool. Submit admission rechecks the exact Profile grant with the SQLite short-lived exclusive lease, atomically with the permanent lifecycle obligation, before any remote task mutation. Cancel uses its own exclusive lease and `job.cancel` grant. Owner-only and delegated scopes fail closed.
4. `status/logs/result` are **read-only**: they can query the same exact pinned SSH backend and remote TaskID without acquiring a write-exclusive `sandbox_execution_leases` row, including when a previous submit ACK/transport cleanup is uncertain. They still require grant and two-step Policy/audit, immutable resource identity and a verified Provider/Tool binding; `get` endpoints never perform a job operation. `cancel` confirmation remains nonterminal until the plugin observes an authoritative terminal marker.
5. The five tools return ordinary bounded complete JSON Outcomes, not `OutcomeKindTask`, so existing MCP servers need no native MCP Tasks extension negotiation. Task handles appear in JSON bodies. Input schemas strictly bound IDs, cursor limits and shell command length. Command text and output never enter the host's policy facts, log diagnostics or audit records.
6. SQLite v14 forward-only migration preserves existing Sandbox/Run/Workspace suite bindings and old Profile grants, widening only reviewed Tool Suite and grant capability constraints. Existing IDs and auth remain stable. `pnpm check`, race/lint, SDK/Desktop binding parity and tests validate all of this. No real Rocky, Wails or Tunnel operations without explicit permission.

**Non-goals:** Cross-backend general RemoteTask, global Task table, AgentTask reuse, automatic reboot resume, checkpointing, native MCP Tasks adapter, Browser or Docker Jobs, unprivileged SSH systemd-user, silent TTL/lease clearance or local Workspace fallback. The source remains explicit about B's root, 4-hour, 64-TaskID and recovery limits.

## Bounded implementation / independent commits

| Cluster | Implementation | Evidence |
| --- | --- | --- |
| C-doc | This source-derived contract and migration/authorization rules | Docs-only commit before code |
| C-contract | Host-curated `sandbox.jobs` requirements, v14 suite/grant migration, protected exact Provider identity + authenticated grant semantics | Migration preserves records; forged/partial offers fail; profile-only Shell fails closed |
| C-execute | Builtin `sandbox-jobs` Provider Facade with five governed Tools; scoped host SSH executor with short submit/cancel leases and independent read-only observation | Invocation/Audit/MCP tests with test-owned SSH/SFTP fixtures and Policy denial, no live Rocky I/O |
| C-supply | Owner explicit Ensure/Remove/Get through management HTTP, Wails Desktop, SDK, CLI, UI with minimal one-view-one-category treatment, i18n | ProviderID/ToolIDs stable, no automatic exposure, contract/bindings unchanged when unwired |
| C-close | Full `pnpm check`, `pnpm security:check`, Go race and documentation summary | Bounded stage closure; no indefinite phase expansion |

## Audited implementation entrypoints

- Host Tool Suite: `backend/internal/toolsuite/{domain/definition.go,infrastructure/curated/registry.go}`; `bootstrap/sandbox_tool_suites.go`.
- Builtin Provider registry: `providerplugin/builtin/builtin.go`; Workspace facade pattern: `workspace/infrastructure/plugin/`.
- SSH B engine and lifecycle: `sandbox/infrastructure/sshremote/jobs*.go`, `sandbox/infrastructure/sqlite/ssh_job_fence.go`, `bootstrap/sandbox_ssh_jobs.go`. The B owner-only seam is private and not a Tool/API.
- SQLite v13: `sandbox/infrastructure/sqlite/migrations.go`; `tool_suites.go` only accepts `sandbox.workspace`; `profile_grants.go` + `leases.go` currently accept only four Workspace operations. Migrated v14 checks must preserve old rows and event history.
- Invocation governance: `providerplugin.GovernanceFactProvider`, `invocation/application/invoke_tool.go`, `sandbox/application/AdmitAcquire`; MCP `OutcomeKindTask` unsupported and deliberately unchanged.
- Contract endpoints/UI: `management/httpapi/tool_suites.go`, `desktop/binding/tool_suites.go`, `contracts/api/openapi.yaml`, `packages/sdk/src/client.ts`, `packages/app/src/domains/sandboxes/{sandbox-tools-panel,sandbox-access-panel}.tsx`.

## Closure evidence (2026-10-13)

| Cluster | Commit | Accepted result |
| --- | --- | --- |
| C-doc | `858fb87` | Source-based ownership, authorization and verification plan committed before implementation |
| C-contract | `030dae8` | Host-reviewed, versioned `sandbox.jobs`; v14 grants (`job.submit`, `job.observe`, `job.cancel`) and **separate** Jobs Suite binding. Existing Workspace Suite table/ProviderID, Sandbox Run, Profile grant/event records and IDs remain intact; standalone Sandbox storage does not require the Provider table to exist when it first opens |
| C-execute | `d768c7e` | Exact-pinned SSH-only host facade `io.mender.provider.sandbox-jobs` with five stable governed Tools and ordinary complete JSON Outcomes, not Core RemoteTask or MCP Tasks. Provider Facade is hidden from generic Provider discovery/create; existing owned configuration remains resolvable |
| C-supply | `1c59861` | Explicit owner GET/Ensure/Remove, HTTP/OpenAPI SDK, Wails Desktop bindings, CLI `sandbox job-tools`, one-category-at-a-time Jobs/Workspace UI and bilingual labels; no automatic Profile or Tool exposure |
| C-close | This documentation commit | Full verification and remaining operator-only acceptance below |

**Automated checks passed:** `pnpm check` exited **0**: plugin protocol lint/generation; external Provider/Sandbox/Tunnel SDK and examples; Go module isolation; generated OpenAPI SDK parity; Wails/Desktop bindings and architecture parity; Go lint, unit tests and **full Go Race suite**; ESLint; strict TypeScript; **i18n 1178 required / 1178 zh-CN translations** across three source roots; **32 Vitest test files / 202 passing tests**; Web and Desktop frontend production builds. `pnpm security:check` exited **0**: no reachable Go symbol vulnerability and no known high-severity production JS dependency advisory (one indirect required Go module vulnerability was not called by the application). `cd apps/desktop && go build ./...` exited **0**; the generated standalone binary was removed after verification. `git diff --check` passed. Pre-existing nonblocking Vite >500 kB chunk-size and macOS linker minimum-deployment-target warnings remain.

**Security and identity proofs:** `TestSSHJobsManagedSuiteOnlyAfterExplicitOwnerPrepare` verified inert catalogue GET, exact SSH root backend requirements, no generic Provider creation/deletion of the owned Facade, explicit five-Tool preparation, stable ProviderID and ToolIDs across Runtime restart, and zero implicit Profile grant/exposure. Versioned contract tests reject Browser/Workspace Shell substitution. v14 migration tests retain Workspace grants and historical grant event IDs, with no Core Task table. The `TestMCPSSHJobsSeparateGrantsAndReadOnlyRecoveryWithUncertainLease` test used a real **local in-memory MCP client/server and local test-owned pinned SSH/SFTP server**: Profile ToolBinding alone could not invoke Jobs; explicit `job.observe` allowed status/result/logs but neither submit nor cancel; `job.submit` did not confer cancel permission; read-only observation still worked with a deliberately held exclusive lease while a fresh submit did not. All five Tool schemas and argument rejection paths were tested with the canonical validator. Audit/Policy facts contain only reviewed effects and immutable SandboxID, never command text or output.

**Operator-only verification still outstanding:** The tests do **not** establish that a real Rocky Linux 9.8 host's systemd PID1 accepted a transient service, that SELinux permits remote script/output writes, that a real root task actually continues after network loss, or that cancellation produces a durable terminal marker under remote process killing. No real Rocky command, user SSH secret, live Wails restart, MCP/Tunnel restart, or remote machine change was performed. Stage B's root-only systemd-system-manager restriction, four-hour maximum runtime, retained 64 TaskID slot limit, lack of host-reboot auto-restart/exactly-once effects, and reviewed recovery for crash-held uncertain leases are unchanged. Native MCP Tasks negotiation remains a separate future concern and is **not** advertised.

**Boundary:** Stage C adds an optional governed Tool *supply* and authorization path, not a Core job daemon, Scheduler, Agent loop, centralized RemoteTask model or automatic remote replay. The SSH plugin remains the authority for TaskID, logs, result, cancellation and reconciliation; Core is limited to exact resource binding, immutable reviewed suite metadata, Policy/Principal/Scope/Audit, short-lived leases and lifecycle fencing. `.agents/`, `.pi/`, `.intent-loop/` stay outside Git staging. Real Rocky smoke testing and lease-recovery/retention policy expansion require separate scoped work rather than extending this stage.

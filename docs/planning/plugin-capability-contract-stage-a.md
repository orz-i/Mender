# Plugin Capability Contract — Stage A closure plan

- Status: **Closed (2026-10-13)**
- Decision: [ADR-0040](../architecture/adr/0040-optional-plugin-capability-contracts.md)
- Scope: bounded preparatory implementation only; Stage B SSH Durable Jobs and Stage C Tool Suite supply remain separate.

## Intent contract

**Outcome:** Capability discovery and Tool Suite compatibility become versioned, explicit and fail-closed without growing Mender into an Agent Runtime or Remote Job Scheduler.

**Success:**
- A1. Define bounded versioned capability offers and requirements with independently checkable `inline` / `detached` modes and explicit durability traits; reject malformed names/versions, unknown modes/traits, duplicate or conflicting offers, incompatible majors or insufficient minors and guarantees.
- A2. Preserve and validate the current Sandbox Workspace stack end to end: the four host-verified workspace operations project to `v1.0/inline`, the reviewed `sandbox.workspace` definition negotiates against that exact backend, Browser and hypothetical detached-only Jobs cannot satisfy Workspace.
- A3. Keep all existing ToolIDs, ProviderIDs, Profile grants, SQLite bindings, public management shapes and sandbox plugin v1 wire messages untouched. No new executable suite, implicit grants, network probe, SSH job, plugin installation or actual resource execution on catalogue read.
- A4. Independent tests prove declarations do not grant authority, older/partial/unknown capabilities fail closed, and only exact-pinned registered runtime backends are eligible. `pnpm check`, lint/race and `git diff --check` pass.

**Invariants:** `Resolve → Admission → Acquire → Dispatch`, external installation verification, process-free discovery, no host fallback, Profile/Policy/Audit, plugin-backed execution state ownership, prior isolation caveats and `OutcomeKindTask` behavior. Do not edit running Wails/MCP/Tunnel/SSH resources, private keys, `.agents/`, `.pi/` or `.intent-loop/`. Respect bilingual UI (no UI changes in Stage A).

**Explicit non-goals:** Extending Sandbox Profile Grant enum/SQLite, generalizing `sandbox_tool_suite_bindings`, implementing `sandbox.jobs` Provider facade, adding new native Sandbox Protocol RPCs, MCP Tasks mapping, Plugin TaskStore, `systemd-run`, checkpointing, global reconciler or remote task scheduler. These remain B/C decisions with separate migrations and tests.

## Executable work clusters / commits

| Cluster | Output | Gate |
| --- | --- | --- |
| A-doc | Accepted decision + source audit + stage contract, committed before code | Commit contains docs only |
| A-contract | Pure bounded versioned capability offer/requirement model; reviewed suite requirements; unchanged public projection | Positive/negative unit tests; no mutable aliasing or implicit authority |
| A-negotiate | Exact backend / versioned inline workspace offer projection used by managed suite eligibility | Existing Sandbox/Provider integration and negative tests pass; no non-Workspace operation exposed |
| A-verify | Full repository validation, status and closure evidence | `pnpm check` + security gate if available; staged commits and clean tracked tree |

## Audited source baseline

- `backend/internal/plugin/extension.go`: generic `Descriptor.Capabilities []string`, arbitrary syntax but no execution/version evidence.
- `backend/internal/sandbox/domain/sandbox.go`: four Workspace operation enum, boolean capability set; intentionally kept as typed Workspace v1 execution port for Stage A.
- `backend/internal/sandbox/application/ports.go`, `infrastructure/pluginregistry/registry.go`, `infrastructure/external/dialer.go`: registry validates exact extension and current workspace subset; external Sandbox protocol v1 rejects extra operation names.
- `backend/internal/toolsuite/domain/definition.go`, `infrastructure/curated/registry.go`: host-reviewed static suite definitions, unversioned set membership, Browser remains unavailable.
- `backend/internal/bootstrap/sandbox_tool_suites.go`: owner-approved Workspace facade establishment, exact backend capability check before creation, no implicit Profile exposure.
- `backend/internal/sandbox/infrastructure/sqlite/migrations.go`: `sandbox_tool_suite_bindings` check accepts only `sandbox.workspace`; Profile Grant capability check accepts only four Workspace operations. **Do not widen in Stage A.**
- `backend/internal/mcp/infrastructure/server/server.go`: `OutcomeKindTask` returns unsupported by design; leave untouched.

## Future handoff contract

- **B — SSH backend-owned Durable Jobs:** TaskID, on-host durable store, systemd runner, idempotent submit, status/logs/result/cancel/reconnect, explicit lifecycle fence. Never occupy a long-lived Mender SSH Session Lease and do not automatically replay uncertain side effects.
- **C — supplied Jobs tools:** opt-in host-reviewed `sandbox.jobs` binding/facade, Profile/Policy/Audit, named Task tools; extend SQLite suite/Grant contract only after proving no TOCTOU lifecycle bypass, and separately consider MCP Tasks negotiation. Non-Workspace plugins can use distinct suites/ports.

## Verification / commit log

| Milestone | Commit | Evidence |
| --- | --- | --- |
| A-doc — architecture and acceptance before source code | `533dd5f` | ADR-0040 accepted, exact scope/non-goals; docs-only commit |
| A-contract + A-negotiate — one atomic hard cut | `e4a9f14` | Versioned capability contracts in `plugin`, host-reviewed `RequiredContracts` with unchanged public `requiredCapabilities`, exact-pinned verified Workspace v1/inline offers and contract-based suite eligibility |
| A-verify — final evidence | Recorded in this document | See gates below; no further feature implementation |

**Automated gates:** `pnpm check` exit **0**: plugin proto/SDK/examples, Go module isolation, OpenAPI/SDK generation parity, desktop binding/architecture parity, Go lint/tests/**race**, ESLint, TS strict, **i18n 1167/1167**, **32 Vitest files / 202 tests passed**, Web and Desktop frontend builds. `pnpm security:check` exit **0**: no reachable Go symbol vulnerabilities, no high-severity production JS audit findings (one indirect module finding was not reachable from application symbols). `cd apps/desktop && go build ./...` exit **0**, with existing non-blocking macOS linker deployment-target warnings. `git diff --check` passed. Vite emitted pre-existing non-blocking >500 kB chunk-size warnings.

**Security / boundary checks:** invalid versions/IDs/modes/guarantees and duplicate contracts rejected; newer minor compatible only within same major, required guarantees never inferred; detached alone cannot satisfy durable requirements; Browser and detached Jobs cannot satisfy Workspace; a newly reported backend operation cannot bypass its immutable descriptor; a backend that loses support ceases to advertise it. A Tool Suite unavailable by design exposes zero runnable tool keys. Registry and descriptor snapshots are cloned; catalogue/listing never starts a backend, touches Credentials, mutates Profile grants or creates Providers.

**Intentional limits:** Only existing `sandbox.workspace` is executable. External Sandbox Protocol v1 remains Workspace-only and accepts **no** versioned async/Jobs declaration. `sandbox.jobs` appears only in a contract test as an **unregistered, unavailable** definition; no Job TaskID, Job Store, remote worker, external protocol change, generic Scheduler, MCP Tasks mapping, SQLite schema change, SDK or UI change was introduced. Future Stage B must prove remote job lifecycle/idempotence and an atomic Sandbox lifecycle fence; Stage C may then add independent optional suite binding and Profile exposure with the required persisted-grant migration.

**Operator safety:** No Wails/MCP/Tunnel restart, Rocky connection, remote SSH command or user credential access. `.agents/`, `.pi/`, `.intent-loop/` remained untracked and untouched. Tracked Git tree clean following code commit; only the explicit documentation closure is committed next.

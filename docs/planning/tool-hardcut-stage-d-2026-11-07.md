# Tool ownership hard cut — Stage D: typed Catalog and source router (2026-11-07)

**Status: Implemented for domain, host-reviewed registration contracts and isolated SourceRef SQLite adapter. NOT activated in the production Runtime.** Stage E/F is a **single coordinated cutover**, not an invitation to run old Provider facade and new native Tool sources side-by-side.

**Base:** [ADR-0064](../architecture/adr/0064-tool-provenance-and-execution-placement-hardcut.md) · [Phase A migration contract](tool-hardcut-phase-a-migration-contract-2026-11-07.md) · [Stage B](tool-hardcut-stage-b-2026-11-07.md) · [Stage C](tool-hardcut-stage-c-2026-11-07.md).

## 1. Delivered implementation

| Owner | Stage D deliverable | Explicit boundary |
| --- | --- | --- |
| `catalog/domain/tool.go` | Canonical Tool now stores one validated `SourceRef(provider|agent|native)`; `NewToolFromSource`/`RestoreToolFromSource` preserve ToolID/SourceKey/SourceRef and immutable timestamps. Old `NewTool` and `ProviderID()` remain for the *currently deployed* Provider-backed route. | A non-Provider Tool has no ProviderID; no fabricated Provider/Sandbox is created. The old Provider accessor must be retired with all consumers in E/F. |
| `catalog/application/sync_provider_tools.go` | Independent Provider discovery now constructs `NewToolFromSource(provider:ProviderID)` and checks stored source before metadata sync. | Existing Provider synchronizer still uses its live v2 Store and does not insert Agent/native sources. |
| `invocation/infrastructure/router/router.go` | Executor registry keyed by exact SourceRef, with Host-only `RegisterSource`; legacy `Register(providerID)` wraps typed identity. Agent instances cannot borrow another Agent's executor. Native route requires exact pinned ToolID/SourceKey from host-reviewed registry. | No Agent/native route registered in production bootstrap; existing Provider route still functions. Source registration does not grant Tool execution. |
| `toolsuite/infrastructure/curated/native_catalog.go` | Nine fixed native ToolIDs now have **complete reviewed** names, descriptions, Tags, bounded JSON input/output schemas. Workspace tools accept optional requested `workspaceId` (authorization checked separately); durable SSH Job tools require an explicit `sandboxId`, and status/log/result/cancel require TaskID. | A model-provided Workspace/Sandbox ID is NEVER an authorization token, mount proof, grant or source. This new schema is NOT advertised to MCP yet. |
| `catalog/infrastructure/sourcecatalog/schema.go` | Proposed typed SQLite Catalog layout (`tools.source_kind/source_id/source_key`, unique composite key, typed source FTS5, Catalog revision triggers and Provider-owned SQL delete/insert guards). | `FreshSchema` is **for isolated new databases and Stage E schema planning**; never added to current `catalog/sqlite/migrations.go` and never executed against production data. |
| `catalog/infrastructure/sourcecatalog/repository.go` | Separately tested future repository: injected Host `OwnerVerifier`, atomic snapshot upsert, exact source/ToolID/key checks, native Schema immutability, explicit refusal of silent Tool retirement or ID reassignment, deterministic source list/count and source-isolated FTS search. | **Not wired to `NewCatalog` or `bootstrap.Runtime`. Agent-owned SQL commit-point guards, Profile/Policy links, permission-preserving retirements, complete migration and per-Profile search join still belong to Stage E/F.** |

**Intentional lifetime split:** Provider, Agent and Native are **different Tool provenance**, not three execution environments. Sandbox/Workspace remain placement, not owned Tool sources. Agent as Tool Task/Session/ACP and SSH Job durability stay in their owning modules. No Agent Loop ownership change.

## 2. Strict data contract for E

The planned source-aware Catalog uses these normalized columns (not a new Source aggregate):

- `tools.id`: stable canonical ToolID.
- `tools.source_kind`: one of `provider` / `agent` / `native`.
- `tools.source_id`: exact ProviderID / AgentID / host-native registry key.
- `tools.source_key`: immutable operation identity unique within `(source_kind,source_id)`.
- Schemas, tags, timestamps, Catalog revision, per-source FTS; no SandboxID or Workspace root in Tool ownership.

The isolated repository requires a verified owner and guards exact identities. **However, an out-of-transaction SourceOwner check is not enough to protect against concurrent Agent deletion**. Stage E must install cross-context SQLite commit-point guards, harmonize Provider configs and foreign references, and migrate every Profile/Policy/Direct/Hybrid/Agent ToolPlacement source identifier in one auditable transaction. Stage F must update every consumer of `Tool.ProviderID()` (Invocation preflight/admission/audit/runtime budget, Provider activation, MCP Direct naming and metadata lookup, management DTOs, CLI, Wails, SDK).

When a remote Provider stops exposing a Tool, the isolated repository currently refuses to retire its ToolID automatically (`ErrRetirementRequiresReview`). E/F must integrate explicit reference-safe Provider retirement that checks Profile/Policy/delivery references under the new SQL guards. This is not a live behavior regression because the v2 Provider sync remains active until cutover.

## 3. Native Tool schema decisions and compatibility

| Source | Supported canonical operations | Explicit target boundary |
| --- | --- | --- |
| `native:workspace` | `workspace.read`, `workspace.search`, `workspace.edit`, `workspace.shell` | Optional `workspaceId` is a **request**; only a registered, permitted Workspace can be selected. Stage C picks verified placement, never an arbitrary Tool-supplied Sandbox |
| `native:jobs` | `jobs.submit`, `jobs.status`, `jobs.logs`, `jobs.result`, `jobs.cancel` | `sandboxId` is required until authoritative Job→Sandbox ownership lookup exists; independent JobID, capability, Profile grant, admission and evidence check remain mandatory |
| `agent:<AgentID>` | `agent.delegate`, `agent.status`, `agent.result`, `agent.cancel`, `agent.resume` | Distinct AgentIDs (even same Backend) retain separate ToolIDs and Task/Session authority. No Mender Agent Loop |
| `provider:<ProviderID>` | Plugin-discovered operations | Provider activation/credential/connection boundary; never silently executed in Sandbox |

The planned Native Schema must not be copied from Provider facades verbatim: the old Workspace facade accepted a Provider-local `workspace` alias and the old SSH Jobs facade pinned Sandbox via Provider configuration. Both are deliberately absent from the future schemas; **this is a breaking API and ToolID transition requiring Stage E dry-run and Stage F client re-export**.

## 4. Negative verification requirements

- Invalid, missing or unknown SourceRef/AgentID/ProviderID fails constructor or OwnerVerifier.
- Same Backend, two different Agents with identical `agent.delegate` names: cannot share tool route or FTS source.
- Arbitrary ToolID/SourceKey under `native:workspace` cannot invoke a privileged native executor. Fixed registry must match ID, key, SourceRef and reviewed JSON Schema/tags.
- Upsert cannot adopt an existing ToolID for a different owner, replace a source key under the same ToolID, lower created/updated times, or silently remove referenced tools.
- Provider Source writes must be checked at SQL commit point (Provider insert/delete guards); Agent source cross-context guards are **E prerequisites**, not Stage D claims.
- FTS query is bounded, tokenized/quoted and scoped to exact SourceRef; Profile-scoped search must still join exact current Profile ToolBindings before exposure.
- Fresh typed schema must be **rejected** by the existing legacy Runtime until the coordinated cut; legacy v2 schema must be rejected by the future adapter. No automatic upgrade or fallback.
- Existing SourceProvider sync, Router, Agent Tool delegation, Workspace bridge and SSH Job suite continue to work without new live source admission.

## 5. Test evidence and limits

**Actual Stage D verification:**

- [x] `go test -count=1 ./backend/internal/catalog/... ./backend/internal/invocation/infrastructure/router ./backend/internal/toolsuite/infrastructure/curated ./backend/internal/workspace/infrastructure/sandboxbridge` — all selected Go packages pass.
- [x] `go test -race -count=1 ./backend/internal/catalog/... ./backend/internal/invocation/infrastructure/router ./backend/internal/toolsuite/infrastructure/curated` — all selected packages pass, including Catalog v2 and isolated SourceRef v3 SQLite.
- [x] `go test -count=1 -parallel=2 -timeout=4m ./backend/...` — full backend ordinary Go suite passes.
- [x] Bootstrap fixtures for Agent Delegation, MCP/OpenAPI/Model Tools, Provider reference guards, Sandbox Tool Suites and SSH Job recovery — selected integration regression tests pass.
- [x] `go run scripts/check-backend-architecture.go` — passes; `golangci-lint run ./backend/...` — 0 issues after fixing three new error message capitalization warnings.
- [x] `pnpm desktop:bindings:check && pnpm sdk:check && pnpm lint && pnpm typecheck && pnpm test && pnpm build` — Wails + SDK codegen stable; i18n 1639/1639; Vitest **76 test files / 512 tests**; Web/Desktop builds pass with existing large-chunk warnings.
- [x] `git diff --check` and Markdown local-link checks before commit.
- [ ] Full canonical `pnpm check` with default Bootstrap Race — **not rerun** because the unresolved B/C 10-minute/long-running Bootstrap Race gate requires separate scoped performance and scheduler remediation. Above focused Race results are not a full-gate substitute.

**Important:** Stage B's `pnpm check` encountered a 10-minute parallel Bootstrap Race timeout; Stage C's bounded full-backend Race run was stopped after long Bootstrap runtime (`exit 143`). These are unresolved **full-suite quality gate** issues. Do not infer full canonical `pnpm check` green from Stage D scoped Race tests. Do not claim staged SourceRef datastore is already active.

**Stage D is not production hard cut.** The verified code is explicitly prewired contracts plus existing Provider-only behavior. E/F must atomically deliver source-aware SQLite + Host SourceOwner guards, Profile TargetScope persistence, new Invocation admission/Policy/Audit routing, Agent and SSH Job owner-specific executors, Source-first MCP/CLI/SDK/Wails contract, and forward-only migration. Incomplete cuts fail closed. The final release must remove obsolete facades, compatibility accessors and dead code, not carry two operating paths forever.

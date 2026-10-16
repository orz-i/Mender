# Agent Runtime Configuration & Workspace Reuse — Phase 40

Status: **B1 complete; B2 integrated and tests passed; Stage C next** (2026-10-16). Authority: [ADR-0047](../architecture/adr/0047-agent-runtime-configuration-workspace-reuse.md).

## Intent Contract

**Outcome:** One existing Agent resource and one existing Workspace Tools source can be reused across explicitly authorized model and local workspace selections, without creating duplicates or changing upstream Tool identities.

**Success checks:** A documented typed model/selection contract; optional model selected via ACP v1 capabilities or rejected before prompt; owner-defined local workspace options with strict Profile access; Task snapshots immutable through config changes; stable MCP Agent tool identity; one Workspace Tools facade invoking multiple separately permitted Sandboxes; HTTP/OpenAPI/SDK/Wails/UI consistency; regressions and full project checks.

**Invariants:** Mender serves tools/resources and governs delegation, never hosts an Agent reasoning/model loop. No arbitrary model CLI flags, raw cwd from model, remote/host fallback, unsandboxed privilege widening, implicit Profile permissions, auto tool publishing, unknown task replay or background model invocation. Existing AgentID/ProviderID/ToolID/Profile bindings preserved. Forward-only DB/schema revisions; no bridge or legacy runtime dual read. Do not modify or commit `.agents/`, `.pi/`, `.intent-loop/`, `.commandcode/`.

**Authority:** Code, schema, docs, deterministic fixtures and tests may be changed. Do not invoke real external account or chargeable model; do not reset user data; do not modify live sandbox/credential ACLs without separate instruction.

## Milestones and gates

| Commit boundary | Scope | Gate |
|---|---|---|
| A docs | ADR-0047, Phase 40 contract, owner/Task/security decisions | Docs review; **commit docs first** |
| B1 core | Domain/SQLite/application selection snapshot and grants; owner workspace administration | `go test ./backend/internal/agent/...`; grant, revocation, model-agnostic task snapshot tests |
| B2 integration | ACP v1 configOptions, launcher cwd, Tool schema, HTTP/OpenAPI/SDK/Wails/UI | Synthetic ACP tests, MCP fixture, Go/TS frontend/SDK tests |
| C workspace | Multi-target Workspace Tools bindings, authorization, route/facts, API/UI | Sandbox grant/lease negative tests, old ToolID identity check |
| D closure | Cleanup, docs, parity and full repository checks | `pnpm check`; audit no changed skill paths |

### Evidence from Stage B

- B1: `c7a6286` persists default model, per-task immutable selection, explicit local Sandbox workspace bindings and Profile workspace grants with SQLite atomic admission fencing. Synthetic grant/revocation tests pass.
- B2: ACP v1 model selector reads session `configOptions`, requires an advertised model and confirmed `session/set_config_option`, passes pre-authorized local cwd into process launch and `session/new`. The four stable delegation Tools accept optional model/workspace. Owner management is available through CLI, HTTP, SDK, Wails and focused Agent UI. New synthetic ACP model-choice fixtures and HTTP bind/grant lifecycle test pass; no real Agent account or model invoked.
- Validation B2: `go test ./backend/internal/cli ./backend/internal/management/httpapi ./backend/internal/agent/...` passed, `go test ./backend/internal/bootstrap` passed, `pnpm lint`, `pnpm test` (232 tests), `pnpm typecheck`, `pnpm i18n:check`, `pnpm go:lint`, desktop architecture check passed. SDK and Wails generation completed; clean-diff drift checks rerun after tracked generated bindings are committed.

No extra conceptual tiers such as Agent Presets, Workspaces inventory or Provider clones. A failed gate stays open and is recorded honestly; don't claim an unverified acceptance.

## Risks to resolve in implementation

- Match the specific ACP v1 `configOptions` model selector shape, not generic JSON or v2 fields. Validate response options and select *only* a listed string ID.
- Ensure the selected root is resolved from owner-approved persisted references and rechecked immediately before process launch. Existing host/restricted-host labels are not proof of stronger isolation.
- SQLite Task admission must fence Profile's selected workspace grant atomically with Task reservation; revocation does not retroactively reassign running work.
- Workspace per-target selection must enter policy/governance facts before leasing, not only inside the executor after Policy has allowed an unknown target.
- Existing managed sandbox/provider removal guards must account for any secondary target associations.

# Agent Runtime Configuration & Workspace Reuse — Phase 40

Status: **Phase 40 implementation closed; full automated acceptance passed** (2026-10-16). Real account/model and restarted desktop operator smoke remain separate, unverified checks. Authority: [ADR-0047](../architecture/adr/0047-agent-runtime-configuration-workspace-reuse.md).

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
- C: `f22a3d7` stores extra Sandbox target aliases under existing managed Workspace Providers, retains the same four ToolIDs, exposes `workspace` as an optional structured argument, and routes invocation through exact Sandbox Policy/lease. Canonical Invocation forwards host-reviewed Policy facts so a changed target cannot be executed with a stale `sandbox_id` preflight. CLI, HTTP/OpenAPI, SDK, Wails and focused Tools UI manage only registered owner targets; no Profile exposure is implicit.
- Validation C: full `go test ./backend/...` passed; real MCP fixture verified default versus selected file contents, no cross-Profile grant reuse, target grant/revocation and alias removal; negative fixtures verified provider/alias isolation and Policy-to-Executor target mismatch. `pnpm test` passed (233/233), `pnpm lint`, `pnpm typecheck`, `pnpm build` and `pnpm go:lint` passed. SDK and Wails generation parity verified after the C commit. Sandbox migration fixtures were updated to simulate old versions correctly and keep historical backup/rollback checks. Targeted race checks passed for Agent, Workspace, Sandbox SQLite, Invocation and workspace/Agent management flows.
- D full-gate evidence (2026-10-16): **`pnpm check` exit code 0**, captured locally in `/tmp/mender-phase40-canonical.log` with exit receipt `/tmp/mender-phase40-canonical.exit`. This includes Provider/Sandbox/Tunnel plugin protocol and SDK/example lint/tests/race/builds, module isolation, OpenAPI/TS SDK generation and Wails binding drift checks, Go lint, **full `go test ./backend/...` and `go test -race ./backend/...`**, ESLint, TypeScript strict checks, **Vitest 42 files / 233 tests**, i18n (1232/1232), and Web/Desktop production builds. Vite reports a non-failing large-chunk advisory; no test/check failure. The first direct remote command attempt did not return results due to transport timeout; the canonical run above completed independently and returned a verified zero exit code.

No extra conceptual tiers such as Agent Presets, Workspaces inventory or Provider clones. A failed gate stays open and is recorded honestly; don't claim an unverified acceptance.

## Risks to resolve in implementation

- Match the specific ACP v1 `configOptions` model selector shape, not generic JSON or v2 fields. Validate response options and select *only* a listed string ID.
- Ensure the selected root is resolved from owner-approved persisted references and rechecked immediately before process launch. Existing host/restricted-host labels are not proof of stronger isolation.
- SQLite Task admission must fence Profile's selected workspace grant atomically with Task reservation; revocation does not retroactively reassign running work.
- Workspace per-target selection must enter policy/governance facts before leasing, not only inside the executor after Policy has allowed an unknown target.
- Existing managed sandbox/provider removal guards must account for any secondary target associations.


## Operator verification after rebuild (not claimed as passed)

- Rebuild/restart the Wails client using the operator's normal workflow. Open an existing Agent, change its **default model**, register a second **ready local_workspace Sandbox** under **Workspaces**, grant a Profile access and verify that the same Agent and four delegated ToolIDs remain visible. Changing defaults alone must never run a CLI or modify a Profile grant.
- Open the **Tools** service backed by Sandbox Workspace Tools, choose **Manage workspaces**, add a Sandbox under an alias and use **Refresh tools**. Verify the same four ToolIDs remain stable, then independently grant and revoke the target Sandbox capability under **Sandboxes → Access**.
- Optionally use a low-risk temporary local file and authorized Profile MCP client to verify selecting and revoking the target; automated fixtures already cover this using isolated resources, not the user's live workspace.
- Do **not** infer available paid models from passive binary discovery. Real Codex/Grok authentication, ACP handshake with the installed versions, account charges and an actual model prompt were intentionally **not** exercised. These require explicit operator authorization. ACP session persistence, Docker/SSH Agent processes and remote Agent execution were intentionally not implemented.

The implementation does not require duplicate Agents or Workspace Tool services to switch among approved targets. Existing identities, ToolBindings and Policy references were preserved by forward-only SQLite schema revisions; no V1 dual-read or bridge was introduced.

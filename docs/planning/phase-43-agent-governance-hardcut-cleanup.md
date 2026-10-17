# Phase 43 — Agent runtime governance hard-cut cleanup

Status: **completed** (2026-10-17). Full automated, isolated real-database, native compilation and live Mender OpenCode delegation acceptance passed. The running native Wails backend restart is deferred to preserve the connected tunnel. Documents first. Authorities: ADR-0038, ADR-0047, ADR-0048, ADR-0049.

## Baseline live audit and safety envelope

The running user Mender v9 SQLite resource inventory has three Agents (Codex, Grok, OpenCode), each with exactly one pinned Agent Delegation Provider. Read-only queries show **zero unpinned Agent Delegation Providers**, **zero dangling pinned Provider configs**, and one explicitly granted additional OpenCode workspace. The Agent Tasks table contains retained historical completed, failed, and currently executing tasks. This means there is **no verified orphan Agent/Provider/Tool/Grant row to physically delete**. Do not delete task history, Provider/Tool IDs, Profile ToolBindings, AgentGrants, Workspace grants, live credential state or schema migration records. All user records remain intact.

## Confirmed cleanup candidates

1. **Unpinned/legacy Agent Provider adoption at runtime.** `bootstrap/agent_tools.go` emits `legacy` when it sees exactly one ProviderConfig bound to an Agent but not pinned in `agent_tool_facades`; `EnsureAgentTools` then silently adopts it after owner click. This bridge was useful during Phase 34 but is no longer applicable to the current database. Remove the `legacy` Agent state, adoption path, UI hint, CLI documentation, and old success fixture. Preserve fail-closed protection: an unpinned matching Provider must become `conflict` and **must not** be rebound, replaced or deleted, even if only one remains. A pinned Provider with missing tools must still be repairable using the same ToolIDs; missing Provider creates only an expressly new managed suite. Multiple or mismatched configs remain conflicts.
2. **Legacy optional Agent execution adapter.** `agent/application/service.go` has `Executor.Execute` and an optional `ConfiguredExecutor.ExecuteConfigured`, with a fallback to `Execute` for default tasks and rejection only for selected model/workspace. All production ACP executions already use immutable `RunConfig`. Hard-cut to one required `ExecuteConfigured` contract; remove the runtime type assertion, nil fallback condition and dual default invocation. Convert test fakes so *all* tasks, including those without overrides, execute with admitted cwd/model/reasoning snapshot. No extra process or permissions.
3. **ACP `selectModel` forwarding helper** `agent/infrastructure/acp/model.go` exists only for its old tests. Remove the adapter and rewrite tests to call `applySessionChoices`, preserving advertisement, confirmation, and model-before-effort checks.

### Explicitly retained (not dead code)

- `agent_schema_migrations` v1–v9 steps and historical v7→v8→v9 conversion functions are **forward migration support**, not disposable runtime shims. Removing them would strand older persisted user stores.
- `agent_tool_facades` pinned identity and Provider deletion trigger; Agent/ToolID/grant/profile/Policy/Principal and immutable Task snapshot lifecycle, cancellation and `unknown` status; process isolation and ACP fail-closed client capability/permission response; exact live workspace target verification.
- Provider/Sandbox/Workspace Tool legacy statuses are *not* in scope; these serve independently governed resources and require separate evidence before any hard cut.

## Real Mender OpenCode delegation acceptance

The owner requested a real `Mender → OpenCode` dispatch. The first read-only repo inspection was accepted and tracked as `atask_af19cb49c2d7f810764330ce530ebcce` but ended `failed` with generic `Agent execution failed`. Review of `acp/runner.go` confirms Mender rejects ACP `session/request_permission` and client-side tool requests by design; no repository file access through delegation should be assumed. A second independent **bounded code review based only on source excerpts explicitly provided in the prompt** was submitted as `atask_150be6591f930ea17609953b8491bf4a`; its actual status/result must be separately verified. Never claim a file-read audit occurred if the external Agent could not access files. Do not relax ACP permissions as a side effect of cleanup.

## Work plan, quality gates and commits

- **A — Docs (first commit).** Preserve identity/data invariants and identify exact code, tests, unverified delegation limitations.
- **B — Remove legacy facade adoption.** Update Agent status, owner CRUD and tests; cross-check active User DB remains all pinned. Commit after Go/React tests.
- **C — Unify Agent execution interface and retire redundant ACP shim.** Refactor fixtures without weakening scoped permissions or Task snapshot; commit after Go test/race and local prompt fixture.
- **D — Final cleanup, data integrity & end-to-end acceptance.** Full `pnpm check`, SDK/Wails parity, independent read-only live database inventories, OpenCode delegation result, document status, staged commit. Do not restart Wails while tasks are running; never mutate production user inventory as a cleanup shortcut.

Hard cuts only: no compatibility adapters, no silent workspace rebind, no new Agent/Provider, no migration reset, no model-controlled cwd. Ignore `.agents/`, `.pi/`, `.intent-loop/`, `.commandcode/` in every commit.

## Implementation evidence and OpenCode results (2026-10-17)

- **A — `111d1fc`**: read-only Agent data/legacy-path audit and hard-cut contract, committed before modifying code.
- **B — `69f6130`**: retire the Agent `legacy` facade status and implicit adoption of an unpinned existing Provider. A unique unpinned Provider now fails closed as `conflict`, as does a duplicated/mismatched Provider. Preserve new managed Provider creation, pinned Provider schema refresh, Profile ToolBindings, Agent grants, ProviderID/ToolIDs, and explicit Agent delete guards. Replace the former adoption-success test with a full negative test showing the existing ToolIDs, Provider config and unclaimed state survive the denied operation; existing pinned tool suites remain ready/idempotent.
- **C — `aa08d36`**: remove the separate old `Executor.Execute` fallback and optional `ConfiguredExecutor` interface. Every Agent task now requires `ExecuteConfigured`, receiving the exact admitted Task `RunConfig{WorkspaceRoot,Model,ReasoningEffort}`. Remove the obsolete test-only `selectModel` adapter and a redundant no-op pre-session callback branch; synthetic ACP and application fakes now implement only the canonical call. A new test proves even a default-only Task runs with explicit canonical cwd/empty model and effort, and its durable result/snapshot match.
- **Independent review and closure — `2b9ab01`**: OpenCode, through the user's actual Mender connector, identified one remaining ACP `withSession` blank-root fallback. The new Runner rejects empty `RunConfig.WorkspaceRoot` *before spawning an ACP process*, with a synthetic test proving that the executable factory is never called. The existing ACP model-then-effort advertisement and confirmation/rejection tests were preserved.

### Real plugin delegation acceptance

The local MCP client discovered the four canonical **OpenCode Agent Delegation** ToolIDs and Mender accepted tasks under the existing `default` Profile. The external OpenCode responses were actually retrieved through `agent.result`:

| TaskID | Result | Trust boundary |
|---|---|---|
| `atask_af19cb49c2d7f810764330ce530ebcce` | `failed`: generic `Agent execution failed` | Asked OpenCode to read local repo; repository read **not verified**; Mender currently rejects ACP client tool/permission requests by design |
| `atask_150be6591f930ea17609953b8491bf4a` | `completed`: review of actual `agent_tools.go` facts | Only the user-supplied source excerpts in the prompt; no file/tool access. Supported removal of legacy adoption and protection of pinned IDs |
| `atask_2e6fd2e8be12f18b780a9c8b35ffae38` | `completed`: review of actual Stage C code contract | No file/tool access. Supported the unified Runner and recommended removal of an empty-root fallback, which was then implemented and tested |
| `atask_0d88024d52e61db2c0faf9b07833c9de` | `completed`: `MENDER_GAOGE_WORKSPACE_OK` | Explicit `workspace=gaoge`, Task snapshot persisted `/Users/mini/Coding/Gaoge` with the Profile workspace grant; no project files touched |

The first failure is a meaningful capability/diagnostics finding; do not misclassify it as successful delegated source scanning or quietly grant client filesystem/shell authority. A future, independently reviewed ACP permission mediation design is needed for Agent-directed real coding operations through Mender. No unauthorized client Tool requests were enabled in this phase.

### Read-only user-data review

Production App Support SQLite remains at Agent schema **v9**, with three pinned Agent Delegation Providers for three Agents (Codex, Grok, OpenCode). There were **zero** unpinned delegation Provider configs, dangling pins, orphan Agent Tasks or dangling AgentGrants; `PRAGMA quick_check=ok`, `foreign_key_check` zero. Existing Agent/Provider/ToolIDs and 25 Profile ToolBindings, two Agent grants and one registered extra workspace with one Profile workspace grant remain intact. The delegated tests add historical Task records, as designed; historical `completed`/`failed` records are governance evidence and were **not deleted**. The Codex Agent is a user-owned, intentionally retained resource even if not currently Profile-exposed.

Old SQLite migrations v1–v9 (including v8/v9 hard-cut migration scripts) remain intentionally: they are required to upgrade older user databases. Only runtime compatibility paths were removed. No automatic data purge, forced legacy Provider reassociation, credential inspection or schema reset took place.

### Testing and rollout boundaries

A focused `go test -race ./backend/internal/agent/...` and Agent Bootstrap tests passed on the final code, as did SDK codegen, Wails binding parity and desktop architecture checks. The complete TypeScript/lint/i18n checks, **43 Vitest files / 249 tests**, Web and Desktop production bundles and Go lint passed. A standalone **native macOS Wails Go executable** compiled from the latest code; its linker emitted only deployment-target warnings. An initial full canonical `pnpm check` exited 0 before the final empty-root guard; the **fresh final `pnpm check` also exited 0** (receipt `/tmp/mender-phase43-final-check.exit`, log `/tmp/mender-phase43-final-check.log`). The final run includes complete Go tests and Race Detector, SDK/plugin/example tests, generated Wails bindings, Go/ESLint, TypeScript strict, i18n **1252/1252**, Vitest **43 files / 249 tests**, and Web/Desktop production builds. Nonfatal Vite chunk-size warnings and macOS linker deployment-target warnings remain unrelated.

An offline compiled Mender CLI opened a private backup of the live database. **Codex, Grok and OpenCode all reported `ready` with their exact pinned ProviderID and four original ToolIDs**, confirming no user inventory rebind.

The user's running Wails native runtime / OpenAI tunnel remains untouched to protect the established chat connector; the deployed native process cannot be claimed to include the new Go cleanup until a controlled user restart. No Agent Task remained running after the delegated tests. Any post-restart GUI check is a distinct follow-up.

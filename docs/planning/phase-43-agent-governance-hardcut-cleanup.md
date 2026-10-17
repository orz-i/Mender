# Phase 43 — Agent runtime governance hard-cut cleanup

Status: active, 2026-10-17. Documents first. Authorities: ADR-0038, ADR-0047, ADR-0048, ADR-0049.

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

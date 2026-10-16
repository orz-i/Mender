# ADR-0049 — Agent-owned local workspace paths without Sandbox prerequisites

- Status: Accepted (2026-10-16)
- Refines: [ADR-0047](0047-agent-runtime-configuration-workspace-reuse.md), [ADR-0038](0038-governed-agent-delegation.md)
- Scope: Agent runtime *cwd selection*. Does **not** change Sandbox Workspace Tools or sandbox execution.

## Evidence and root cause

The user's Wails screenshot shows key `GAR`, plus a ready local Sandbox `Gaoge-Agent-Runtime`, but a disabled Confirm button. The Agent workspace form only admits `^[a-z0-9_-]{1,64}$`, silently rejecting uppercase input, with no visible validation hint. Live App Support SQLite lists three Agents and ready local Sandbox, but zero rows in both `agent_workspaces` and `agent_workspace_grants`; the presented operation never reached the backend.

Even with a lower-case key, the current Agent workspace design requires a separate **Sandbox** resource before an owner can register an alternative local cwd. This is unnecessary coupling: creating an Agent already accepts a canonical local `workspaceRoot` directly. The Agent backend launches an ACP process in its owner-approved cwd; it does **not** invoke the Sandbox Router, acquire a Sandbox lease, or inherit Sandbox Profile grants. Reusing Sandbox IDs here misstates its authority and imposes an extra management task.

## Decision (hard cut)

1. An Agent workspace is now an **owner-approved alias → canonical local directory path** *bound to that Agent*. It is not a new Sandbox or Tool provider. The existing Agent's `workspaceRoot` remains the implicit default. Add an additional workspace by alias + existing absolute directory; users may type/paste a path or use the Wails native directory picker. No automatic sandbox creation, installation, or permission widening.
2. Only the local owner can register/remove aliases and grant/revoke explicit **Agent ↔ workspace alias ↔ Profile** authority. An AgentGrant alone continues to authorize only the default workspace. Model/Tool arguments may select a registered alias, never an arbitrary cwd, sandbox ID, or new filesystem path. Keep stable AgentID, ProviderID, ToolID and Profile ToolBindings; no Agent loop.
3. Canonicalize the path with `filepath.EvalSymlinks` and require an existing directory on owner registration. Persist only its clean canonical absolute root. On every selected Task admission re-check that the approved root still exists, has not become a symlink target, and is a directory. The launcher independently enforces the same condition before `session/new`. No implicit host/remote fallback. Agent's existing owner-selected `host` / `restricted_host` execution isolation is unchanged and **is not strengthened by having a Sandbox record**.
4. Durable admission must compare **AgentID, alias, exact canonical root and Profile workspace grant** in one atomic SQLite INSERT decision, in addition to the existing AgentGrant, Policy, concurrency budget and Task snapshot. Concurrent unbind/rebind cannot turn approval for one root into permission for a different root. Existing Task snapshots remain immutable through later alias changes.
5. The UI accepts user-typed upper-case aliases by normalizing to lowercase *before validation* (e.g. `GAR` → `gar`), and explains the allowed format. Invalid keys produce a visible explanation, rather than mysteriously disabling Confirm. No forms on inventory pages; workspace editing stays in the focused dialog.
6. **SQLite v9 forward-only hard cut:** rename the existing `agent_workspaces.sandbox_id` column to `workspace_root`. Previously bound Sandbox identities, if any, are converted **once** inside an atomic migration by joining each exact existing local/ready/owner-matching Sandbox and validating its canonical root; preserve alias, timestamps, Agent and Profile workspace grants. If a legacy reference cannot be validated or requires an incompatible owner/isolation, abort startup without destructive changes. If no historical bindings exist, the schema migration requires no Sandbox table. No dual-read, runtime compatibility flag, or second workspace inventory.
7. Update the single owner API (`PUT /agents/{agentId}/workspaces/{key}`) with `{workspaceRoot}` instead of `{sandboxId}`, Wails/SDK/CLI/UI and migrations/tests simultaneously. No backward-compatible aliases. Remove Agent's injected Sandbox resolver and related dead code from Bootstrap; keep Sandbox Workspace Tool multi-target routing **unchanged**.

## Acceptance

- Same live Agent can register `gar` pointing directly to the current `/Users/mini/Coding/Gaoge-Agent-Runtime` **without** registering/linking its existing Sandbox. This must not automatically grant the existing default Profile access.
- A Profile with AgentGrant but no alias grant is rejected before Agent process launch. After explicit grant, an accepted Task snapshots `workspaceKey=gar` and the canonical root; existing ToolID remains. Unbind/rebind, symlink change and nonexistent path fail closed.
- Existing schema v8 → v9 migration preserves all Agent and tool identities. Verify both no-bindings and a populated legacy binding/grant fixture. Production snapshot/backup check before native restart; do not mutate user Agent resources during migration verification.
- HTTP/OpenAPI/SDK/Wails/CLI and React tests pass; `pnpm check` including Go race, i18n, production builds; Wails GUI acceptance after native restart when available.

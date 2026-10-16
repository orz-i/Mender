# Agent Backend Identity UI Hardening — Phase 41 follow-up

Status: confirmed and being repaired (2026-10-16). Scope: UI/UX contract and regression tests; no persisted resource migration.

## Reproduction evidence

The owner attached three real Wails screenshots: an Agent named OpenCode displays `Grok` on its overview and inventory; an attempted second OpenCode creation displays `Codex` on the review screen. Read-only inspection of the live Agent store confirms existing `OpenCode` is stored as `backend=opencode`, `plugin_id=io.mender.agent.opencode`. HTTP/Wails projection already preserves that backend. Two bugs remain in the UI:

1. `AgentsPage` and `AgentDetailPage` render `backend === "codex" ? "Codex" : "Grok"`, mapping **every** newly added backend to Grok.
2. `AgentCreatePage` initializes mutable backend state from `useSearchParams` once, with a silent `codex` fallback, and the marketplace-pinned flow's **Back** button can return to a different/backend-selection flow. React Router retains the same mounted component when only `?pluginId` changes, leaving initial state and draft from a previous plugin.

## Decision and acceptance contract

- Create **one typed, exhaustive Agent Backend presentation registry** for Codex, Grok and OpenCode. Backend names in inventory, detail, selector and review must be a function of the backend identity returned by the SDK, not an unrelated Agent name or binary-detection status. Adding another backend must produce a TypeScript error until it is added to the registry.
- A valid `/agents/new?create=1&pluginId=io.mender.agent.opencode` route is **pinned to OpenCode** from configuring through review and submit. Marketplace entry is authoritative; the page may not silently fall back to Codex or expose another backend as a substitute. A change of `pluginId` while the component is mounted must remount/reset that form's draft and backend. Back from a pinned form returns to the marketplace rather than revealing unrelated backend selection.
- Unknown/non-Agent `pluginId` must fail **closed**, with a direct Back action, not turn into Codex. The general `/agents/new` flow requires an **explicit** backend selection: no hidden default, Continue disabled until chosen.
- No modifications to existing AgentID, ProviderID, ToolID, Task, Profile grants, governance, database schema, plugin installation or process launcher. This fixes presentation/intent invariants without copying Agents, compatibility bridges or changing the live user's OpenCode configuration.
- Add tests for all three backend labels, generic explicit selection, invalid plugin IDs, marketplace-pinned submission and same-mounted-route changes between Codex / Grok / OpenCode. Include negative tests for state/draft retention and pinned-backend escape via Back.
- Document first and commit separately; then fix TypeScript, run targeted tests, `pnpm check`, commit; inspect current Wails frontend using existing Agent and a **non-submitted** OpenCode creation review. Do not create or delete user Agents during acceptance. Avoid `.agents/`, `.pi/`, `.intent-loop/`, `.commandcode/` in commits.

The plugin detail URL already uses `/agents/new?create=1&pluginId=...` with the reviewed plugin ID, and the Backend schema permits only the exact three built-ins. The live store proves the mislabeling is not a migration/data error.

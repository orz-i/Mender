# ADR-0048 — Built-in OpenCode ACP Agent backend

- Status: Accepted for staged implementation
- Date: 2026-10-16
- Refines: [ADR-0038](0038-governed-agent-delegation.md) and [ADR-0047](0047-agent-runtime-configuration-workspace-reuse.md)

## Discovery and motivation

The Mac mini has a local trusted OpenCode v2.0.30 binary at `/opt/homebrew/bin/opencode` (resolved to its Homebrew Cellar executable). `opencode acp` is a supported native ACP stdio subprocess. An isolated no-prompt, no-credential-logging test against that exact binary completed ACP v1 `initialize` and `session/new`, reporting Agent `OpenCode`, 44 model choices, a `model` selector, `effort` in category `thought_level` with choices `low/medium/high/xhigh/max/default`, and an independent `mode` selector (`build/plan`). This establishes the *protocol integration surface*, not that every model is authenticated or inference is guaranteed.

The prior Agent backend domain and `agents.backend` SQLite CHECK allow only `codex` and `grok`. The plugin inventory, marketplace allowlist, creation UI, CLI help, OpenAPI and Wails projection share that two-backend restriction. Duplicating Agent Delegation Tools or creating a new Provider type would be a wrong fix.

Official OpenCode documentation: https://dev.opencode.ai/v2/docs/cli/acp/ (ACP v1 stdio and model/effort/mode session config).

## Decision

1. Introduce a single built-in Agent backend `opencode` with exact plugin ID `io.mender.agent.opencode`. Register it in the *existing* built-in Agent catalogue and approved marketplace Agent-create allowlist. The owner can create a governed Agent with its own AgentID; it gets the **same existing Agent Delegation suite** (`delegate/status/result/cancel`) and the same Principal/Profile/Policy admission and task recovery behavior as Codex/Grok.
2. The host-reviewed LocalFactory resolves only `opencode` in the trusted executable search locations. Launch exactly `opencode acp` with canonical selected local working directory and the current host/restricted_host process isolation. No shell, `--auto`, arbitrary executable, endpoint, user-provided flags, inherited PATH or implicit relaxation of isolation. Availability lookup is inert and does not contact user accounts.
3. Use Mender's existing ACP v1 handshake/session and live `model` and category `thought_level` choices; the OpenCode `effort` option must be applied by the **ID actually advertised** after model selection, not hard-coded as Grok's `reasoning_effort`. The OpenCode `mode` selector is Agent-owned and stays at the CLI's configured default for this phase. No implementation of an Agent Loop, multi-session persistence, OpenCode API backend, or Model provider. Do not enable external tools or approve ACP client permission requests implicitly.
4. Add a **v8 forward-only Agent schema** allowing `opencode` while retaining `CHECK(backend IN ('codex','grok','opencode'))`. SQLite cannot directly widen this CHECK. Perform an atomic, FK-safe controlled table rebuild on the Agent store connection: verify existing live FK and trigger inventory, disable FKs only for the protected migration transaction, copy every persisted Agent column, replace and rename the new table, reinstall reviewed Agent→Provider deletion guard, check FK integrity, commit, restore FKs. Fail closed on any mismatch; preserve existing AgentID, TaskID, Grants, workspaces and pinned ProviderID/ToolID. No data wipe, writable_schema modification or runtime compatibility bridge.
5. Maintain OpenAPI/SDK/Wails/React/CLI representation and existing two default-valued runtime selectors. Marketplace must provide the direct create action only when OpenCode binary is available. Keep backend model/effort discovery explicitly user initiated; never issue a paid prompt during listing or creation.
6. Do not create a configured OpenCode Agent in the user's database automatically. The owner must select its local workspace and isolation mode and decide whether to publish/authorize tools. The backend implementation is a built-in capability, not a connection to a separate LLM API account.

## Acceptance gates

- Document-first commit before implementation.
- New backend/domain binding and SQLite v7→v8 fixtures with protected FK migrations (including existing Agent, historical Task, Profile grants and both managed Provider identity and workspace references); fresh DB must also reach v8. An unknown backend/plugin ID must still fail.
- Passive binary discovery, explicit launcher command argv and no-process-on-list tests; a local ACP handshake/session probe that emits only protocol metadata and no model prompt. Synthetic OpenCode ACP `effort` option must route and confirm without hard-coded ID.
- Catalogue and marketplace entry/action, Create Agent UI and API/Wails enum parity, SDK/Wails generated contract and bilingual copy; production Agent count/grant identities unchanged.
- `pnpm check` including Go race, lint, TypeScript, frontend tests, SDK and Wails generation checks. A restarted Wails UI smoke test is distinct from automated acceptance; real OpenCode paid inference requires separate operator approval.

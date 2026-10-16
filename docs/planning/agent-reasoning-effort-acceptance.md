# Agent Reasoning Effort — Phase 40 acceptance follow-up

Status: active (2026-10-16); refines [ADR-0047](../architecture/adr/0047-agent-runtime-configuration-workspace-reuse.md). No new Agent identity, Provider, Tool or runtime.

## Observed gap

Real local Grok ACP v1 `session/new` advertises `configOptions` selector with `id=reasoning_effort`, `category=thought_level`, `type=select` and values `xhigh/high/medium/low`. Mender v6 persisted and showed only `defaultModel`, and `agent.delegate` submitted only prompt/model/workspace. **A user's inability to choose a thinking level is a missing Mender selection contract, not evidence the installed Grok CLI does not support it.**

## Contract and limits

1. Add one optional **defaultReasoningEffort** to the existing Agent resource; add per-call **reasoningEffort** to the existing canonical `agent.delegate` input. The empty value means **use the Backend's own default**; this is not another conceptual `Agent` or `Profile` resource. Model, reasoning effort and workspace retain independent defaults and task-level overrides.
2. Store effective `model`, `reasoningEffort`, workspace root and alias as one **immutable admitted Task snapshot**. Validate the effort identifier before admission; use only exact advertised ACP `thought_level` / `reasoning_effort` selector choices and confirmation from `session/set_config_option`. An unsupported or rejected selection must **fail closed before session/prompt**. When a model is selected first, check effort against the resulting updated `configOptions`; do not trust stale option lists.
3. An owner-only, explicit no-prompt Agent capability inspection already exists. Extend its bounded response with an optional `reasoningEffort` select group. It remains inert on GET/list, and does not mutate the Agent, store credentials, or consume a model prompt. Backend options vary; **do not hardcode the four Grok values as a universal enumeration**.
4. The Agent overview uses one focused **Runtime Defaults** edit dialog with two selects (Model and Reasoning Effort) when advertised; hide/disable an unsupported effort selector with a clear explanation. The dialog updates both defaults **atomically** in one owner-authorized operation. Existing empty/default settings survive the forward-only migration.
5. Update HTTP/OpenAPI, SDK, Wails, CLI, UI/i18n, Tool schema, SQLite tests and ACP synthetic fixture; no compatibility dual-write or legacy bridge. Preserve existing AgentID, ProviderID, ToolIDs and Profile grants. No adjustment to Agent permissions/host isolation/credential handling and no new Agent loop.
6. Do not mutate the user's production Grok defaults or launch a real paid prompt without an explicit separate acceptance step. Real no-prompt ACP inspection and `set_config_option` are allowed; synthetic ACP covers prompt dispatch.

## Gates and commits

- **A docs**: contract + preservation/fail-closed requirements; commit first.
- **B backend**: v7 forward-only SQLite migration, typed defaults / task snapshots, Tool schema and ACP option handling. Ensure old DB + Task history survive.
- **C surfaces**: owner-only settings API, generated OpenAPI/SDK/Wails bindings, focus dialog with live effort dropdown and CLI; update tests/i18n.
- **D acceptance**: `pnpm check`, SDK/Wails parity, synthetic multi-select ACP fixtures, Profile negative tests, explicit no-prompt Grok inspection and read-only production DB integrity. After user app restart, macOS native GUI smoke remains separately reported.

## Safety check

Unsupported Agent backend: leave reasoning default empty and do not offer a fake fixed choice. Prompt-supplied values select only ACP option IDs, not CLI flags or executable arguments. Failed setting does not change accepted Task identity, approval, sandbox access or Provider configuration.
## Implementation and acceptance evidence

- `df77ece` — document-first contract and limits.
- `13d8d05` — v7 forward-only Agent/Task schema, reasoning defaults, immutable task snapshot, ACP advertised selector and confirmation, optional delegation argument. Synthetic tests include wrong categories, unsupported values, unconfirmed choices and updated options after model selection. The v6→v7 fixture preserves existing AgentID, historical Task and AgentGrant.
- `7c163dc` — atomic owner defaults update and live-model/effort inspection in HTTP/OpenAPI, SDK, Wails, CLI and focused React dialog. Existing Agent display and Task detail expose effective effort, when present. No automatic CLI launch during list/read.
- `ed903b2` — explicit refresh of already-ready Agent Delegation Tool Suite schemas, preserving original ProviderID, ToolIDs, existing Profile ToolBinding and AgentGrant. No automatic profile permission or new Provider.
- Local Grok ACP v1 acceptance (**no prompt sent**) started a temporary session, selected `grok-4.5`, then requested and confirmed `reasoning_effort=high`. After that model selection, the session advertised `high`, `medium`, `low`, demonstrating that available reasoning choices are model-dependent; Mender uses the updated response to authorize the second choice.
- Initial targeted acceptance: Agent + Bootstrap + management + desktop + CLI Go suites, TypeScript strict checks, 234 frontend tests, Go and frontend lint and Web/Desktop production builds passed. The additional ready-facade refresh test and 235th frontend test passed. Full canonical `pnpm check` and native GUI operator verification are recorded after their respective gates finish.

## Final automated and native acceptance (2026-10-16)

**Automated gate:** the final canonical `pnpm check` completed with **exit code 0** (receipt `/tmp/mender-agent-effort-final.exit`; log `/tmp/mender-agent-effort-final.log`). It included protocol generation, SDK examples and race tests, `go test ./backend/...`, `go test -race ./backend/...`, Go lint, TypeScript, ESLint, i18n, **42 Vitest files / 235 tests**, SDK and Wails generation parity and Web/Desktop production builds. One non-failing Vite large-chunk advisory remains unrelated.

**Database/rebuild:** native Wails dev client was started from the Phase 40 reasoning changes and rebuilt at local Mac time **22:07:58**. It remained connected to its Vite development server (HTTP 200 on port 9245). Production App Support SQLite migrated to **Agent schema v7**. A private temporary SQLite backup was taken in advance, with active Agent Task count and Sandbox execution lease count both zero. The preserved resource counts were **2 Agents, 2 historical Tasks, 1 AgentGrant, 2 Agent Tool facades, 7 Sandboxes**. Grok kept its existing `grok-4.7` default model and empty default reasoning effort. No local file/Tool/Sandbox permission was changed during model inspection.

**Native GUI:** macOS accessibility navigation opened the existing **Grok - executor → Overview → Edit runtime defaults** dialog. The real Wails WebKit model control showed `Grok 4.7 (grok-4.7)` and the new reasoning control showed **Backend default high**. Opening its menu showed four authenticated ACP options: **Extra High (xhigh), High (high), Medium (medium), Low (low)**. The dialog was **canceled without saving**, so persisted Grok defaults are unchanged.

**Existing Tool source:** before refresh, the Grok `agent.delegate` Tool Schema lacked `reasoningEffort`. The new **Agent → Tools → Refresh Agent tools** action was clicked in native Wails. Read-only database comparison verified the same AgentID, ProviderID, all four original ToolIDs, same default model and effort, the original AgentGrant and the existing Profile ToolBinding. The refreshed `agent.delegate` schema now advertises exactly `prompt`, `model`, `reasoningEffort`, `workspace`. No additional Provider, Agent, Tool or Profile grant was created.

**Real ACP no-prompt verification:** a separate short-lived Grok ACP v1 session in a temporary workspace selected `grok-4.5` first and then `reasoning_effort=high`; the CLI confirmed both. The post-model options contained `high/medium/low`, validating why model-specific effort lists must be checked at dispatch. No inference prompt was submitted for this acceptance. Browser/GUI work and managed service were left running; the owner may now change defaults explicitly in the UI if desired.

This closes the requested reasoning-effort selector feature and tool-schema refresh. It does **not** claim live billable inference with every model/effort combination.

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

# Phase 41 — OpenCode native ACP Agent backend

- Status: implementation complete; native Wails smoke and no-prompt ACP acceptance passed (2026-10-16).
- Authority: [ADR-0048](../architecture/adr/0048-opencode-native-acp-agent-backend.md).

## Intent contract

**Goal:** the local OpenCode CLI is a first-class built-in Agent Backend. Its marketplace card can open the Agent-specific creation flow. An owner may select it once, and use model/effort/workspace defaults or task overrides under the existing Agent Delegation and Profile governance. Nothing in this change constructs an Agent planning/inference loop.

**Evidence:** local `opencode v2.0.30` and explicit `opencode acp` confirmed. Safe test-created ACP v1 Session offered 44 models, thought level as selector ID `effort`, and mode options; no prompts sent.

**Non-goals:** automatic login, creating a user Agent without its workspace/isolation selection, synthetic model list or universal thinking levels, managing OpenCode background service/API, direct remote Agent execution, implicit ACP permissions, hidden compatibility adapters.

**Preservation:** hard-cut to v8 schema; preserve all historical and active identities; do not touch `.agents/`, `.pi/`, `.intent-loop/`, `.commandcode/`. No paid external prompt or modification of the live production Agent inventory for tests.

## Sequential checkpoints

| Commit | Scope | Gate |
|---|---|---|
| A | ADR-0048 and phase contract | Docs before implementation |
| B | Domain/plugin ID, bounded executable launcher and forward-only v8 table constraint migration | Go domain/Agent store/launcher tests, FK and identity snapshots |
| C | Marketplace, desktop/UI, OpenAPI/SDK and CLI surface | Catalogue/creation/permissions tests, i18n/typecheck and generated binding parity |
| D | Integration & close | Real local ACP v1 handshake/session and inspection (no prompt), fixture task path, `pnpm check`, documentation and clean tracked git status |

Do not treat a partial check, detected executable or available model inventory as proof of real model entitlement. If client capability requests are not supported, continue failing them closed.

## Completion evidence (2026-10-16)

- **Docs-first:** `6b03d47` fixed the backend, protocol, isolation and migration contract.
- **Backend/migration:** `74140d9` introduced `opencode` / `io.mender.agent.opencode`, trusted `opencode acp` command, and schema **Agent v8** with atomic table CHECK-constraint rebuild. Synthetic historical Agent+Task, Grant, Workspace, pinned Provider and Profile ToolBinding fixture and unknown-trigger fail-closed test passed.
- **Surfaces:** `480521f` added the built-in marketplace Agent-create action, Wails/SDK/OpenAPI/CLI/React representation, dynamic ACP selection fixtures and owner-only creation tests. `9fe1c44` added an OpenCode-specific warning about restricted-host credential/config access.
- **Real OpenCode ACP (no prompt):** local `opencode v2.0.30` completed ACP v1 `initialize` + `session/new` in an isolated temporary workspace. Default model `opencode/ll-alpha-free`, **44 advertised models**, and `effort` (`thought_level`) choices `low/medium/high/xhigh/max/default` were observed. Explicit `session/set_config_option` for the current model and `effort=medium` was confirmed. `mode` choices `build/plan` remain OpenCode-owned.
- **Isolated Mender CLI:** an OpenCode Agent created in a new test database using explicit `host` isolation; `mender agent models` returned the same 44 models and six effort choices in about two seconds; no Agent Task or Profile Grant appeared. On this Mac, explicit inspection in `restricted_host` returned Backend unavailable, therefore the UI warns and **never silently falls back** to unisolated execution. Model account entitlement and paid prompt execution are not inferred from successful inspection.
- **Full quality gate:** the final `pnpm check` returned **exit code 0**, receipt `/tmp/mender-phase41-final-check.exit`; includes full Go race/tests, plugin SDKs/examples, OpenAPI generation, Wails binding drift checks, TypeScript, i18n **1245/1245**, 42 Vitest files / **236 tests**, lint and Web/Desktop builds.
- **Offline production snapshot:** a private read-only SQLite backup of the actual pre-deploy v7 database was migrated to v8 with the real Go Agent Store. SQLite `quick_check` was `ok`, foreign key check reported zero violations and FK enforcement remained enabled. Row fingerprints of **11 inventory tables** matched exactly before/after migration, including Agent, Task, Grantee, 21 Profile ToolBindings, 33 Tools and 10 Provider configurations. The backed-up test never opened runtime tunnels or modified the user's live DB.
- **Wails live deploy:** controlled Wails dev rebuild/restart at Mac-local **22:56:18** opened the original Application Support DB and migrated it to **Agent v8**. The original **2 Agents, 2 historical Tasks, 1 AgentGrant, 2 Agent facades, 21 Profile ToolBindings and 33 Tools** remained unchanged, with no active Tasks. SQLite `quick_check=ok`, `foreign_key_check=0`, and the frontend was HTTP 200. Native macOS WebKit Accessibility navigation found **Plugin Marketplace → OpenCode Agent (Enabled) → Add agent**, opening its correct configured OpenCode-specific creation form with the explicit isolation warning. The owner has **not** been automatically assigned a new configured Agent; production OpenCode resource count remains zero.
- **Scope:** no new Provider class, duplicate Agent Tool suite, automatic auth, paid prompt, implicit filesystem permission or remote Agent process. This implementation intentionally does not expose OpenCode's `mode` selector; its own default applies.

Operator follow-up: use OpenCode's marketplace card to create one configured Agent with a chosen workspace and explicitly accepted isolation, then grant only the required Profile capabilities. Real inference remains subject to OpenCode's configured accounts/model entitlement and the selected sandbox/host isolation.

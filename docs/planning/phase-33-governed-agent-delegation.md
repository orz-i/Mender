# Phase 33 — Governed Agent CLI Delegation

Status: **Phase 33 core implementation closed; synthetic ACP/MCP acceptance verified** (2026-10-11). Architecture: [ADR-0038](../architecture/adr/0038-governed-agent-delegation.md). Operator guide: [Agent CLI Delegation](../development/agent-delegation.md).

## Intent Contract

**Outcome:** Existing CLI Agents such as Codex and Grok can be registered as typed, governed Mender resources. An authorized upstream Profile can submit a bounded task through ordinary Mender Tools, receive an opaque TaskID, query/cancel its state and fetch its verified result. The subordinate Agent keeps its own loop. Mender does not become an Agent harness.

**Success:** (SC1) exact installed backend identity and typed create/inspect; (SC2) no process/credential resolution on discovery or forbidden call; (SC3) persisted task state, bounded output, truthful cancellation/recovery; (SC4) Profile membership, Principal, agent delegation and Policy authorization at dispatch and Task access; (SC5) local ACP/CLI fixture-based execution and negative cases; (SC6) CLI, HTTP, SDK, Wails/Desktop, Web parity with focused create/details UX; (SC7) repository checks and isolated end-to-end test.

**Invariants:** preserve existing Providers/Tools/Profile IDs, Sandbox grants, Tunnels, installed plugin pins and current service exposure; never launch the CLI by merely showing a card; no automatic Agent installation, real account access, outbound cloud charges, approval bypass, host fallback or user workspace modification. Keep `.agents/`, `.pi/`, `.intent-loop/` untouched and out of Git commits. Go DDD/clean architecture, strict TS, React/shadcn and one category of information per viewport. Migrate forward only, no shadow inventory or long-lived compatibility code.

**Autonomy:** implement contract/testing/focused UX inside Mender; do not authorize credentials, change host security policy, perform real Agent work or access remote accounts without explicit user instruction. Missing local `codex-acp` is an availability diagnostic, not permission to install it.

**Deferred:** distributed orchestration, A2A, automatic model selection, recursive tool exposure, remote Agent Process on Docker/SSH, MCP Tasks advertisement without actual spec support.

## Stage A — Decision and protocol fixtures (separate commit)

- ADR-0038, this acceptance contract and staged work graph before code.
- Verify actual Grok stdio and Codex adapter availability without authenticating, running a prompt or editing user files.
- Record ACP fixture requirements: initialize/session/new/session/prompt/session/update, permission rejection, cancellation and stdout framing.
- **Gate:** accepted ADR, high-risk invariants and independently verifiable stage checks. Commit only docs.

## Stage B — Agent domain and owned execution (separate commits)

- Introduce `agent.backend` in typed plugin platform with exact descriptor semantics, inert catalogue and pinned installation conflict checking. Add Agent and AgentTask application/domain infrastructure, SQLite forward-only store, status transitions/recovery fencing, and owner-restricted CRUD.
- Provide deterministic local ACP stdio adapter, bounded stdout/stderr/event handling, capability negotiation, refusal of unsolicited approval/file/network operations, process cancellation, and Codex/Grok launch plans resolved only from host-reviewed binaries. No external Agent packages until typed process host is verified.
- Add `agent.activate`, `agent.task.submit`, `agent.task.cancel`, distinct Runtime budgets and launch class. Same process isolation guarantees as the existing classes, with explicit local-only execution; no SandboxID chosen by Tool arguments.
- **Gate:** deterministic fake Agent tests cover success, rejected permissions, invalid protocol, timeout, cancellation, restart/unknown, executable pin and no binary on read. No real account needed.

## Stage C — Delegation Provider and trusted access (separate commits)

- Add host-owned Agent Delegation Provider which exports stable task-related Tools into canonical Catalog and Profile. User/model arguments contain task content and safe options only. Submission returns a durable ID quickly; later task access is re-authorized.
- Profile-level AgentGrant is owner-managed with exact ProfileID+AgentID binding; `tool.invoke` and `agent.task.submit` checks both apply. Task status/result/cancel must verify caller scope and ownership; do not rely on opaque ID secrecy.
- MCP Tasks exposure is contingent on actual transport implementation and negotiation; initial standard Tool polling remains valid. No false capability advertisement.
- **Gate:** denied Profile call produces zero agent/process side effects, permitted Profile dispatch succeeds, cross-Profile task ID fails, revocation prevents new tasks, and Audit remains minimized.

## Stage D — Typed management and closure (separate commits)

- Extend HTTP/OpenAPI, SDK and Wails/Desktop management with Agent list/detail/create/access/tasks/cancel. Add focused UI under Runtime, Marketplace Agent filter and exact Add route, localized labels and diagnostic states.
- Probe executable/version without running a model or performing login. Require explicit selection and review before activation.
- Run Go unit/race/lint, `pnpm check`, frontend DOM tests and isolated local end-to-end fixture; browser screenshot + packaged desktop QA remain recorded if tooling is unavailable.
- **Gate:** one authorized upstream client can submit to a fixture Agent, view terminal result and cancel a separate fixture task; no real Codex/Grok prompt required.

## Stage discipline and evidence

A→B→C→D are bounded packages with commits at each completed, tested boundary (no A1–A100 proliferation). Failed gates are documented, not relabelled as success. Leave runnable small implementation increments; incomplete or unsafe integrations remain unavailable. Do not commit generated secrets, session transcripts, runtime databases or dot-skill directories.

### Stage log

- **A:** `10bc3b7` ADR/contract committed before implementation.
- **B:** `9d8fda7`, `feba53a`, `937484b` establish typed Backend inventory, Agent/Task domain, explicit local ACP v1 launcher, durable Tasks and task budgets, fail-closed Profile Grants, generation-fenced recovery and immutable, minimized admission/grant events. Fixture tests exercise success, unapproved permission requests, unknown client requests, invalid/oversized frames, cancellation, timeout and shutdown semantics. ACP request-permission termination/process-group cleanup is included in the final focused gate.
- **C:** `3b4d358` and `937484b` expose one pinned Agent through the normal Catalog/Invocation/Policy pipeline, including Profile ToolBindings, Principal Scope, explicit AgentGrant and owner-only task access. Test fixture covers missing/revoked grants, cross-Profile TaskID isolation, Policy denial and Audit. A **real MCP SDK client session** additionally confirms `tools/list` → `tools/call` → TaskID → result polling and revocation using a synthetic executor (`381e4de`).
- **D:** `fd52e5c`, `123ce99` and `8b13530` implement OpenAPI, TypeScript SDK, Wails/Desktop bindings, Go CLI, localized Agents page, focused Add/Detail/Tasks/Access/Activity, Marketplace Agent type routing, and bound Delegation Provider creation. `381e4de` closes unauthorized ACP client requests and exercises the Profile through an actual MCP client session. The automated suite includes CLI CRUD/grant/audit, HTTP owner management, Wails generation, i18n and React UI tests. CLI discovery checks file existence **without executing version/login/model**; actual CLI protocol/version/auth checks and packaged GUI screenshots need a separate explicit operator test and must not be described as passed.

## Closure evidence and remaining operator verification

- **Automated gate:** `pnpm check` exited **0** on the Phase 33 worktree. Provider/Sandbox/Tunnel SDK and protocol generation, Go module isolation, SDK generation, Wails binding regeneration, desktop architecture, Go lint, all Go tests and race tests, ESLint, TypeScript, i18n, Vitest, Web and desktop frontend builds passed.
- **Protocol evidence:** `TestAgentDelegationThroughMCPClientSession` verifies an actual in-memory MCP SDK client/server exchange with Tool discovery, denied ungranted call, successful delegated fixture TaskID, result polling, and rejection after ProfileGrant revocation. `TestACPFailClosedOnClientPermissionAndFilesystemRequests` ensures unsolicited approval/client filesystem requests do not produce successful Task outcomes. No real CLI prompt, account or external API was accessed.
- **Recovery and accounting:** deterministic fixtures cover policy-denied zero side effects, cross-Profile TaskID isolation, bounded input/output, atomic global/Principal/Agent concurrent Task admission, cancelled versus unknown outcomes, restart fencing, Agent reference safety and immutable data-minimized Audit.
- **Operator-only follow-up:** Codex CLI requires a separately installed `codex-acp` adapter; Grok's built-in adapter plans native `grok agent stdio`. Actual credentials, runtime sandbox compatibility, paid model access, version handshake against live installed CLIs, and packaged desktop screenshot smoke are **not verified** in Phase 33. They must not be inferred from passive binary detection or fixture success. No automatic installation or model charges occurred.

**Explicitly not claimed:** an independent Mender Agent planning loop, persistent/recoverable ACP conversations, native approval continuation, full MCP Tasks extension advertisement, A2A, recursively exposed Mender tools, or Agent process placement on Docker/SSH. These require separate design and admission gates, not an unbounded Phase 33 follow-up.

## Desktop dev Marketplace catalogue contract regression (2026-10-12)

**Observed:** After the Phase 33 Agent Backend addition, the Wails development client's Marketplace reports that the desktop returned incompatible Mender data and shows an empty result. This is an adapter-validation regression, **not** a reason to delete or migrate the user's database.

**Diagnosis:** Go Runtime `ListPluginCatalogue` correctly emits `agent.backend` for the built-in Codex/Grok Agent extensions. Wails `Service.ListPluginCatalogue` forwards `kind` unchanged, and the shared Marketplace UI already understands `agent.backend`. However, `apps/desktop/frontend/src/desktop-client.ts` `pluginCatalogueEntry` still validates only the pre-Phase-33 `provider.tools`, `sandbox.backend`, `tunnel.transport` enum. The first Agent entry throws a `MenderClientInvariantError`, aborting the entire list.

**Repair contract:** Update only the intended Wails desktop adapter kind boundary to include `agent.backend` (no permissive unknown-kind fallback). Add a desktop adapter regression test exercising both available and unavailable Agent Backends alongside Provider/Sandbox/Tunnel and planned entries; preserve rejection of an actually unknown extension kind/status and malformed planned metadata. Confirm shared Marketplace conversion still creates `/agents/new` only for enabled built-in Agents and leaves unavailable/unsupported external Agent drivers inert. Run desktop/frontend tests, full `pnpm check`, and confirm that the live Wails dev frontend can reload the new adapter without clearing DB or restarting user processes.

**Guardrail:** This is a focused bug fix; do not add an Agent Runtime/compatibility bridge or change plugin storage, permissions, install semantics, or task execution.

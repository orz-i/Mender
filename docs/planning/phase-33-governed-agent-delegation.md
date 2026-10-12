# Phase 33 — Governed Agent CLI Delegation

Status: **In progress (2026-10-11)**. Architecture: [ADR-0038](../architecture/adr/0038-governed-agent-delegation.md).

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
- **B:** Core persistence and stdio runner committed as `9d8fda7`; local launcher, Resource/Policy integration and task service are in progress. Full stage-B exit gates remain open.
- **C:** Not started.
- **D:** Not started.

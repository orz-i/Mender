# Phase 34 — Marketplace offerings, managed facades and tool-suite governance

Status: **Active (2026-10-12)**. Architecture decision: [ADR-0039](../architecture/adr/0039-offerings-resource-tool-suites.md).

## Intent Contract

**Outcome:** Users configure meaningful resources/integrations (e.g. two distinct Grok Agents, AnySearch Remote MCP, a Browser Session) without manually layering generic Provider forms. Internally canonical Tool identity, exact source/target, policy and execution boundaries remain correct.

**Success:**
- SC1: the five-layer decision and resource ownership rules exist as committed architecture/acceptance docs before code.
- SC2: a newly created Agent has an appropriately named, separately identifiable, host-managed delegation tool facade; existing unique hand-created facades remain bound and usable without ToolID or MCP export-name churn. Multiple Agents cannot accidentally dispatch to one another. Agent read/list remains side-effect free.
- SC3: the normal UI shows each Agent's tools and Profile access on the Agent resource and no longer encourages manual `Agent Delegation Tools` Provider creation; recovery and conflict states remain actionable.
- SC4: an AnySearch marketplace offering pre-fills its supported MCP URL/transport and asks only for missing non-secret config and optional Credential Reference; custom MCP paths continue working; no false online/authorized status or automatic tool publication.
- SC5: Workspace/Browser tool-suite contracts are typed, capability-negotiated and enforced. No Browser facade is advertised as operational before a verified backend/fixture. Supported native MCP browser tools need no Sandbox wrapper.
- SC6: Go + TS contract/API/Desktop parity, relevant negative and regression tests, full quality gates; bounded commits with no credentials or user resources touched.

**Invariants:** Existing Provider/Tool/Agent/Sandbox IDs, Profile ToolBindings, aliases, namespaces and Policies remain stable. `tool.invoke`/Provider Activation, AgentGrant, Sandbox admission, Runtime Budget, Principal, secret resolution, result guard and Audit are never bypassed. No silent provider rebinding/deleting, duplicate facade, host fallback, process launch or network probe from list/read, no new Agent loop. Preserve `.agents/`, `.pi/`, `.intent-loop/` untracked and untouched. Follow DDD/clean architecture, strict TS, focused views, bilingual i18n, one category of information per viewport.

**Autonomy:** The implementation may choose private host-owned binding records, typed safe APIs and reusable UI components. No user real Agent task/remote account, Docker provisioning, native plugin install, browser billing, Credentials activation or destructive migration. Missing external browser adapter means capability model + explicitly unavailable UI/fixtures, not false readiness.

## Stage A — decision and contracts (docs-only commit)

- Record prior source/UX diagnosis, accepted ADR-0039, this intent contract and per-stage acceptance checks.
- Draw exact distinction between reviewed offering, installed extension, configured resource, suite binding and Profile exposure.
- Record current Agent Provider/Tool stable identity, unique-adoption/conflict rules, no side effects on reads and forward-only hard-cut constraints.
- **Gate:** docs committed before any code; no protected dot-skill or runtime files staged.

## Stage B — Agent-backed managed tools and focused ownership

- Add a first-class host-owned Agent→Agent Delegation facade lifecycle, exact association/uniqueness and create/ensure workflow on explicitly authorized mutation. Existing uniquely bound Provider/Tool identities remain unchanged. Multiple existing Providers are visible conflicts, not silently merged.
- Keep ordinary Provider/Tool/Policy interfaces working internally; move management presentation toward resource ownership, with Agent-specific tools overview and Profile bindings. An Agent may be created with tools present but **no Profile grant or binding**.
- Preserve deletion guards and add regression tests for idempotency, concurrent duplication, denied grants, restart/backfill, existing Provider identities and MCP projection from two Agents. Avoid automatic repairs during listing/startup.
- Validate Go focused tests / race, HTTP + Wails/Desktop/SDK contract parity, TS UI and i18n; commit backend/binding and UI independently when verified.

## Stage C — integration marketplace and Remote MCP presets

- Add immutable, reviewed *Integration Definition* metadata in a catalog separate from native installation inventory. Initial curated AnySearch preset with published Streamable HTTP endpoint and optional API credential. No downloaded executable, hidden inline secrets or guessed Tool list.
- Marketplace shows user-facing integration cards/actions and leaves native extension management in an Advanced view; exact Connect action opens existing typed MCP creation prefilled with audited endpoint/transport, no arbitrary field injection.
- Verify raw custom MCP connection still works, preset config is sealed unless explicitly allowed, endpoints/Schemas are never probed or authorized from list reads, credentials are references, failures are visible. Contract + HTTP/SDK/Wails parity as needed.

## Stage D — Tool Suite resource binding and capability closure

- Define a first-class host-owned Tool Suite definition/binding, exact resource identity and capability/admission check. Adapt existing Agent Delegation/Workspace facades without altering ToolIDs.
- Introduce a separately typed Browser Session capability boundary and tool-schema fixture only where execution/lease semantics are real. A browser-only environment cannot acquire `workspace.read/search/edit/shell`. If no real managed-browser backend exists, ship only a reviewed unavailable contract/fixture; do not show operational Add or fake security status.
- Maintain API/CLI/MCP parity and regression tests: wrong-suite binding, cross-resource task ID, unauthorized session, no host fallback, no unintended Profile exposure, cancellation/unknown execution and no read-time remote effects.

## Stage closure discipline

Stages A→B→C→D are **four bounded outcomes**, not an endlessly nested phase tree. Commit documentation first, then bounded tested backend/data, then focused UI/integration as independently reviewable commits. Record exact hashes and test evidence here after each gate. Full `pnpm check` and `pnpm security:check` before closing the entire phase; any not-run manual packaged Wails or remote provider checks remain explicitly outstanding.

### Execution log

- A: pending
- B: pending
- C: pending
- D: pending

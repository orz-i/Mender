# Phase 47 — Post-governance hard-cut cleanup and real Agent acceptance

Status: Audit in progress (2026-10-17). Scope: one bounded cleanup round, not a new Agent Harness. Authority: ADR-0052 and Phase 46 accepted behavior.

## Goal and non-negotiable boundary

Remove provably dead code, obsolete runtime compatibility branches, redundant implementations, and *verified orphaned or incompatible* historical state following the Phase 43–46 Agent governance hard cut. Keep Mender's job as a tool/resource aggregator, governance plane and supplier to independently running Agents. No Mender-owned Agent Loop.

**Protect production:** currently the live Wails app owns `~/Library/Application Support/Mender/mender.db`. Agent schema = v11; 3 existing Agents, 13 historical Tasks, 2 AgentGrants, 0 Tool placements; SQLite `quick_check=ok`, no foreign-key violations. These facts do not authorize bulk deletion. Historical schema-migration receipts and old Task history are **not** intrinsically incompatible data. Preserve all Agent/Provider/Tool/Profile/Sandbox/Task IDs and grants unless a row-by-row owner-reviewed deletion manifest identifies an actual invalid record. No clearing SQLite tables just to make the runtime pass. Create a private consistent backup and before/after row-count/fingerprint ledger before any production data change. Never copy or disclose secrets.

**Protect code and UI:** preserve `.agents/`, `.pi/`, `.intent-loop/`, `.commandcode/`; don't stage them. DDD/Clean Architecture, canonical Policy/Invocation/Sandbox/Task boundaries, owner-only controls, fail-closed ACP, no Host fallback, no broadened Seatbelt, no new compatibility bridge. Keep migration support if needed by a valid clean install or recovery; removing it requires an explicit new baseline + fixtures and a hard validation gate, not a grep-based guess.

## A — Read-only evidence / real delegation

1. Record git status, schema versions, current joins and integrity, stale references, runtime call sites, candidate files and tests.
2. Distribute two **real** bounded code-review tasks using Mender-exposed OpenCode and Grok Agent `delegate → status/result`, after verifying provider→AgentID bindings. Existing registrations are explicitly `host` and **do not imply** Phase 46's restricted governed-tool assurance. Such tasks are analysis-only, never authorize code/shell writes. If ACP asks for native access and the owner doesn't approve, record failure. Never promote those to governed execution by automatic Host fallback.
3. Independently verify findings in the local repository; Agent prose is evidence of a delegation result, not proof of file modification.

## B — Hard-cut implementation (only proven candidates)

Document each removal with: symbol/path, old purpose, modern owner, all references, migration/test dependency, expected behavior change and rollback/test evidence. Perform targeted, small code edits, do not add adapters or retain two paths. Scope any data deletion to explicit confirmed invalid rows and backup/foreign-key validation. No silent retirement of legitimate historical Tasks.

## C — Acceptance

- Stable Tool/Provider/Agent/Profile identities; 4 delegated Agent facade Tools per Agent; no new global tools from cleanup.
- Run focused regression first, then canonical `pnpm check` including Go Race Detector, frontend, i18n and Wails binding/architecture parity.
- Real scenario: both Agent delegations produce observable Task IDs and terminal outcomes or precisely classified fail-closed errors; optionally exercise positive restricted OpenCode disposable Read/Edit using opt-in test, and negative Grok restricted path, without changing the user's source tree.
- State what was cleaned, what remains required, what is unsupported, production DB side effects, and exact commits. Close this phase without an expanding stage taxonomy.

## Live inventory at entry

- Branch `main`; HEAD `e1aad3d`, ahead of `origin/main`; only Skill/agent environment directories untracked.
- Wails dev process and tunnel-client running after user restart.
- Production SQLite Agent v11, 3 Agents (Codex/Grok/OpenCode, all configured in explicit Host mode), 13 Tasks, 2 AgentGrants, 0 placements, 1 additional Agent workspace. Current profile count 1; Provider count 11; Catalog Tool count 37. No foreign-key violations or SQLite corruption.
- Mender profile exposes **two real Agent delegation Tool suites**; provider `prov_0ee3ba44c2f8f3d3d444a4b9001ff9b4` targets OpenCode and `prov_1633739d42de01f3d438ce26b2e9f99c` targets Grok. Use their exact existing delegate/status/result Tools.
- Agent schema migration receipts include versions 1..11. They are compatible history, not proof of orphaned live data.

## Checkpoints

- [ ] Documentation and read-only manifest committed
- [ ] OpenCode/Grok real Agent task receipts and independent findings
- [ ] Targeted code/data cleanup with regression proof
- [ ] Native/quality and schema integrity validation
- [ ] Final scope and residual limitations recorded

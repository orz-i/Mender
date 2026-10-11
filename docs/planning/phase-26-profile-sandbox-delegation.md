# Phase 26 — Profile to Sandbox Delegation

Status: Complete (verified implementation; user-controlled live grant remains opt-in). Governed by ADR-0036.

Goal: allow a Profile MCP service to acquire one explicitly authorized Sandbox capability without changing ownership or weakening Policy.

Acceptance: deny by default; grant and revoke by the Sandbox owner only; only named existing Profile IDs and read/search/edit/shell capabilities; trusted Profile Principal and matching Origin/Scope; atomic lease reservation checks; immutable grant/revoke audit; Policy enforcement remains independent; no lifecycle authority granted; typed HTTP/SDK/Wails and focused UI; temporary-database end-to-end MCP tests and full repository gates.

Work packages: A docs; B SQLite v8 and atomic authorization; C management API/SDK/Wails; D UI; E acceptance and closeout. Separate commits. No changes to user Sandboxes, secrets, or Tunnel. Exclude skill directories. Deferred: remote individual identities and revoking live in-flight leases.


## Closure evidence (2026-10-11)

**Functional implementation: complete.** No Phase 26 stage proliferation.

| Commit | Deliverable |
| --- | --- |
| `c19ac01` | ADR-0036 and least-privilege authorization contract |
| `835ce08` | SQLite schema v8, capability grants, atomic lease fencing, immutable audit and temporary-db MCP integration |
| `b6a198a` | Typed owner-only HTTP/OpenAPI/SDK/Wails grant endpoints and tests |
| `3b143de` | Focused bilingual Sandbox Profile-access preview, history, review/grant/revoke dialogs |
| `2e45ab1` | Malformed Profile ID and duplicate-capability HTTP boundary validation |

Verification: `pnpm check` exited 0 (Go lint/tests/race, provider plugin proto/SDK/example, module isolation, generated HTTP/Wails contract checks, TypeScript, ESLint, i18n, Vitest, Web/Desktop builds). Frontend **23 test files / 132 tests** passed, with **934 zh-CN translations**. Concurrent cross-connection grant revocation/acquisition passed focused Go Race tests. A temporary-db in-memory MCP test demonstrated Workspace read denied before grant, working after grant, denied after revoke, and cross-Profile/edit attempts denied. `wails3 build` generated the native macOS app. `pnpm security:check` found no known Go or production JS vulnerabilities.

Deployment is **opt-in**: no live user Sandbox, Profile, Credential or Tunnel was modified. The running `wails3 dev` app must restart before the new UI appears. Its Sandbox detail → **Profile access** view lets the owner explicitly grant the existing ChatGPT-linked Profile the desired capabilities. Authorizing a Profile means authorizing *all* callers who can access that Profile endpoint, subject to Tool binding and Policy checks. Remote, live user-workspace MCP execution still requires that owner action and is not claimed as already verified.

Deferred: remote end-user Principal attribution, host-spanning delegation and in-flight lease force cancellation. No compatibility bridge or implicit grant was added.

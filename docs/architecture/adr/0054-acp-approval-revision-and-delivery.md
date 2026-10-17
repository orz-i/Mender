# ADR-0054 — ACP Approval Version Fencing, Delivery and Fair Pending Queues

- Status: Accepted for bounded implementation (2026-10-17).
- Supersedes ADR-0053 only where its cross-domain Policy race and best-effort waiter delivery were explicitly left unproved. Preserves Phase 46 restricted governed Tools and Mender's non-Agent-Loop role.

## Context and authority

A real Host OpenCode repository audit using the owner's pre-existing, time-limited trusted mode produced 10 task-bound read/allowed_once/policy receipts without per-file owner clicks. A Grok read-only audit completed with grok-4.7/high but had no ACP permission receipts. Independent source review identified: Policy admission preceding a separate Agent SQLite approval write, approval ledger changes preceding a nonblocking waiter send, and a global pending list limited to 50 before filtering Agent/workspace. Fix these contracts with deterministic tests; an Agent's report alone does not prove an exploit.

## Policy consistency: one forward-only generation fence

- Policy SQLite v3 adds a monotonically increasing revision to the singleton policy_settings row. A default-decision UPDATE or any policy_rules INSERT/UPDATE/DELETE increments that revision in the same SQLite transaction, including direct SQL writes. Pure reads do not increment. Migration preserves all rules/default/history and grants no new authority.
- Before each owner or automatic *positive* ACP approval, capture the Policy generation BEFORE re-evaluating the original submitting Principal's admission. The exact Agent approval transaction/statement compares the captured generation with the current value from the SAME SQLite database. If different/unavailable, no allowed_once may be recorded; owner rejection remains possible. Bare Agent test fixtures must initialize a temporary Policy v3 store rather than silently bypass the fence.
- The fence applies through durable commit only. It does not revoke an already-delivered native Host action, or guarantee external process cooperation. Keep it backend-neutral; no model-controlled revision or compatibility bridge.

## Pending decision delivery

- Remove silent select/default sends which mark a waiter resolved yet may lose the decision. Require a live waiter reservation and explicit in-process handoff around the database commit. A cancelled, expired or detached waiter must not return owner success or add a fresh positive allow. A DB error must reach the consumer as an error rather than an ACP allow_once.
- Use only the originally offered ACP allow_once/reject_once option, never allow_always. Claim only that an approval was durably committed and handed to the active Mender ACP request consumer, not that the external Agent actually read or obeyed the response.
- Cancellation, duplicate owner clicks, parallel owner/policy decisions and shutdown remain bounded, no indefinite blocking of owner UI. Conflicts fail closed.

## Fairness and bounded automation

- Retain the owner-only global bounded preview list. Implement a separate, eligible-only AgentID/workspace-filtered SQL query that filters BEFORE LIMIT for pending-release operations; requests from another Agent or ineligible actions cannot hide the target workspace.
- Both newly submitted and previously pending automatic approvals obey the same per-Task cap of 64 policy receipts INSIDE SQLite. After cap, expiry, revocation or Policy version mismatch, fall back to safe manual review when the Task still exists, never authorize extra actions. Batch operation return values cannot hide unresolved work.
- No automatic retries after a Task is terminal/expired.

## Parallel ownership

- Real Mender OpenCode delegation: SQL-only slice in backend/internal/agent/infrastructure/sqlite/approval_pending_scope.go, approval_pending_scope_test.go, approval_pending_policy.go. Owns targeted pending query and cap parity, plus isolated tests. No edits in approval_policies.go, application/approval_policy.go, policy subsystem, docs, user files or Git commits.
- Coordinator (this chat): Policy v3 revision schema and tests, Agent approval revision guard and service/bootstrapping, in-process waiter delivery and tests. Shared repository but disjoint paths; no Agent commits, review all diffs, explicit staging and sectional commits.
- No owner policy setting modification or expansion, no new Host trust/fallback, no credential reads or release of secrets.

## Acceptance

Migration tests v1/v2 to v3; Policy rule/default/direct-SQL changes increment revision; deterministic after-admission policy mutation rejection; expired/cancelled/detached waiter tests; >50 global pending but targeted workspace success; equivalent 64 cap on both paths; Go Race Detector, pnpm check, private live database copy migration and actual Mender Task receipts where available. Production Wails/SQLite are never restarted or altered by this audit.

No Mender-owned Agent Loop, no automatic remote authority, no deletion of history or Skill directories.

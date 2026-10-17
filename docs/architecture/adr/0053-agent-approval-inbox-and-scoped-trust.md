# ADR-0053 — Agent 本地审批收件箱与限界放行

- Status: Accepted for implementation in this bounded iteration (2026-10-17).
- Supersedes only Phase 44's *always manually reviewed* UX; does not change ACP v1, Phase 46 governed ToolSupply, Profile grants or Agent ownership.
- Mender remains a tool/resource aggregation, governance and supply plane, not an Agent Loop.

## Observed problem

The native Wails UI currently exposes pending ACP permissions only after navigating Agent → Task → View. A request waits up to 2 minutes, only one explicitly reviewable file read may be allowed once, and subsequent reads need another click. This makes long independent Agent tasks depend on continuous owner attendance. Some requests are unreviewable; Host ACP approvals are **cooperative** and do not prove actual native filesystem confinement.

## Minimal product contract

1. **Central owner inbox** at `/approvals`, accessible from the application navigation, with a compact pending count/indicator. Live owner-only polling (bounded) shows pending Agent name, Task identity, approved preview kind/relative target, expiration, and Approve once / Reject. Each decision still targets exactly one live ApprovalID. No need to open Agent or Task details.
2. **Per-Agent per-workspace approval mode**, locally administered in a focused editor, keyed by immutable AgentID, workspace key and exact current canonical root:
   - `manual` (default, no stored grant): normal per-request approval;
   - `workspace_read`: auto-select only the original ACP `allow_once` for each **reviewable** in-workspace single-file read. Revalidate the canonical path and current original submitter's AgentGrant/Profile/Policy every time. Unsupported or unreviewable operations still require owner rejection/review;
   - `trusted`: a **time-limited explicit owner grant** for the selected Agent/workspace (24 hours maximum), allowing known ACP-native `read/edit/execute` permission requests without the owner present. Only the original one-shot `allow_once` option is echoed per action, never `allow_always`. Unknown or malformed ACP actions fail closed. The owner can revoke instantly; changes affect future requests without rewriting Tasks.
3. An explicit warning beside `trusted` explains: in **Host** isolation, the Agent subprocess has OS-user native filesystem/shell authority and may read/write/execute beyond its cwd, regardless of Mender previews. Trust authorizes the *Agent's cooperative ACP decisions*, not its entire process. Do **not** market it as sandboxed or centrally governed Read/Edit. Use the existing optional restricted-host + Mender ToolSupply for protected project-root writes.
4. The owner grants are persisted in forward-only Agent v12 SQLite settings. `agent_approval_policies` stores no secret/prompt/file content, only AgentID, workspace key, immutable root, mode and expiry. No row means manual. **Every owner policy insert/update/revoke** writes a minimal, immutable `agent_approval_policy_events` audit record via SQLite triggers, retaining the AgentID/workspace/root, action, mode and expiry even after a grant is removed. Existing Agent history and approvals (v1–v11) remain unchanged. Unbinding a workspace with an explicit grant requires disabling it first. New permissions cannot be set by MCP delegated tools, external HTTP or an Agent prompt.
5. Every automatic permission remains one bounded, immutable `agent_task_approvals` ledger entry marked `decision_source=policy` and status `allowed_once`. Its SQLite transaction rechecks the running Task generation/cancellation, exact workspace root, Profile AgentGrant/workspace grant and unexpired owner policy. The application also re-runs the original submitting Principal's current Policy/admission **before** that SQLite transaction. This prevents reuse after already committed grant revocations; because Policy evaluation and ledger persistence are separate domain operations, it does **not** promise strict simultaneous atomicity for a Policy mutation racing between those steps. Such cross-domain Policy-generation fencing is a separate security improvement. No auto-retry after task/cancellation/unknown.
6. Reuse the backend-neutral ACP PermissionRequest and owner service. Never embed OpenCode-only logic in domain authorization or create an Agent orchestration loop. Normal interactive choices still verify only safely previewable single-file reads; the trusted owner grant is a separate explicit, time-limited risk acceptance. No model-supplied policy, unknown request type or arbitrary “allow all” shortcut.

## Acceptance and rollback

- Original Host Agent remains `manual` with no database setting after upgrade.
- Wails owner inbox lists cross-Agent pending events with one click; model/MCP and Web clients cannot enumerate/change approval settings.
- In a disposable Runtime: repeated reads auto-approve without waiting, produce individual ledger records, and preserve exact offered option IDs. Unreviewable read/edit/execute remain manual in `workspace_read`. `trusted` processes known ACP categories only when owner opted in; revoke/expiry/Task cancel/Profile grant/Policy/root mismatch fail closed; unknown and duplicate IDs never auto-allow.
- Normal Wails menu shows preview-first modes with action dialog and explicit trust warning. Do not expose approval forms on the Agent index.
- Full `pnpm check`, Go race, generated Wails binding parity, TypeScript/i18n and stable production row fingerprints. The v11→v12 default state must have **zero** policy grants and **zero** invented permission-change events; future policy set/revoke events must be append-only and immutable. No changes to production approvals before a controlled user restart; do not auto-enable `trusted` for existing Agent data.

## Out-of-scope and stopped work

The previous provider-v1 cleanup and Agent audits are **paused**, not abandoned. Uncommitted staged deletions and replacement tests remain in the worktree and must not be mixed into these commits. Do not delete old data or run unattended Host prompts during this approval task. Remaining Code Mode fetch/network and arbitrary native execution are not universally governed.

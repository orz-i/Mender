# Phase 48 — Agent 审批中心与无人值守放行

Status: implementation in progress. Authority: [ADR-0053](../architecture/adr/0053-agent-approval-inbox-and-scoped-trust.md).

1. Docs/ADR and paused cleanup worktree fingerprint; commit only new docs.
2. Agent v12 policy row, atomic automatic approval audit, owner-only APIs, bounded pending list and negative tests. One full cut; no compatibility bridge.
3. Native Wails SDK/client, focused Agent permission settings and global approvals inbox with pending indicator. Default-off, no form wall on inventory, Chinese and English.
4. Synthetic ACP read/read/unreviewable/trusted/revoke and full quality gates, isolated DB and native UI acceptance. Do not silently change user Agent settings. Return to cleanup after owner acceptance.

Existing unfinished provider-v1 cleanup files are intentionally excluded from Phase 48 commits.

# Phase 49 — ACP Approval Correctness and Fairness

Status: implementation in progress (2026-10-17). Authority: ADR-0054.

1. Commit ADR/plan and record the unchanged production DB, Agent trust settings, Git HEAD and approval state.
2. Submit scoped real OpenCode work for non-overlapping SQL pending fairness and 64-cap parity only (no Agent commit). Concurrently implement policy revision fence and in-process waiter delivery, independently review the Agent diff.
3. Test v1/v2-to-v3 policy upgrade, preserved rules/default/other business tables, revision triggers, deterministic post-admission revocation, owner-only decisions, cancelled/duplicate waiter, queue saturation, targeted-workspace filtering, cap 64.
4. Run focused tests, complete Go race, pnpm check, latest native Wails build when necessary, and a consistent production DB clone migration. Real Agent or Wails acceptance with exact Task IDs, no mutation of active user policies or worktrees beyond tracked intentional files.
5. Document tested and residual boundaries, commit in bounded units; avoid escalating into many subphases.

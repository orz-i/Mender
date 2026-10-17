# Agent 真实开发任务验收：不止 Read

Status: real delegated task exposed a Host OpenCode execution-policy blocker; repair and disposable full coding acceptance pending (2026-10-17). This remains a bounded real code-delivery scenario, not a new Mender Agent Harness or numbered implementation phase. Builds on ADR-0054 / Phase 49.

## Why

The prior delegated tasks disproportionately exercised read-only repository reviews. In Phase 49, OpenCode produced some SQL source changes but the delegated Task ended `failed`, and the coordinator repaired/tested/committed them. A disposable Phase 46 Read/Edit smoke is not a full end-to-end repository development delivery. The user explicitly expects **Agent owns read → edit → run tests → report**, with Mender providing permission governance, resource/tool supply, durable Task lifecycle and audit but **not Agent Loop**.

## Concrete engineering assignment

Fix the remaining ACP permission response **check-then-write cancellation race** without granting new capabilities. The current implementation calls `ctx.Err()` in `permissions.go::dispatchPermission` and later calls `stream.send` in `runner.go`, leaving a window. Implement a minimal cancellation-aware bounded send path under the existing stream write lock, with deterministic regression tests. Do not claim perfect external Agent receipt or absolute atomicity with an asynchronous context cancellation that occurs *after* write begins.

OpenCode is the primary **real engineering owner** of these exact files only:
- `backend/internal/agent/infrastructure/acp/runner.go`
- `backend/internal/agent/infrastructure/acp/permissions.go`
- `backend/internal/agent/infrastructure/acp/approval_cancel_delivery_test.go`
- optionally one NEW `backend/internal/agent/infrastructure/acp/permission_send_gate_test.go`.

It must actually edit Go code, create regression coverage and run `gofmt`, `go test ./backend/internal/agent/infrastructure/acp -count=1` and `go test -race ./backend/internal/agent/infrastructure/acp -run 'TestACP.*Cancel' -count=1`. No read-only substitute, shell/file writes beyond those files, new dependencies, schema changes, permissions broadening, secrets, Git commit/stash/reset/rebase, or Wails restart. Agent may run read-only Git diff/status but must not stage/commit. Shared branch is owner-controlled; the coordinating assistant reviews every diff and commits after independent tests.

## Observed failed real coding Task and correction

- Real Mender OpenCode Task `atask_67ce358d25defb904fdbb1b1bffc0732` was explicitly authorized to edit ACP source and run Go tests. It returned **failed** after 4 `read / allowed_once / policy` and 1 `execute / allowed_once / policy`, with **zero source edits or completed test output**; the user request was **not delivered**. Mender persisted only the generic `Agent execution failed` diagnostic, so those events alone do not prove the precise internal failing operation.
- Inspection revealed a deterministic configuration conflict: ordinary Host OpenCode overlay `opencodeACPReviewConfig` sets Code Mode `execute:deny`, while OpenCode v2 exposes major tooling through Code Mode. Phase 46 separately established this mismatch and allowed `execute` only for restricted ToolSupply. For a normal Host Agent the correct **cooperative** envelope is `execute:ask`, NOT automatic `allow`; `manual` mode still cannot authorize an unreviewable execute and must fail closed. The owner's explicitly configured, time-limited `trusted` mode can return one original ACP `allow_once` per request. The immutable Agent approval/Policy fences remain intact.
- Require normal Host configuration test proving `execute:ask` together with native `read/edit/shell:ask`, `external_directory/subagent:deny`, plus ACP negative manual mode and positive expiring trusted mode. The trusted user must understand Code Mode may have JS network/session effects, so this is **not mandatory sandbox containment**. A separately registered restricted-host ToolSupply still requires its unchanged owned overlay.
- Before claiming full development, run a REAL installed OpenCode CLI on a brand-new disposable Go workspace with an explicit owner trust grant in a separate SQLite, ask it to edit source and run tests, verify actual file bytes and tool approvals, then repeat a production Mender delegated coding task after controlled application deployment. Do not substitute a synthetic ACP fixture or coordinator implementation for Agent success.

## Acceptance

1. Real Mender `agent.delegate` returns a TaskID and terminal `completed`, with task result describing actual file changes and executed test commands, and owner-independent checks confirming modified source and test results. If Task fails, record failure instead of claiming full Agent delivery—even if files changed.
2. A deterministic test holds the stream write lock, then cancels the Task **before** releasing it. The permission response must not select a late `allow_once`; cancellation/release racing the lock is covered by Race Detector. Legitimate noncancelled responses still select only the offered `allow_once`/reject option, no `allow_always`.
3. All unchanged ACP host/governed permission tests pass; no new Agent Loop or provisioned Tool/Provider; schema v12 Policy v3 and existing Host trust untouched. Full canonical `pnpm check` and a separate native Wails Go build after owner review.
4. Explicitly distinguish the bounded improvement `send` cannot authoritatively prove external Agent receipt. Mender's governance audit remains an authorization ledger, not delivery/execution proof.

## Coordination

Do not disguise a failed Agent job as successful by having the coordinator silently implement its work. If the Agent cannot edit or run tests with the user's pre-existing scoped approvals, identify the precise missing capability. The coordinator may fix defects found during review as an explicitly separate contribution, but must report those separately.

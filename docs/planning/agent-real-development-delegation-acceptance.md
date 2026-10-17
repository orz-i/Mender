# Agent 真实开发任务验收：不止 Read

Status: in progress (2026-10-17). This is a bounded real code-delivery scenario, not a new Mender Agent Harness or numbered implementation phase. Builds on ADR-0054 / Phase 49.

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

## Acceptance

1. Real Mender `agent.delegate` returns a TaskID and terminal `completed`, with task result describing actual file changes and executed test commands, and owner-independent checks confirming modified source and test results. If Task fails, record failure instead of claiming full Agent delivery—even if files changed.
2. A deterministic test holds the stream write lock, then cancels the Task **before** releasing it. The permission response must not select a late `allow_once`; cancellation/release racing the lock is covered by Race Detector. Legitimate noncancelled responses still select only the offered `allow_once`/reject option, no `allow_always`.
3. All unchanged ACP host/governed permission tests pass; no new Agent Loop or provisioned Tool/Provider; schema v12 Policy v3 and existing Host trust untouched. Full canonical `pnpm check` and a separate native Wails Go build after owner review.
4. Explicitly distinguish the bounded improvement `send` cannot authoritatively prove external Agent receipt. Mender's governance audit remains an authorization ledger, not delivery/execution proof.

## Coordination

Do not disguise a failed Agent job as successful by having the coordinator silently implement its work. If the Agent cannot edit or run tests with the user's pre-existing scoped approvals, identify the precise missing capability. The coordinator may fix defects found during review as an explicitly separate contribution, but must report those separately.

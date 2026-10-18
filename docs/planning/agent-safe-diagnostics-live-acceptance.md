# ACP 失败诊断：部署后实机验收与双 Agent 开发

Status: live configuration-stage verification accepted; real coding tasks and independent checks in progress (2026-10-18).

## Scope and safety

The user has rebuilt and reconnected Mender. Wails and Tunnel are running with the source at `b41baaf` and policy schema v3 / Agent schema v12. Existing owner Host trust grants belong to the user; this round does not enable, extend or revoke them. No historical Task data or production SQLite rows will be deleted, no compatibility adapter added, and Mender remains a tool/resource/permission supplier rather than Agent Loop.

Two explicitly expected configuration rejections exercise the **deployed** safe failure classification without any real Agent prompt or tool invocation:

- OpenCode `atask_3255ba655e547929eba8a4f37f41bd73`: failed, `Agent model or reasoning configuration failed`.
- Grok `atask_4b4933aa6a99c1b9ed6bb335dae6d153`: failed, `Agent model or reasoning configuration failed`.

Both were created through Mender external Agent delegation using an unadvertised model ID; no process prompts, native file changes or Shell calls were requested. This proves the installed Agent v12/Policy v3 Runtime returns bounded stage diagnostics, not underlying remote text, but does not prove every model/provider failure is correctly categorized.

## Genuine parallel coding acceptance

Do not substitute code reading, a failed Task with partial edits, or a coordinator-created patch for an Agent's successful delivery.

**OpenCode assignment:** narrow ACP error-message minimization in `backend/internal/agent/infrastructure/acp/failure_stage.go` and `failure_stage_test.go`. The two error types `rpcRejectedError` and `malformedOutcomeError` currently interpolate `method` into Error(), even though method is trusted on existing call paths. Harden their displayed messages to immutable constants with no method interpolation, preserve their type-based classification and errors.Is/Unwrap, add regression tests containing a deliberately injected sentinel method. Run gofmt, ACP Go package tests and targeted Go Race Detector. No Agent Git commit or other paths.

**Grok assignment:** separate application test-only implementation in `backend/internal/agent/application/task_failure_diagnostic_test.go`. Add a deterministic integration test using the existing application fixture that returns a stage-wrapped `context.DeadlineExceeded`, confirming terminal `unknown` and existing `Agent outcome unknown after timeout` precedence rather than a new failure stage. Verify no diagnostic leaks fake error payload and Task result remains empty. Run gofmt and targeted Go tests. No other files, no Git commit, no external network. Explicit `grok-4.7/high` is the previously verified working per-Task configuration; no user default changes.

Both work in disjoint files with real Mender TaskIDs. Owner independently reviews Go diffs and outputs, runs race/full gates, commits code with accurate attribution only for completed Agent deliverables. Before any production-code merge, no unexplained modifications outside the allocated scopes. If Grok cannot edit/test, preserve `failed` plus the new safe stage diagnostic rather than guessing its root cause.

## Closure gates

- Two deployed configuration probes return exact safe diagnostic as above.
- Real Agent code edits + actual tests + terminal `completed` are required individually.
- No credential/file content in Task `diagnostic`.
- Go race, Go lint, `pnpm check`, native Wails Go build; production DB readonly verification.
- Do not restart production Wails on the user's behalf or change its Host approval modes.

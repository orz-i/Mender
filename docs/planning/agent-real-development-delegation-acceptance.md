# Agent 真实开发任务验收：不止 Read

Status: Host Code Mode approval-gate repair implemented; disposable real OpenCode code-edit + execution + terminal Task accepted, external Mender production code-delivery after restart remains pending (2026-10-18). This is not a new Agent Harness or numbered implementation phase. Builds on ADR-0054 / Phase 49.

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

## 修复与真实编码验收结果（2026-10-18）

- 文档先行提交 `bff31a6`，适配器修复提交 `8300b3c`。OpenCode v2 官方文档指出 `execute:deny` 将移除 Code Mode 与只通过它到达的工具（`https://opencode.ai/v2/docs/tools`）。原 Host 配置确实如此；已最小硬切为 `execute:ask`，而不是 `allow`。ACP 原本的 `manual` 模式不会自动通过无法核对的 execute，请求仅能在所有者明确配置限时 `trusted` 时得到一次性授权。Host 仍是协作式授权，非强制沙盒；当前原生 `read/edit/shell` 仍保持 `ask`，`external_directory/subagent` 仍 `deny`；restricted ToolSupply 的固定策略没有修改。
- **第一次完整代码委派失败**：生产 Mender Task `atask_67ce358d25defb904fdbb1b1bffc0732` 返回 `failed`，数据库只有 4 read + 1 execute `allowed_once/policy`，没有实际编辑或运行测试的结果。当前 generic `Agent execution failed` 不能解释内部具体故障。不能将其算作成功 Agent 交付。
- **真实 CLI/完整编码一次性验收成功**：明确启用 `MENDER_REAL_HOST_CODE_DEVELOPMENT=opencode` 的隔离 Bootstrap Runtime，从系统已安装的 OpenCode v2.0.30 运行真正的 ACP Agent，使用一次性 Host Agent、临时独立 SQLite、全新 Go module 以及仅属于该测试实例的 `trusted` owner 模式。测试要求 Agent 把错误的乘法实现修改正确并运行 Go 测试。
- 实际测试进程在约 **133 秒**后返回 `TaskCompleted`，ACP SQLite 审计记录 `read=3/edit=1/execute=11` 条成功 `allowed_once/policy`；断言 `multiply.go` 包含正确 `a * b` 代码，并在临时根独立执行 `GOPROXY=off GOSUMDB=off GOWORK=off go test ./...` **通过**。日志 `/tmp/mender-real-agent-development-20261017/third-run.log`，退出码 `0`；原始任务输出没有作为逐条 shell 指令审计持久化，因此 **不能仅凭这些事件逐字证明模型调用过指定的那条 `go test` 命令**，只能证明真实文件编辑、真实 ACP 执行审批、Task 完成和独立测试正确。
- 另两次有记录的 QA 尝试分别在 180 秒（默认 CLI 模型）和 75 秒（显式 `opencode/glm-5.3-flash-free`）超时并失败；75 秒尝试已有 `edit=1/execute=11/read=2` 且代码实际被修改。两次失败被保留为真实可靠性证据，不合并成成功率 100%。最终成功的一次明确选择 `opencode/glm-5.3-flash-free` 且指令要求完成测试后立即结束。
- **正式质量门禁通过**：`/tmp/mender-real-agent-development-20261017/full-check.exit = 0`，包含 Go 全量测试与 Race Detector、Go lint、SDK/Wails 绑定、前端 TypeScript/ESLint、Vitest **46 文件 / 268 项**、i18n **1309/1309**、Web/Desktop 构建；原生 Wails Go 桌面程序也单独编译通过。原生 ACP 适配器定向 Go Race 测试通过，`golangci-lint` 0 issues。真实编码测试默认 **skip**，绝不在无人值守 CI 中启动模型或授予 Host 信任；生产数据库、Agent、Task、授权、仓库源文件均未受 QA Agent 的执行影响。变更仅为 OpenCode ACP 启动时的请求策略，仍由 Agent 自己控制开发循环，Mender 只审批和供应工具。
- 生产 Wails 需要正常构建重启载入 `8300b3c` 后，才能再次通过 Mender 外部 `agent.delegate` 分发实际项目代码任务。**这一步尚未验证成功**，不得把一次性 Runtime 验收冒充外部隧道生产分发完成。

## Acceptance

Current gate: **disposable real CLI coding PASS; external Mender-delegated real repository coding still PENDING a controlled Wails build/reconnect**. The earlier real source modification Task failed and is not counted as delivered; the failure exposed the Code Mode hard deny, which was repaired by the coordinator, not by the Agent.

1. Real Mender `agent.delegate` returns a TaskID and terminal `completed`, with task result describing actual file changes and executed test commands, and owner-independent checks confirming modified source and test results. If Task fails, record failure instead of claiming full Agent delivery—even if files changed.
2. A deterministic test holds the stream write lock, then cancels the Task **before** releasing it. The permission response must not select a late `allow_once`; cancellation/release racing the lock is covered by Race Detector. Legitimate noncancelled responses still select only the offered `allow_once`/reject option, no `allow_always`.
3. All unchanged ACP host/governed permission tests pass; no new Agent Loop or provisioned Tool/Provider; schema v12 Policy v3 and existing Host trust untouched. Full canonical `pnpm check` and a separate native Wails Go build after owner review.
4. Explicitly distinguish the bounded improvement `send` cannot authoritatively prove external Agent receipt. Mender's governance audit remains an authorization ledger, not delivery/execution proof.

## Coordination

Do not disguise a failed Agent job as successful by having the coordinator silently implement its work. If the Agent cannot edit or run tests with the user's pre-existing scoped approvals, identify the precise missing capability. The coordinator may fix defects found during review as an explicitly separate contribution, but must report those separately.

# Agent 真实开发任务验收：不止 Read

Status: **real external Mender OpenCode repository test-development delivery accepted** (2026-10-18): one delegated Task edited tracked Go test code, ran tests, returned completed and was independently verified/committed. Three separate delegated production-race-fix attempts failed; coordinator implemented that fix separately. Complete post-change quality gates passed; latest ACP fix requires user-controlled Wails restart. No new Mender Agent Harness or Agent Loop. Builds on ADR-0054 / Phase 49.

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
- `8300b3c` 已经通过随后一次正常用户构建/重启由真实 Mender 外部委派验证：OpenCode Task `atask_798778de8ef6af7e5ee68e461ed36c48` 确实修改 Mender 仓库代码、调用执行工具并终态完成（详见下方收据）。但最新独立 ACP 竞态修复 `70e9a98` 尚未被生产 Wails 重新构建、重启并复测，不能将其视为当前运行时已部署。

## Acceptance

Current gate: **disposable real CLI coding PASS; external Mender real repository regression development PASS; original ACP production-race-fix Agent deliveries FAILED and were implemented by coordinator instead**. All contributors and Task outcomes are documented; no false Agent attribution. Latest coordinator fix still needs normal Wails deployment.

1. Real Mender `agent.delegate` returns a TaskID and terminal `completed`, with task result describing actual file changes and executed test commands, and owner-independent checks confirming modified source and test results. If Task fails, record failure instead of claiming full Agent delivery—even if files changed.
2. A deterministic test holds the stream write lock, then cancels the Task **before** releasing it. The permission response must not select a late `allow_once`; cancellation/release racing the lock is covered by Race Detector. Legitimate noncancelled responses still select only the offered `allow_once`/reject option, no `allow_always`.
3. All unchanged ACP host/governed permission tests pass; no new Agent Loop or provisioned Tool/Provider; schema v12 Policy v3 and existing Host trust untouched. Full canonical `pnpm check` and a separate native Wails Go build after owner review.
4. Explicitly distinguish the bounded improvement `send` cannot authoritatively prove external Agent receipt. Mender's governance audit remains an authorization ledger, not delivery/execution proof.

## 真实生产 Mender 仓库开发委派（2026-10-18）

**验收目标不是 Read。** 当前 Wails 与 Mender Tunnel 在重新构建后已接入，OpenCode 使用已存在、未修改或延长的 owner `trusted` 授权。协调者通过 Mender Profile 原生 `agent.delegate → status/result` 提交**明确允许修改代码、执行 Go 测试**的任务，并将 Agent 修改文件与自己修改文件区分提交。Mender 没有参与 Agent Loop，也没有新增 Tool/Provider/Sandbox 或自动扩大授权。

| 实际 TaskID | 开发目标与测试参数 | 最终状态 | 持久化 ACP 记录 | 代码结果 |
| --- | --- | --- | --- | --- |
| `atask_1d5261ea4bec2d2ada8f54d988815d27` | 修复 ACP send/cancel 竞态；继承 `max` | **failed**，约 338 秒 | read 7 / execute 15，均 `allowed_once/policy` | **0 修改**、无测试成功证据 |
| `atask_be54e86395897faee1da2693c1ff1d32` | 缩小版相同修复；`default` | **failed**，约 56 秒 | read 3，均自动批准 | **0 修改** |
| `atask_798778de8ef6af7e5ee68e461ed36c48` | 编写 restricted_host 无 ToolSupply 时 Code Mode 必须 `ask` 的回归测试；`low` | **completed**，约 78 秒 | read 2 / **edit 1 / execute 9**，均自动批准 | OpenCode **实际新增 65 行 Go 测试**，报告 Go 测试通过；本地可观察实际 `go test` 进程 |
| `atask_d4152cc9717a680269068e084a5302e4` | 再次尝试生产 ACP send/cancel 修复；`low` | **failed**，约 32 秒 | read 3，均自动批准 | **0 修改** |

**真实完成的 Agent 贡献：** `backend/internal/agent/infrastructure/acp/opencode_launcher_test.go::TestOpenCodeRestrictedWithoutToolSupplyStillRequiresOwnerExecuteApproval`。OpenCode 仅修改该文件，新增 65 行，验证普通 restricted Host 没有 ToolSupply 时原生 read/edit/shell/execute 均为 `ask`、external_directory/subagent 为 `deny`，也未意外注入 governed-only OpenCode 项目配置开关。Mender Task `completed` 与 `read/edit/execute` 审计独立一致。协调者**没有替 Agent 编写这份测试**，仅审核 `git diff`、独立运行完整 ACP Go package、Race Detector 和 `pnpm go:lint`（0 issues），随后提交 `2b61d91`。这是首次在**用户实际 Mender 仓库**中通过外部 Mender 委派形成 `Agent 编辑→Agent 测试→Task 完成→协调者复验→Git 提交` 的完整链路，而非一次性沙盒或只读审计。

**实际生产代码修复是协调者另行完成，不能计入 Agent 贡献。** 因上面三个 ACP 生产代码修改委派失败，协调者独立完成 `backend/internal/agent/infrastructure/acp/runner.go::sendPermission/sendLocked` 和 `permissions.go::dispatchPermission` 的最小修复：只在 ACP 权限响应路径持有现有发送锁时重检 Task context，阻止锁争用期间已取消的迟到 `allow_once`；正常 ACP 消息发送路径与原始一次性选项保持不变。新增阻塞发送锁、活跃响应、Task 取消的 Go 回归测试，ACP package 与 Race Detector 及 25 轮定向 Race Detector 均通过，lint 0 issues，另行提交 `70e9a98`。这保留已经在 ADR-0054 声明的取消检查与实际管道 Write 间的极短竞态，不保证外部 Agent 读取/执行已授权操作。

**安全、部署和可靠性边界：** 此处 Host `trusted` 是用户已启用的协作式 ACP 授权，**非**强制沙盒。失败 Task 的 `Agent execution failed` 诊断仍缺乏分层的非敏感失败阶段信息，不能臆断是模型、工具参数、配置或上游限制；从 `max/default/low` 各次结果也不能推导确定的模型可靠性因果。所有用户生产 Agent、Policy、Profile 授权、旧 Task、Sandbox 配置和 Skill 目录保持原状，测试未删除数据。最后的生产 ACP 代码修复需下次正常 Wails 构建重启才能加载；**已有的真实委派成功针对 Agent 测试代码交付，不代表此修复已在生产部署后的另一条委派中复验**。


### 最新质量与交接收据（2026-10-18）

- 仓库开发 Agent 贡献 `2b61d91`、协调者 ACP 生产修复 `70e9a98`，均经 `git diff --check` 和独立单元测试审查后分段提交；没有提交或修改 `.agents/`、`.pi/`、`.intent-loop/`、`.commandcode/`。
- 完整 `pnpm check` **exit 0**：收据 `/tmp/mender-real-repo-delegation-20261018/check.exit`，日志 `/tmp/mender-real-repo-delegation-20261018/check.log`；后端全量 Go Race Detector、go lint、bindings/architecture、TypeScript/ESLint、**46 个 Vitest 文件 / 268 项**、中文 i18n **1309/1309**、Web/Desktop 构建通过；最新原生 Wails Go 编译另行通过，临时可执行文件已删除。
- 真实 Task 账本与原仓库代码交叉核对：4 条 Mender 源码开发委派中 **1 completed、3 failed**。成功的 `atask_798778de8ef6af7e5ee68e461ed36c48` 用时约 78 秒，有 `read 2 / edit 1 / execute 9` 条成功的 `allowed_once/policy`；它写出的 65 行实际保留在 `2b61d91`。另外三条失败任务无受审源码修改，不应算作开发交付。无法仅用这些 approval kind 证明每一条原生命令的具体内容，但实际 `go test` 进程与 Agent 回报、协调者复验相符。
- 生产用户 SQLite 只读复核：Agent schema **v12**、Policy schema **v3**、3 个现有 Agent、已有 Grok/OpenCode 两条有效限时 Host 信任策略、0 个受治理 Tool placements、`quick_check=ok`、外键异常为零。测试过程没有新增、取消或续期用户授权，没有修改生产 Agent、Profile、Task 历史或外部 Provider；Mender 隧道和 Wails 进程未被协调者重启。
- 协议边界仍保留：`allowed_once` 是授权决定记录，不是外部 Agent 已收到/执行响应的凭证；`sendPermission` 只在获得锁、写出响应之前检查 Context，取消恰好发生于最后一次检查后/pipe Write 期间仍可能竞态。Host 的权限授权是协作式，不是强制项目目录写入隔离。尚未解决的可靠性改进是对 `Agent execution failed` 引入不泄漏原始 stdout、stderr、凭据和提示词的分层失败诊断。

## Coordination

Do not disguise a failed Agent job as successful by having the coordinator silently implement its work. If the Agent cannot edit or run tests with the user's pre-existing scoped approvals, identify the precise missing capability. The coordinator may fix defects found during review as an explicitly separate contribution, but must report those separately.

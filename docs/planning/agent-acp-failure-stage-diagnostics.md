# Agent 失败阶段诊断与真实开发委派

Status: **implemented and fully accepted by automated tests; production Wails activation pending user-controlled rebuild** (2026-10-18). Boundary: Mender only admits, governs, supplies and audits external Agents; it does not run their reasoning loop.

## Confirmed post-rebuild baseline

- User-rebuilt Wails dev process and Mender Tunnel are connected; repo HEAD `e0cb565`, tracked worktree clean. Production schema: Agent v12, Policy v3, 3 Agents and 2 existing owner approval modes; `PRAGMA quick_check=ok`.
- Previous real Mender OpenCode coding attempts `atask_1d5261ea4bec2d2ada8f54d988815d27`, `atask_be54e86395897faee1da2693c1ff1d32`, `atask_d4152cc9717a680269068e084a5302e4` ended `failed`, all persisted only `Agent execution failed`. The separate true code-edit and test delivery `atask_798778de8ef6af7e5ee68e461ed36c48` completed. These failures must not be reinterpreted retroactively.

## Goal and design

Replace the generic `Agent execution failed` with a **finite, immutable and non-sensitive** stage code/message, grounded in a failure from the native ACP runner. No raw CLI stderr, model prompts, tool parameters, remote RPC error messages, credential values, shell commands, full file paths, session identifiers or model output may enter `agent_tasks.diagnostic`, Wails bindings, external profile Tools or logs. A stage code identifies *where* failure surfaced, not its root cause or a proven model error.

The native runner returns an error implementing the backend-neutral interface:

```go
interface { SafeAgentFailureStage() string }
```

Only these fixed values are accepted by the Agent Application Service:

| Code | Owner-facing stable diagnostic |
| --- | --- |
| `process_launch` | Agent process could not start |
| `initialize` | ACP initialization failed |
| `session_create` | ACP session creation failed |
| `configuration` | Agent model or reasoning configuration failed |
| `permission` | Agent permission request was rejected |
| `prompt_rejected` | Agent rejected the ACP prompt |
| `prompt_transport` | ACP prompt transport failed |
| `protocol` | ACP protocol validation failed |
| `result` | Agent returned an invalid completion |

Anything else, including maliciously supplied free-form stage labels, remains `Agent execution failed`. Existing cancellation, timeout, agent outcome unknown, result overflow and governed ToolSupply failures retain their established semantics/priority; no modification of prior Task records. No new database table or schema bump is required.

### Implementation ownership

1. **Docs first**; commit this scope before implementation.
2. **OpenCode real coding owner**, precise non-overlapping ACP-only files: `backend/internal/agent/infrastructure/acp/runner.go`, NEW `failure_stage.go` and NEW `failure_stage_test.go`. Implement stage tagging and fixture tests; run actual gofmt + Go package tests and Race Detector. Do not commit or edit outside these files, use network, secrets, Profile settings, production DB, Wails process or Skill directories. Real Task must finish `completed` with a verifiable source diff; otherwise the coordinator must report failure, not claim successful Agent development.
3. **Coordinating assistant**, disjoint Application-only files: `backend/internal/agent/application/task_failure_diagnostic.go`, `task_failure_diagnostic_test.go`, `service.go`, targeted test fixtures if needed. Map the *fixed*, trusted vocabulary to fixed strings, test censorship of malicious/raw error chains, and preserve cancellation/outcome logic. Don't modify OpenCode-owned files while its Task is running.
4. Independent line-by-line review of Agent diff, targeted Go test/Race Detector, production-consistent **private SQLite copy** acceptance, full `pnpm check`, native Wails build, sectional Git commits. No forced restart or migration of the active user Wails.

### Acceptance

- Synthetic native ACP failures at process launch, init, new session, invalid protocol, model configuration, RPC prompt rejection, premature prompt transport close, unresolved permission and malformed final result produce correct **fixed codes**; tests must not persist raw fake secrets even when underlying `error` contains them.
- The application persists exactly one safe message for each known code. Unknown or injected stage names and raw error chains cannot influence the diagnostic; no arbitrary `fmt.Sprintf` of an untrusted error.
- Running real OpenCode via Mender can still edit/test and use the existing explicit owner `trusted` mode; this round must not claim a failed real Agent Task as successful development. Any live smoke test may use a disposable workspace and no user data mutations.
- Original user Task history, Agent/Provider identities, ToolSupply, grants and Policy v3 behavior remain unchanged. Schema upgrade unnecessary.
- Limited nonsecret stage reporting is **not** full observability of model APIs or native OS shell outputs and must not be described as such.

## 真实委派、完整回归与交接（2026-10-18）

**交付范围已收口，运行实例激活待下一次正常构建重启。** 固定 ACP 失败阶段的应用层映射、Runner 阶段标记、后端与 Bootstrap 真实 Runtime/一次性 SQLite 集成测试，以及中文 Wails Task 详情均已完成。本轮没有让 Mender 承担 Agent Loop、增加 Provider/Tool、扩大原生 Host 权限或迁移用户历史数据库。

### OpenCode 真实编码任务账本

| Mender TaskID | Task 终态 | 真实执行 / 产物 | 验收分类 |
| --- | --- | --- | --- |
| `atask_40992b6948e30d431f10f35321f4f9f2` | **failed** | 5 次 Read、1 次 Edit 的 `allowed_once/policy`；产生 `failure_stage.go` 初稿，未完整接线或测试 | 仅保留部分代码，经协调者独立审查后集成；**不算成功交付** |
| `atask_53f29f839c26ba50817f318e07c458bf` | **completed** | 1 次 Read、1 次 Edit、11 次 Execute；OpenCode 新增 `failure_stage_test.go` **141 行**，真实运行 gofmt 与 Go 测试并通过 | **真实仓库代码开发交付成功**；协调者单独修正测试 lint 并复验 |
| `atask_93b5a36cff475a81a40d19970f27e9c1` | **failed** | 5 次 Read，无目标 Runner 源码修改 | **不算成功交付**，由协调者独立完成余下阶段接线 |

协调者另行实现 `agentFailureDiagnostic` 白名单映射、ACP Runner 各阶段接线、模拟远端 RPC 敏感消息的集成夹具、真实 Runtime 的终态安全持久化测试、中文 UI 显示及验收文档。真实开发贡献不应与 Agent 报告混淆。

### 代码分段提交

- `2bd616c` — 文档先行，定义固定九阶段、敏感信息最小化和真实委派验收边界。
- `93dfa22` — Agent Application 固定白名单诊断及安全持久化单测。
- `1b67e2a` — ACP 不透明的错误阶段包装器，以及 OpenCode 实际新增、协调者审计修正的安全测试。
- `41552fc` — ACP initialize/session/new/configuration/permission/prompt/result/process 阶段接线，远端 RPC 错误消息彻底不采信；Bootstrap 一次性 SQLite 验收。
- `12000ad` — Wails 的任务结果弹窗使用固定诊断的简体中文翻译与 React 用户路径测试。

### 独立验收及用户数据

- 全量 `pnpm check` **exit 0**，收据 `/tmp/mender-agent-safe-diagnostics-20261018/check.exit`，日志 `/tmp/mender-agent-safe-diagnostics-20261018/check.log`。包含全部 Go 测试与 Race Detector、Go lint、Provider/Sandbox/Tunnel SDK/协议、Wails 绑定/架构、TypeScript、ESLint、Vitest **46 个文件 / 269 项测试**、i18n **1325/1325**、Web/Desktop 构建。原生 Wails Go 应用另行编译成功，临时二进制已删除。Vite 分包尺寸建议为非阻断警告。
- ACP 使用**实际 Go 子进程**模拟错误初始化、会话创建、配置/模型选择、权限、RPC prompt 拒绝、传输中断和无效结果；每个场景的固定阶段正确。模拟错误文本包含假的 bearer/private path，不会进入 Task 诊断。真实 Bootstrap Runtime + 临时 SQLite 中的进程启动失败最终只写入 `Agent process could not start`，不持久化底层凭据或失败 Task 返回文本。未知阶段回到 `Agent execution failed`，取消/超时的原生命周期优先。
- 生产数据库**只读**核对：Agent schema **v12**、Policy schema **v3**、3 个现有 Agent、37 条历史 Task、138 条 ACP 决策记录、原有 Grok/OpenCode 两条有效 Host `trusted` 设置、0 个受治理 Tool placement、`PRAGMA quick_check=ok`、无外键异常。**本轮没有修改生产 Policy、Agent、审批设置或历史记录**；新增的三个委派 Task 是正常历史事件，未被更改或删除。
- **部署门禁**：当前观察到的生产 Wails 是用户本轮开始时 08:45 构建的旧进程。最新诊断代码在该进程之后提交，**仍需用户正常重构建/重启**才能在外部 Mender 委派任务的 `diagnostic` 中看到细分信息。过去的失败 Task 不会自动重分类，也不应复制错误文本去修改历史记录。

**边界：** 诊断阶段说明错误 surfaced 的 ACP 生命周期位置，并不证明模型 API、账户、网络、工具命令或远端返回文本中的具体根因。Mender 继续不保留 Agent 原生 stdout/stderr、模型内部推理或命令体，也不会把 `allowed_once` 误认为 Agent 已收到或执行操作。

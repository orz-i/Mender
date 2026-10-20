# Session / Task 生命周期职责分离：实施契约（2026-10-20）

状态：阶段 A 审计完成；阶段 B/C 等待本轮代码与测试验收。协议基线严格保持 ACP v1；Agy 走现有 Native Headless。Mender 仅治理工具、授权、执行边界与证据，不运行 Agent Loop。

## 当前基线（只读核验）

- `main`，HEAD `5feb579`；已跟踪文件无改动；四个既有未跟踪目录 `.agents/`、`.commandcode/`、`.intent-loop/`、`.pi/` 不纳入本阶段提交。
- 开发 Wails 与 MCP Tunnel 正在运行。真实 SQLite `agent_schema_migrations` 为 v18，`agent_tasks` 的 `completed/failed/unknown` 分别 30/28/8；`PRAGMA quick_check=ok`，`PRAGMA foreign_key_check` 无结果。禁止把此快照当作持续固定状态，禁止直接修改生产 DB。
- 最近已完成 Agy Parent→Child 的真实 MCP Resume 验收，见 `agy-headless-native-session-continuation-2026-10-20.md`。本阶段不重做或重放既有任务。

## 代码职责矩阵与实际不变量

| 能力 | 责任所在 | 现状 |
| --- | --- | --- |
| Native ID / SessionRef 与归属 | `sqlite/execution_evidence.go`、`agent_native_sessions` | Session 身份不可变，原生 ID 不进入公开 Task |
| Session 在任务间的关联 | `agent_execution_attempts.session_ref` + `agent_task_continuations` | 已支持一个 Session → 合法顺序链上的多个 Task |
| 会话创建/加载/身份握手 | `acp/runner.go` / `antigravity/runner.go` | ACP v1 动态能力协商；Agy 恢复在 Prompt 前精确核验 ID |
| Session 恢复授权 | `application/resume.go` 与 `sqlite/native_resume.go` | 新 Task、来源身份与 Workspace 匹配、当次授权、一次性原子预约；不复用旧授权 |
| Task 执行预算、独立终态 | `application/service.go`、`sqlite/store.go` | 每个 Task 独立 TaskID / timeout / status；unknown 不自动恢复 |
| 进程和原生终态证据 | `sqlite/execution_evidence.go` | Task generation/runtime fence，TerminalSeen 和 LeaderReaped 分别记录 |
| Task 活动展示 | `sqlite/TaskObservation` | 原生进度与本地 Worker 心跳独立，公开投影不含敏感 ID |

## 经代码审计确认的治理问题

**P1：Task completed 未在持久化终态转换处要求可信证据。**

- `sqlite/store.go:Transition` 的 `running → completed` 目前仅校验 TaskID / generation / status，不要求同代 `terminal_seen` 和 `leader_reaped`；`application/service.go:run` 在任意 Executor 返回 `nil` 时直接尝试 Completed。
- 这允许错误实现或虚假的 Runner success 使 Task 显示完成。现有 `ResumePlan` 再次校验证据，能防止非法续接，但无法防止错误的 Task 自身结果。
- 收口：仅使用**已有** attempt/event 表，在 Completed 事务中验证同一代终态和进程回收证据；不要求 SessionRef 存在（Task 完成独立于 Session 身份）。运行时无法确认/持久化完成证据时，将任务安全归类为 `unknown`，不保存未证实结果，不能自动重试。保持 Failed/Cancelled/Unknown 各自已有准入与含义。
- 测试包括：仅 Session 存在、仅终态、仅进程回收、无 attempt、stale generation 均不得 Completed；有完整证据可 Completed；Runner 错误报告 success 但无证据须 Unknown。历史迁移测试只构造历史数据，不向生产引入宽松兼容路径。

**P2：Session 可续接能力投影忽略受治理的隔离边界。**

- `sqlite/execution_evidence.go:TaskObservation` 对 ACP 的 `load_supported/resume_supported` 只看 capability，未考虑 `restricted_host` 的一次性私有 HOME 会在执行后删除；服务端 `application/resume.go` 实际禁止 restricted-host 续接。
- 收口：沿用 `unavailable/unverified` 既有枚举（不新增状态），仅 `host` 的真实可尝试 Session 能显示 `unverified`；Agy 必须继续满足合法 UUID。即使 `unverified`，也**不**代表可继续未知结果 Task 或已证明原 Prompt 完成。

## 实施分段与验收合同

1. **A 文档先行**：独立提交这份职责矩阵，明确无需新增 Session 表、状态机、ACP v2 抽象或权限路径。
2. **B Session 投影修正**：在 `TaskObservation` 限制 Host；补充 Host/Restricted 与 Agy 回归。旧数据库无需迁移；旧记录不回写。
3. **C Task 可信终态加强**：`Transition` 在 Completed 的同一原子 SQL 更新中核验 attempt + native terminal + leader reap，`Service.run` 对完成证据失效 Fail Closed 到 `unknown`；补充错误成功、并发代际、持久化证据故障测试。原有 ACP v1/Agy Runner 不更改协议与 Prompt 发送逻辑。
4. **D 综合验证与清理**：`go test ./backend/...`、`pnpm go:lint`、相关 Go Race、SQLite/ACP v1/Agy Resume 回归、Vitest、SDK/Wails binding 检查。确保工作树仅包含约定改动；独立提交验收文档。

## 不做的事

- ACP v2、v1/v2 双栈、前瞻性兼容桥、自动原 Prompt replay、`unknown` 转成功、外部 SessionID 透传、Agent 内部 Loop/推理调度。
- 不改生产 SQLite、运行中的用户任务、权限配置、Profile ToolBinding、现有 Agent ToolID；不因修正 Session 投影引入新 UI 表单或额外操作流程。

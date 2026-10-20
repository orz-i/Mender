# 2026-10-20 Session/Task 新版 Wails 真实 MCP 验收与 Agy 续接诊断收口

## 范围与基线

- 本文记录用户按 `e2b6a0d` / `b7f4447` 重新构建重启 Wails 之后的**新**实机验收。原有历史 `unknown` Prompt 绝不发送或续接。
- Wails 新实例 PID `96246`、Tunnel PID `96453`；MCP 本地端点响应；源码 `main`，Agent SQLite v18、最初 Profile ToolBindings 32、历史 `unknown=8`、数据库 `quick_check=ok` / FK 错误 0。四个既有未跟踪工具目录保持不纳入提交。
- 仅委派严格无工具、无文件读取/写入/命令执行的标记型新测试任务；授权配置、Agent 默认模型/努力、旧 Task 原生 Session 数据和历史状态都未改写。

## 真实 Agy：新 Task 可信完成，续接在 Prompt 前 Fail Closed

| Task | 关系 | 结果 | 证据 |
| --- | --- | --- | --- |
| `atask_5b05358588ac17ddb9340dc0b42772a5` | 新 Parent | **completed**，结果 `MENDER_SESSION_TASK_DEPLOY_20261020_OK` | `session_ready`、`prompt_sent`、`terminal_seen`、`leader_reaped`，独立 600s |
| `atask_615adefcec6e65253605abc4953b929d` | Parent 的新 Child | **failed**，原始诊断过于笼统 `Agent execution failed` | `process_started` 后 45 秒内未取得身份握手，**0 prompt_sent**、**0 session_ready**，leader_reaped，独立 600s |
| `atask_223011b14a940af0e17f819f65ea3fe7` | 同 Parent 的显式安全再试 Child | **failed**，同样的无 init 结论 | 原上一失败 Child 已确认 failed/no prompt/leader reaped 才允许一次新的续接预约；此次也未发送 Prompt、已回收 |

- 真实 Agy 运行实例 PID `97098` 及续接进程 PID `97696`、`99027` 都是 `PGID=PID`、`TTY=??`、`STAT=Ss`，成功排除之前控制终端继承导致的 `T` stopped 问题。
- 不能把两次续接失败当作 Agent 回答失败：新业务 Prompt 根本没有送达。旧 Parent 的成功/结果原样保留，没触碰原有历史 `unknown`；不再为寻找成功而重派或延长握手。
- 官方 Headless 文档说明 `--conversation <id>` 能加载已有对话、`stream-json` 输出 `init`，但不能据此推断**每次新进程在任何用户 Prompt 之前**都可靠地产生可验证的 `init.conversation_id`。必须保持当前预提示词身份比对 Fail Closed；具体 CLI/账户/服务侧异常当前证据不能归因。
- Native SessionRef 是否有效和 Task 结果是否可信是独立问题，不能凭进程退出、SessionID 或父历史 Git 文件推测完成。

## 真实 ACP v1：按 Session / Task 分离成功

- 首次 OpenCode 旧 Catalog 的 `agent.delegate` Output Schema 尚无 `timeoutSeconds`，真实 MCP 返回 `output_schema_mismatch`；调用已真实创建 `atask_7bc5c1b8a4d2fb280d8e152b7091d2da`，最终 **failed** 于 Prompt 前的 `Configured Agent reasoning effort not advertised by ACP session`（Agent 默认 `xhigh` 未获本轮协商接受）。没有因外部 Tool 错误再发原 Prompt。
- 用 Mender 现有 `mender agent tools <agentId> --prepare` 主人动作刷新 OpenCode 已 Ready 的 Catalog，原 `tool_6110d9db2a2443623b7c2635a78e4c3b` ToolID 不变，Output Schema 现支持 `timeoutSeconds`；真实 ChatGPT `describe_tool` 热更新确认。Profile ToolBindings 仍 32、历史 unknown 仍 8，无新增授权。
- 实机无 Prompt ACP 模型检查取得实际协商的 current model `opencode/minimax-m3.1-flash-free` 与努力 `default`。只在测试 Task 选用该组合，**没有更改 Agent 持久化默认配置**。
- Parent `atask_6daef75219b3b96af58f9c80544045ec` **completed**、结果 `MENDER_ACP_V1_CURRENT_SESSION_OK`，`session_ready` 在 `prompt_sent` 之前，原生 `terminal_seen` + `leader_reaped` 都存在。
- Child `atask_20152dbd0c46c6a80a3810d6043a888a` **completed**、结果 `MENDER_ACP_V1_CURRENT_SESSION_OK|MENDER_ACP_V1_RESUME_OK`；新 TaskID、同一 SessionRef、独立 600s、新授权执行及可信终态均通过。**ACP v1 Parent → Child 实机验收通过**。

## 当前唯一必要的代码收口：错误分类，不修改执行机制

发现 `backend/internal/agent/application/task_failure_diagnostic.go` 已将受信任固定 `headless_resume` 阶段映射为 `Native Agent session could not be resumed`，但 `backend/internal/agent/infrastructure/antigravity/runner.go` 在**预提示词**握手无 init / 45 秒超时 / ID 不匹配时只返回裸 `agentdomain.ErrResumeUnavailable`，导致新 Child 的 `diagnostic` 退化成 `Agent execution failed`。

修正仅限：
1. 复用已有 `headless_resume` 固定阶段，将该阶段与 `ErrResumeUnavailable` sentinel 同时保留（`errors.Is` 契约不变）。只用于**发送 Prompt 之前**的恢复准备失败；取消/超时 context 语义不改，发送后未知结果仍然 `ErrOutcomeUncertain`。
2. 增加 Antigravity Runner 真实子进程 Fixture 阶段/错误分类测试以及 Application 固定映射测试；确保无 Prompt、未创建新对话、无敏感错误文本输出。
3. 不修改 Task Status 定义、不改 SQLite、不自动重试、不宽松身份判断、不扩展 ACP v1/v2 或 Session 接口。既有历史 Child `diagnostic` 是不可变记录，**不回写**为新文案。

## 代码之外的明确遗留

- Agy Headless `--conversation` 在该环境下两次连续没能在规定 45 秒内提供预提示词 `init`。需要单独的真实 CLI/认证/网络/会话持久化证据才能讨论恢复成功；不因失败而扩权、重放或启动隐式探针。
- 先前已经记录的「首次 Prompt 发送后终态证据丢失时，部分协议错误可能进入 failed」是独立的 Task 不确定性分类问题，不混入本次 Agy **零 Prompt** 安全诊断收口。
- 新版若再次代码修改，仍需使用者重建运行中的 Wails 后才能验收新诊断；源码测试通过不等于已部署。

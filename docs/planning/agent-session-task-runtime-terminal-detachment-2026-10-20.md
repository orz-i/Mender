# 2026-10-20 部署后 Agy TTY 作业控制阻断：修复契约

## 实机证据

- 新 Wails Dev 运行实例 PID 69497 继承交互终端 `ttys001`。MCP 可调用，历史 `unknown=8`。
- 通过真实 Mender MCP 委派新无工具、无文件访问的 Agy smoke Task `atask_5384a0f50d5abcefa9bc8930ac749840`。持久化 `attempt_reserved/process_started/prompt_sent`，无原生 init、progress 或 terminal，Worker 心跳持续。
- 原生 Agy PID 73629 的 `TTY=ttys001`、`PGID=73629`、`TPGID=69307`，持续 `STAT=T`。前台组不是 Agent；说明继承控制终端且处于后台进程组。具体停止信号（例如 SIGTTIN）未经直接信号跟踪，仍标为推定而非确定。
- 当前 `backend/internal/runtimegovernance/infrastructure/processisolation/command_unix.go` 使用 `Setpgid:true`，保留了控制终端的继承，存在后台组终端读取被 job control 暂停的风险。
- 通过真实 `agent.cancel` 清理新 smoke：Task 变为 `cancelled`，主进程回收，但无 `terminal_seen`，不能称其成功。观测到独立的 Agy `--bg-updater` 进程，不声称所有子孙进程停止。没有重发 Prompt，也未动原先 `unknown` 历史记录。
- 对原 `unknown` 使用 `agent.resume` 做负向门禁验证：返回不可续接，Task 总数、续接数不变。

## 最小解决方案

1. **进程隔离层修复**：Unix `configureCommand` 改用 `syscall.SysProcAttr{Setsid:true}` 而不是 `Setpgid:true`。新 Session Leader 自带 PGID=PID，避免继承父进程控制终端，同时仍允许原 `kill(-PID,SIGKILL)` 进程组取消；Windows 不变。
2. **测试**：小型 Unix test 明确验证独立 OS Session、PGID 和取消组行为；纯本地、临时目录，不执行真实 Agent/生产数据。覆盖 Host/Restricted Host/ACP v1/Agy/Bootstrap/Wails 回归。
3. **边界**：不更改 Agent Loop、任务预算、SQLite Schema、Session ID、Profile grants、ToolID，不引入 PID 探测自动恢复或重放。Worker Heartbeat 仍仅代表本地监督者活着。
4. **部署要求**：源码和 Go 测试 PASS 不证明目前正在运行的 Wails 使用新代码；后续重建重连后，再进行一个新的无工具 Parent→Child MCP 终态验收，不重放旧 Prompt。

## 验收合同

- 在父进程继承交互 TTY 的真实 Wails 运行场景里，新启动的 Agent 子进程不继承控制终端（TTY 应为 `??`）。
- 进程组取消仍有效；Task 的 completed 始终需要 `terminal_seen` 和 `leader_reaped`；unknown 不因 Session 存在变为 completed。
- 先文档提交，再实施/测试/分段提交；不修改已存在的用户 Session/Task/授权数据。

## 实施与实际验收（2026-10-20）

- 文档先行提交：`34a2536`；最小源码提交：`b7f4447`。正式改动仅 `command_unix.go` 的进程 Session 启动选项与新增 `command_unix_test.go`。未引入新的数据库表、Agent/Task 状态、授权或协议兼容分支。
- 新回归在修复前确认 **FAIL**（`Setsid=false / Setpgid=true`），修复后 **PASS**。测试校验 `Setsid=true`、子进程 PGID=PID、终端 `TTY=??` 和组级取消。使用 `/usr/bin/script` 的虚拟交互终端（PTY）运行已编译测试，**PASS**，避免只在 CI 无 TTY 环境中得到无意义的空终端结果。
- 完整 `pnpm check` **退出码 0**：Go lint 0 issues、全量 Go tests、全量 Go Race、ACP v1、Agy、Host/Restricted Host 隔离、SDK、Wails binding / architecture、ESLint、TypeScript、Vitest 与 Web/Desktop build。另执行相关进程隔离、ACP 与 Agy 指向性 Go Race，均通过。前端打包体积提醒属于非致命现有警告。
- 额外的独立真实 CLI + PTY 无工具试验 **没有达到业务成功验收**，如实保留两项证据：`TestRealAntigravityNoToolMenderNativeLauncherAndRunner` 在 model inspection 阶段返回 `agent backend unavailable`；`TestRealAntigravityHeadlessContinuationWithPrePromptIdentity` 的第一轮 Headless 返回 `headless_protocol`（没有可信 init/terminal）。两次测试中子进程均为 `TTY=??`、非 `T` 停止，说明 OS 终端作业控制问题与尚未解释的 CLI 端服务/协议无响应需分开处理。不能把它们计为 Parent→Child PASS，也不能据此猜测认证、网络、版本或模型服务的具体原因。
- MCP 只读复核确认新增烟测最终 `cancelled`，`leader_reaped=true`、`terminal_seen` 缺失，仍不能声称工作成功；历史 `unknown=8` 保持不变，原已完成 Agy Parent/Child 继续保持原有可信证据。
- 本次新增代码尚未进入运行中的 Wails 进程。需要再次在本机重建并重连，之后再观察新的无工具 Agent 子进程 TTY/运行过程及原生终态。

## 隔离的后续 P1 候选：Prompt 送达后缺少可信终态

本轮独立真实 Agy Headless 在业务 Prompt 已送达后得到 `headless_protocol`。代码核查发现：首轮 `antigravity.Runner` 对某些缺失/损坏终态的协议错误仍以普通失败阶段返回，而已有 Agy Resume 对同类终态证据缺失返回 `ErrOutcomeUncertain`。由 Application 将这种普通错误持久化为 `failed`，可能错误地掩盖 Prompt 已有副作用却没有可信终态的事实。ACP v1 也需要独立检查这项不变量。

这是**与终端 Session 隔离热修不同**的 Task 终态证据分类议题。本次仅立项记录，不在 OS 修复中混入新的协议失败分类、数据迁移或状态机。下一轮应先为首轮 ACP/Agy「已派发 Prompt + 原生终态丢失」写精准反例，区别明确原生拒绝和结果不确定，再独立修正及验收。

## 同期 Profile 绑定变化（只读观察）

本轮开始时默认 Profile 的工具绑定数为 **31**，收尾时读到 **32**；MCP 来源发现结果相应地显示 Grok Agent 可见工具从 **4** 变为 **5**（多了原已存在于 Catalog 的 `agent.resume`）。Grok 工具的 Catalog 元数据在 UTC 08:36:05 更新。现有 `profile_tools` 无单条创建时间/操作者字段，因此**不能从这些只读数据归因**这次 Profile 绑定改变是用户显式操作、其他会话更新或同步行为。本轮执行者没有调用 Profile 修改工具，也没有回滚、追加或改写此绑定。应保留此审计差异供后续权限核对，不宣称 Profile 绑定数量从始至终未变。

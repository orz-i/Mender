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

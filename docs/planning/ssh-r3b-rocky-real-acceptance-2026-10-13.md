# R3b SSH Durable Jobs — Rocky Linux 实机验收报告

- 验收日期：2026-10-14（远端及 Mac mini 显示时区 CST；对应 2026-10-13 美国太平洋时区）
- 阶段：**R3b 实机核心闭环通过（正常执行 + 启动前异常恢复 + 迟到执行围栏 + 生命周期）**
- 验收方式：通过已连接的 ChatGPT/Mender Rocky SSH Tool、Mac mini 上的 Mender 原生 Runtime/CLI，以及专门的 `liveacceptance` Go 构建标签测试；没有伪造远端系统服务执行结果。
- Mender 定位不变：面向 LLM/Agent 的工具和资源聚合、治理及供应；SSH Backend 拥有 TaskID、Journal、systemd 执行与恢复证据。Core 仅管理身份、Policy、Scope、租约、审计和 Sandbox 生命周期。
- 原有 Rocky 资源：`sbx_1b80d48ebb2591ecae6df534d9721e63`。不对它提交新任务，也不修改其旧版异常记录。
- 临时验收资源：`sbx_a89d88098ca8da03a4421cfd51cd98a8`、`sbx_7eb4ae89e6493dfec2b709b9b5ff8283`，均有独立的远端 `SandboxID/TaskID` 命名空间。
- **运行环境说明：** v2 使用单独授权的本机验收 Runtime，并在该进程中设定 `MENDER_SSH_JOBS_V2=enabled`；没有为当前 Wails 应用永久开启 v2，也没有改动它的启动配置。默认运行仍不应被视为已自动切换 v2。

## 1. Rocky Linux 真实环境确认

| 检查 | 实机结果 |
| --- | --- |
| Linux Kernel | `5.14.0-687.17.1.el9_8.x86_64` |
| systemd | `252-67.el9_8.4.rocky.0.1` |
| util-linux flock | `2.37.4` |
| GNU coreutils sync/mv | `8.32` |
| 私有 Jobs 存储文件系统 | `ext2/ext3`（`stat -f -c %T`） |
| `systemctl is-system-running` | `running` |
| 真实 `fsync@openssh.com` | 由 SSH Backend `CheckV2` 在固定可信主机与真实 SFTP 会话中成功验证 |
| 已绑定的登录/主机公钥身份 | 已使用 Mender 原有固定 SSH Host Key 和凭据链路完成认证，未导出任何密钥字节 |

## 2. 正常 v2 systemd 长生命周期

仅在临时资源 `sbx_a89d88098ca8da03a4421cfd51cd98a8` 下创建唯一新任务：

- TaskID：`sjob_ab1421a1b2021b608b112058f514ed5c`
- 后端真实执行：`systemd-run`，初始 `running` → 后续 `completed`，持久化 exit=0。
- 读取实际 stdout 获得固定验收标记 `mender-r3b-rocky-real-acceptance`。
- 使用相同随机幂等键再次提交，返回原 TaskID 的 `completed`，未产生第二次运行。
- `job-admission --pause` 后新键提交由 Core 直接拒绝；不需要触碰远端任务。
- 远端 `Quiescent` 得到有效证明，正式 Stop → Destroy 成功，独立验收资源最终为 `destroyed`。
- 本项带标签的真实测试返回退出码 0，运行约 187 秒。

## 3. 真实启动前异常注入与恢复

先创建第二个专用资源 `sbx_7eb4ae89e6493dfec2b709b9b5ff8283`。任务 `sjob_939764b1fb9eb39257069de5f44f5a35` 正常完成，使这一新资源拥有持久化的 Core SSH Jobs 生命周期义务。

仅对该资源新建另一随机 TaskID `sjob_3166db5c8956e6accdc35d4e12b18c73` 进行受控、包内部的故障注入：

- 使用真实 SSH/SFTP 连接成功写入 v2 Manifest 及 `run.sh`。
- 在脚本 SFTP fsync 与目录持久化确认边界注入丢失的 ACK，返回 `ErrJobUnknown`，且 **`launch.intent` 尚未写入、`systemd-run` 从未调用**。
- 真实 `Inspect` 报告：`prelaunch_candidate` / `unknown` / `uncertain`，`recoverable=true`。`Quiescent` 在恢复前严格拒绝。
- 正式 Core Stop 被拒绝（CLI exit=5）；Resource Run 仍为 `ready`，未伪造任何终态。
- Core `job-admission --pause` 持久化后，重新读取仍 `paused=true`。
- 使用错误的证据 SHA-256 Fingerprint 请求 Resolve：被拒绝（CLI exit=5，`ErrJobConflict`），远端恢复文件未被写入。
- 使用最新正确指纹与显式审核原因，经所有者 `job-resolve --confirm`，真实 SSH Backend 在同一 `launch.lock` 上取锁、核验 Manifest/Script/Launch、写入并持久化 `resolution.json`。
- 后续独立读回及文件/目录 fsync 证明：`resolved_before_launch`；`activity=quiescent_proved`；**`state=unknown`、无伪造 exit**，`recoverable=false`。
- Core 仅持有短期租约；恢复审计事件结果为 `unknown → conflict → unknown → resolved`，记录原因摘要而非原文；最终没有残留执行租约。

## 4. 真实的迟到 systemd-run 安全验证

在**已经持久化 Resolve 之后**，对该专用 TaskID 的预制脚本发送一次实际的迟到 `systemd-run --collect --service-type=exec` 命令，测试单元为 `mender-sjob_3166db5c8956e6accdc35d4e12b18c73.service`。

| 实机观测 | 值 |
| --- | --- |
| 迟到请求发送前的 Unit LoadState | `not-found` |
| 真实 systemd-run 请求 | 接受 |
| 运行后 Unit ActiveState | `inactive` |
| `start.intent` | 未产生 |
| `stdout` | 未产生 |
| `exit` | 未产生 |
| 用户命令是否执行 | **否：v2 gate 检测到 immutable resolution 并退出** |

这证明恢复完成后，旧分发请求到达执行点也不能越过同一锁及裁决标记。在迟到 systemd 单元完成后再次检查，两个 TaskID 分别报告 `terminal` 与 `resolved_before_launch`，Sandbox 全部 `quiescent_proved`；正常 Stop 成功，`sandbox destroy --confirm` 成功。第二个资源最终 `destroyed`，没有活跃执行租约。

## 5. 旧版记录保护及工作区清理

- 原 Rocky Sandbox `sbx_1b80d48ebb2591ecae6df534d9721e63`：最终仍 `ready`，任务数仍为 5，未被 Pause/Stop/Destroy。
- 原 v1 异常 TaskID `sjob_29deeed6015ee33f63cd35f5a7038e1d`：始终 `manifest_only` / `unknown` / `uncertain`；证据指纹与验收前完全一致。没有清理、更名、重放或强制解除。
- 两个新建验收 Sandbox 都已逻辑销毁、没有活跃租约。测试 TaskID 的远端持久化墓碑按设计保留，**没有删除宿主机文件**。
- 没有触及其他用户/生产任务、远端系统配置、SSH 私钥或 Host Key。
- 相关源文件和测试均应经 `gofmt`、`pnpm go:lint`、带 `liveacceptance` 标签的无授权默认跳过测试，以及全量 `pnpm check` 复核。

## 6. 边界与仍需独立验证的故障

**结论：R3b 在受信任 Rocky Linux/systemd 252 上的核心提交、真实 SSH/SFTP 持久化、受控预启动异常 Resolve、迟到系统服务启动围栏和完整生命周期的实机验收通过。**

此次没有让真实主机断电、强制重启、破坏实际物理网络链路，亦没有执行危险的磁盘/文件系统持久化灾难测试。故障注入针对真实远端 SFTP Journal 的明确写入 ACK 边界，发生于 `systemd-run` 调用之前；它不是整机断电的替代证据。R3b 不提供跨任意用户命令的 exactly-once 保证；root 登录并非隔离。

当前 Wails 进程未观察到 `MENDER_SSH_JOBS_V2=enabled` 环境开关，因此**实机验收成功不会自动让生产 Wails Profile 的 SSH submit 改为 v2**。该配置切换应作为后续明确的运行时发布操作单独进行，而不是由验收脚本私自修改。SSH Backend 的 64 个 TaskID 目录限额和 v1 历史 `unknown` 应保持原有保守治理。

## 验收脚本（复用前须审查）

- `backend/internal/bootstrap/ssh_r3b_live_acceptance_test.go`：`//go:build liveacceptance`，使用显式运行时数据库及原 Rocky Template，执行前预检、真实 v2 canary 与生命周期验收，或创建用于故障注入的专用 Sandbox。
- `backend/internal/sandbox/infrastructure/sshremote/ssh_r3b_live_fault_test.go`：`//go:build liveacceptance`，严格只接受名称为 `R3b Rocky acceptance temporary` 且 Run=`ready`、generation=1、仅已有 1 条 terminal record 的专用 Sandbox；使用已授权本地 Keychain 连接真实 SSH，在每个阶段固定的持久化回执边界返回 `unknown`，且明确断言从未调用 systemd-run。
- 默认测试没有对应显式环境授权时直接跳过。切勿将此用例指向原用户 Rocky Sandbox。

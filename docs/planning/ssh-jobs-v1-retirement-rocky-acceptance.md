# SSH Jobs v1 退休与 Rocky v2-only 存储切换验收

- 执行日期：2026-10-14～15（Mac mini / Rocky CST，时区可能与 ChatGPT 用户当地日期不同）
- 结果：**生产 SSH Jobs v1 原始数据清理完成，运行时代码硬切为 v2-only，实际云端提交验证通过**
- 目标 Sandbox：`sbx_1b80d48ebb2591ecae6df534d9721e63`
- 原则：不运行旧任务、不清除 Core 治理审计、不伪造历史 `unknown` 为成功或失败；绝不将 v1 数据清理当成远端命令未执行的证明。

## 代码硬切与质量门禁

| 提交 | 内容 |
| --- | --- |
| `e049f68` | 文档先行，定义旧路径原子封锁、TaskID 墓碑、v2 新根目录及中止条件 |
| `6d3c3e0` | 删除 v1 Submit、Status/Manifest 解析、恢复分支和相关测试；SSH Backend 仅执行、观察、恢复 v2。新增与协议无关的 `.retired-ids` 防重放墓碑 |
| `0a50e3e` | Core 移除 v1/v2 选择器与激活 API，SQLite 迁移到 Schema 18；保留 Admission Pause、Policy、Owner、租约及恢复审计 |
| `27862b8` | 删除旧 CLI、HTTP、Wails、SDK、OpenAPI、UI 中的协议选择/激活操作及相关兼容逻辑 |

完整 `pnpm check`（含 Go Lint/全部测试/Race、SDK 生成、Wails Bindings、前端类型检查、测试和构建）通过，`exit=0`。在真实生产 Mender 数据库的隔离副本中进行了 v17→v18 迁移测试，`PRAGMA integrity_check=ok`。生产数据库在实际切换前生成了本地受限备份：`~/Library/Application Support/Mender/backups/mender-pre-v1-retire-20261014-232020.db`，模式 `0600`，SQLite 完整性检查通过。备份未上传或写入 Git。

**ID 稳定性例外：** `mender:ssh-job:v1:` 旧字符串继续作为不可见的 TaskID 哈希域分隔符，**仅为防止已接受的旧幂等键映射成新 TaskID 导致命令重新执行**。这不是 v1 执行、解析或协议兼容桥；强行重置该盐值会破坏防重放不变量。

## Rocky 实机操作记录

生产环境初始状态：Schema 18、Run `ready`、`paused=true`、0 活跃执行租约。Rocky 远端原私有根目录 `.mender-ssh-jobs-v1` 覆盖 3 个 Sandbox，记录共 10 条：**5 条 v1（4 条终态、1 条 manifest-only/unknown）和 5 条 v2**。没有处于 active 的受管理 systemd service。

首次网络异常期间只执行了读取探针；没有复制、删除或重命名。SSH TCP 恢复后，实际操作顺序如下：

1. 读取并逐项验证全部 10 条原有记录，拒绝符号链接、开放权限或无效 Manifest；验证 v2 脚本 SHA-256、真实终态/恢复证据，以及所有精确 systemd Unit 都不在运行。
2. 在独立 `.mender-ssh-jobs-v2/<SandboxID>/` 创建受限目录，原样复制 **5 条 v2 Journal** 的每个文件，并将文件与目录逐级 fsync、回读逐字节校验。没有改写旧版 v2 Manifest / Script Digest / Exit。
3. 为 **5 条退休 v1 TaskID** 写入 `.mender-ssh-jobs-v2/<SandboxID>/.retired-ids/<TaskID>`，每条仅含固定内容 `retired\n`。不存在旧命令、用户输出、原 Manifest 或 SSH 凭据，且标记文件写入、fsync 后仍可验证；旧 ID 不会被 v2 Submit 重新分配。
4. 在隔离的 Linux `/tmp` 夹具验证 `renameat2(RENAME_EXCHANGE)` 能原子交换非空目录与普通文件。然后在实际宿主机将旧根目录与 fsync 后的受限保护文件 **原子交换**。旧绝对路径此后不再可遍历，迟到的 v1/v2 系统脚本无法访问原 TaskID 路径；旧内容进入固定私有隔离位置。
5. 在隔离区仍存在时，用 **当前 v2-only Runtime** 从新根目录验证原 Rocky Sandbox 的两条现存 v2 任务均为 `terminal/completed/quiescent_proved`，`managed=true`、`paused=true`。
6. 全量复查隔离区 10 条原始记录：每个 v2 新副本文件与旧内容逐字节一致；每个 v1 的原始 TaskID 对应唯一最小墓碑；没有未知目录、符号链接或运行中的服务。随后只删除已验证的私有隔离区，fsync 其父目录。
7. 删除结果：**5 条 v1 原始 Task 目录**与 **5 条旧根目录 v2 冗余副本**消失。正式新根目录的 **5 条 v2 完整 Journal** 保留、**5 条退休 ID 墓碑**保留，原旧根目录位置只剩 **56 字节不可遍历的保护文件**；隔离区不存在。旧 v1 原始数据在远端共清理 **5,011 字节**（常规文件系统删除，不宣称磁盘物理擦除）。
8. 确认生产 Schema 18、`ready`、0 执行租约，恢复 `paused=false`。原 Tunnel 和 Profile 均未改动，也没有新建 Provider、Agent Loop、通用 RemoteTask 或 Credential。

## ChatGPT 云端最终验收

通过用户实际连接的 **ChatGPT → OpenAI Tunnel → Mender Profile → SSH Jobs → Rocky systemd/SFTP**：

- 使用新随机 128-bit idempotency key 发出只包含 `printf` 的任务。
- 新 TaskID：`sjob_1618d76ba93daa0c1dcbfd26f6a13532`。
- 云端 `result` 返回 `completed`、`exitCode=0`；`logs` 返回固定标记 `mender-v2-only-retirement-canary\n`、cursor=33、`truncated=false`。
- 真实远端新路径 `.mender-ssh-jobs-v2/.../<TaskID>/manifest.json` 的 `version=2`；旧路径不可遍历。新总数为 **6 条 v2 Journal + 5 条退休 TaskID 墓碑**；没有旧隔离目录。
- 同一幂等键及**完全相同**命令再次调用，直接返回同一 TaskID 的 `completed/exitCode=0`，不重复执行。
- 最终 SQLite：Schema `18`、Sandbox `ready` generation 1、`paused=false`、0 执行租约、无 `sandbox_ssh_job_protocols` / `sandbox_ssh_job_protocol_events` 旧选择器表，`PRAGMA integrity_check=ok`。

## 退休后的安全边界

- 原 `sjob_29deeed6015ee33f63cd35f5a7038e1d` **没有被裁定执行结果**。其旧版原始任务文件已根据用户要求删除；最小退休墓碑与旧路径保护文件阻止原键重复分发及迟到脚本运行。
- 旧路径仍需保留**仅 56 字节的保护文件**。它不是 v1 任务数据或兼容代码，而是防止迟到旧脚本重建/访问旧路径的永久安全围栏；删除它会削弱退休安全性。
- Core 过去的审计信息、只读备份文件以及文件系统快照可能独立保留历史元数据；本次是有验证的逻辑数据清理，**不等同于介质安全擦除**。
- 未进行整机断电或真正的物理网络分区灾难演练，也不声称恶意 root shell 具有通用 exactly-once 语义。SSH Backend 的 64 个任务配额仍适用。

**验收结论：** SSH Jobs v1 执行和兼容分支已在代码中永久退休，生产数据库硬切至 Schema 18，Rocky 的 v1 原始任务数据完成受控清理，v2-only 的云端执行闭环通过。保留的唯一旧路径痕迹是防迟到执行的固定保护文件，以及防止旧 TaskID 重放的最小版本无关墓碑。

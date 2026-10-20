# Agy CLI Headless 原生会话续接：v1.4.2 实证与实施契约

日期：2026-10-20。前轮针对 `agent.resume` 的 blanket “Agy unsupported” 判断已被真实 CLI 实验推翻；保留已有未知结果、授权和 Session 证据限制。

## 已验证的 CLI 行为

- `agy --conversation <conversation-id> --input-format stream-json --output-format stream-json --disable-slash-commands --sandbox` 可以在**新的 CLI 进程**恢复同一 Native Conversation。先前原对话与第二轮的 `init.conversation_id` 一致、`num_turns` 从 1→2、历史标记被准确复述。
- 对不存在的 ID，即使 ID 是合法 UUID，CLI 也可能退出码 0、输出 SUCCESS，但默默创建新 Conversation。这是安全风险，**不能直接把 ID 当作续接已成功**。
- 更关键：在 Resume 的 stream-json 模式中，CLI 可以在接收任何 User NDJSON 之前输出 `init.conversation_id`。真实有效 Session 初始化时 ID 匹配；不存在 ID 初始化时 ID 不匹配。通过预检后才发送 Prompt，真实 Session 成功，随机 ID 被拦截，未发送 Prompt，临时工作区无修改。
- 上述结论是该机器已安装的 `agy 1.4.2` 实测。未来版本必须依赖同一严格前置握手；若行为变化/无预检事件，在运行时 Fail Closed，**不回退到发送 Prompt 以换取 init**。

## 安全实现边界

1. 仍然是“一个已完成 Task → 同一已验证原生 Conversation 的新 Prompt”，**不是**恢复正在执行中的中断 Task，也不是再次执行旧的 Prompt。`unknown`、`running`、`cancelled`、缺 `terminal_seen` / `leader_reaped` 的 Task 继续不能 Resume。
2. 沿用现有 `SessionContinuation`、`CreateResumedTask` 的完整 Task/Agent/Principal/Scope/Workspace/NativeSessionId/Generation/一次性预约，且保持 Host-only。CLI 启动前仍按 Mender Policy 与 ProfileGrant 验证 `AgentTaskResume`、`Activate`、`Submit`。
3. Launcher 从 Mender 私有存储的原生 Session ID 组装**验证过的字面 argv** `--conversation <canonical UUID>`；不能接受用户或模型传入的 CLI 任意参数。
4. Runner 在已认证的配置下启动进程，记录 `process_started`，**必须等待**第一个正确的 `init.conversation_id` 与预期 ID 全等，且在发送任何 Prompt 前将 `ExistingSessionRef` 与匹配 Native ID 原子记录至现有执行证据。
5. 缺失/格式错误/超时/不匹配的首个 `init`、证据写入失败，一律在发送 Prompt 前拒绝。对已入库的新续接 Task 可以安全记为 failed，不能伪造完成。若**已发送** Prompt 后丢失结果/连接，则仍然以 `unknown` 保留不确定性。
6. 新产生的有效 Agy UUID Native Sessions 标记支持 continuation；旧版 Mender 创建但标记 `load_supported=0` 的 Native Sessions 不强制重写或迁移历史证据——仅在合法 UUID、Agent host、严格所有者与终态条件下可进入**前置身份核验**，最终安全性仍取决于当次 CLI 初始化 ID 一致。
7. 不扩展 Agy 的 ACP 回调、Mender ToolSupply 和 Trusted 授权；原有手动/Trusted 模式依据**新的 Task**重新检查，绝不自动继承历史授权；不做“自动继续”或未知任务重复派发。
8. `agent.resume` ToolID 与已有 Profile 绑定保持不变，只更新支持 Agy host 的动态 Tool 元数据说明。Restricted Host 仍标记不可用。

## 验收矩阵

- Agy Runner fixture：正确 ID、初始化早于 Prompt、唯一 SessionRef 持久化、成功后新 Task 完成；错误 ID、缺 init、格式错误、init 阶段错误、取消/超时一律 zero prompts，并且不落入新对话执行。
- Service/SQLite：只允许 Host + proven-complete 同 Agent / 同 Principal / 相同已授权工作区；旧历史不自动修改但合法已存 UUID 可请求验证；未知/无终态/跨来源/撤权/重复续接均严格拒绝。
- Launcher：验证 UUID 值，仅通过 `--conversation` 传入，拒绝空、dash、非 UUID、含换行和其他注入，无 Prompt/credential 出现在 argv。
- 完整 Go/Bootstrap/MCP 回归、Race 重点回归、Go Lint；隔离的真实 Agy CLI 测试验证两个独立进程恢复记忆以及假 ID 预检不派发任务。生产数据库不更改，旧任务不重放。

## 与此前结论的差别

`docs/planning/agent-task-budget-observation-2026-10-20.md`、`docs/planning/mcp-schema-refresh-and-agent-resume-p1-2026-10-20.md` 中把 Agy 标为“永远不支持原生 Resume”的描述是**旧版实现状态，不是 CLI 能力事实**。新实测证明 Headless 多轮会话支持，并提供可安全用于预提示词校验的 Native `init`。仍不支持对未知结果做可信终态恢复。
## 实施与验证结果（2026-10-20）

### 源码落地

- `efc439b` — Host Agy 新增受验证的原生 `--conversation` 续接。任务准入沿用 Mender 已有 Task、Agent、Principal、Scope、Workspace、Profile/Policy 授权、Native SessionRef、原始 completed/terminal_seen/leader_reaped 证据以及单次预约约束。原有 5 个 Canonical Agent ToolID 不变，Restricted Host 仍禁止。
- `e9ebffd` — Domain UUID 校验、Launcher 只允许字面 `--conversation <UUID>`、Runner 在**发送业务 Prompt 之前**强制比较 `init.conversation_id` 并入库确认 SessionRef。无/错 `init`、虚假新会话回退、证据写入失败和预算截止都禁止派发 Prompt；派发后未获得可信终态仍为结果未知。
- 新产生的 UUID 格式 Agy Session 标记为可尝试续接；历史存储的 Agy Session 能力位为 0 的记录**不批量修改**，只允许合法 Host UUID 在真实 CLI 身份核验阶段尝试进入续接。旧 `unknown` 仍严格不允许 Resume。
- 发现 Headless CLI 在某些受限环境下可能无输出停顿；新增 45 秒独立预提示词握手截止，在超时/取消时使用原有进程组终止机制，不会持续占满新任务的 10/20/30 分钟预算。没有放宽运行环境、权限或新增 Agent 调度模式。

### 真实 CLI、Mender Runner 验收

- 实际测试命令：`MENDER_REAL_AGY_CONTINUE_NO_TOOL=1 go test ./backend/internal/agent/infrastructure/antigravity -run '^TestRealAntigravityHeadlessContinuationWithPrePromptIdentity$' -count=1 -timeout=76s -v`。
- **PASS (29.65s)**：独立第一个原生 Agy CLI 进程创建 Host 对话，第二个 CLI 进程通过同一个 Session ID 按 `--conversation` 续接，回答能读到上一轮 marker；新 Prompt 只在 `init` 身份严格匹配、Observation 确认后发出；不存在的合法 UUID 被拒绝，Prompt 不发出。运行完成后未发现本轮残留的 `agy --input-format` 进程。
- 此前相同测试曾因最小化启动环境中 Agy 长时间无终态而中断，并留下两个仅属于本轮临时工作目录的进程；其身份经 cwd 和 argv 双重确认后已清理。增加有界测试/进程清理后本轮实机通过。这是一个环境相关的**间歇性风险**，不能从一次 PASS 推断所有网络条件稳定；若后续重复出现需独立诊断 CLI 认证/网络，严禁直接透传不受控的宿主环境变量。
- Mender 的正常业务仓库、用户历史 Agent Session、任务与凭据未用于这次隔离 Prompt；真实测试使用一次性本机临时目录和明确“不调用工具、不访问文件”的短提示词。

### 隔离安全覆盖

- Domain：合法/非法 UUID 及参数注入约束。
- Agy Runner：正确会话在 Prompt 前绑定 SessionRef；错误会话、新会话回退、无 init、非法帧、无进度、记录 SessionRef 失败、取消/超时没有业务 Prompt；Prompt 派发后终态身份错误转换为 `ErrOutcomeUncertain`。
- SQLite：旧版 Agy 能力位 0、但具备合法 UUID 的已完成 Host Task 可参与受限制的前置校验；历史 Task 不被迁移或重写，重复续接被防重门禁拒绝。
- Bootstrap/MCP：只给显式绑定 Resume 的 Profile 许可；继续创建新的 Child Task，沿用同一私有原生 SessionRef；父任务结果保留；不向 MCP 返回 Native Conversation ID；不可用的 Restricted Host 仍标注不可用。

### 后续部署验收边界

1. 源码和隔离原生 CLI 测试已通过，但**当前运行中的 Wails Desktop 尚未重建为这些提交**。用户构建后须显式进入 Agent → 工具 → 刷新智能体工具，验证已存在的 ToolID 不旋转，运行中的 MCP Server 由既有 Catalog 版本热刷新获取新说明，无须重启。
2. 仅选用一个**隔离且已完成**的 Agy Task，新授权 Profile 显式绑定 `resume` 后，执行新的无工具 Prompt，再验证 Parent/Child、Native Session 身份以及 MCP 状态与 Result；不要把这个验收和生产旧 `unknown` 的自动恢复混为一谈。
3. 本轮**未实现**对在途中断原 Prompt 的继续执行、Mender Agent Loop、原生 ACP 审批、自动重派或结果猜测。未知终态继续保留 `unknown` 和人可审核的执行证据。

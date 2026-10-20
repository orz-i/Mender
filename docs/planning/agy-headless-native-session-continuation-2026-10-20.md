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

# Mender P1 评估：运行中 MCP 工具 Schema 与 Agy Resume 诊断

日期：2026-10-20（Mac 本地时间）
性质：只读评估，尚未修改任何运行时代码、生产配置、Profile 绑定或历史任务。

## 结论

两项都维持 P1。A 是 MCP 运行投影的一致性缺口；B 是可信的能力限制在通用 Invocation 错误通道里变成泛化 Provider 故障。都可复用现有机制治理，无需新的 Agent 状态、调度器或用户配置。

## A：Agent 工具同步后 MCP 未立即更新

### 已确认根因

1. backend/internal/bootstrap/agent_tools.go 的 EnsureAgentTools 会通过 syncConfiguredProviderForOwner 更新 Catalog 工具 Schema/描述/标签，且不会修改 Profile ToolBinding。
2. backend/internal/catalog/infrastructure/sqlite/migrations.go 中已有 catalog_tool_revision，Tool 的 INSERT/UPDATE/DELETE 触发器维护版本；revision.go 提供 CatalogRevision。当前只读数据库观测到 Catalog Revision 213、默认 Profile Revision 9、44 个 Tool、31 个 Profile ToolBinding。
3. backend/internal/bootstrap/mcp_profile_refresh.go 的 managedProfileMCPHTTPHandler 只检查 Profile Revision。Profile 未变就返回旧 immutable HTTP Handler。
4. backend/internal/mcp/infrastructure/server/server.go 的 Adapter.New 构建时缓存 Profile 已绑定工具定义。meta.go 的 describeHandler 优先读取缓存；其 calculateFingerprint 虽加入 CatalogRevision，但只保护 source-first 分页 cursor，不能使缓存的工具对象变新。
5. 已重现实例：Wails 刷新 Agy 工具后 Catalog Schema 已新、同一 ChatGPT 连接仍见旧值；手动重启 default MCP Server 后恢复。

### 方案比较

- A0 让用户每次同步工具后重启 MCP：零代码但增加使用成本和连接中断，不应作为最终方案。
- A1 合并 Profile Revision 和全局 CatalogRevision 为运行快照版本键：实现简单，跨 Runtime 生效。但当前 SyncProviderTools 无差异检查，Store.ReplaceProviderTools 会先删后插，即使相同定义也推动 CatalogRevision 增长；不相关来源的同步可能迫使全部 Profile 重建和重新激活 Provider。
- A2（推荐）以现有 CatalogRevision 为廉价脏标记；在版本变化时仅对当前 Profile 的已绑定 Tool 输入/输出 Schema、标题、描述、标签等有效元数据计算内容指纹。若实际内容不变则推进已核对版本，不重复构建；若确有变化，在 managedProfileMCPHTTPHandler 已有 mutex/atomic 机制下重建投影。正常请求保持 O(1) 的版本读取快路径，且无需新数据库表或用户参数。
- A3 只发送 MCP notifications/tools/list_changed：不能修复服务器自己持有的旧 Adapter，适合作为 A2 后的协议增强，而不是独立修复。

### 边界和验收

- 同一 HTTP MCP 会话、Listener 和 Tunnel 不变，不改 Profile Revision 或 ToolID。已同步 Schema 后下一次 mender_describe_tool 或（Direct/Hybrid）tools/list 应返回新内容。
- 构建前后双重检查 ProfileRevision 与 CatalogRevision，在两个 Runtime 并发提交时不发布过期快照；出现存储/Provider Activation 故障时拒绝提供旧投影并允许下次请求重试，保留原 Invocation 的最新授权门禁。
- 相同内容重新同步及无关来源修改时，当前 Profile 不应重复构建；跨 Runtime Catalog 写入应被检测到。
- 验证 Direct / Dynamic / Hybrid、Tool 输入输出 Schema、source-first 光标失效、旧 ToolID、撤销授权、同时在途请求。
- MCP 2026-07-28 标准的通知需要客户端已订阅 subscriptions/listen 中的 toolsListChanged；Go SDK v1.8 与 ChatGPT 所使用的协议版本/订阅情况尚需验证。不以客户端通知来代替服务器端热更新。
- 此方案只覆盖 Tool 元数据更改。Provider 显示名称等不触发 catalog_tool_revision 的改变，需另行评估，不与本轮捆绑扩张。

## B：Agy Resume 只能得到 provider_error

### 已确认根因

1. backend/internal/agent/infrastructure/delegationprovider/plugin.go 的 resume 分支先校验参数，再调用 Service.Resume。
2. backend/internal/agent/application/resume.go 对 Antigravity、受限隔离或缺乏可信完成/Native Session 的 Task 返回 domain.ErrResumeUnavailable；不创建新 Task，是正确的安全行为。
3. domain.ErrResumeUnavailable 是普通 errors.New，没有 DiagnosticSummary 方法。backend/internal/invocation/application/error_summary.go 只接受实现此接口的可信错误，防止原生错误泄露敏感路径/凭据；invoke_tool.go 因此使用缺省 failed/provider/provider_error/provider execution failed。
4. backend/internal/mcp/infrastructure/server/server.go 的 mcpResult 已正确映射为 MCP isError=true，说明问题发生在 Invocation 对外结果组装前，而非 MCP 传输层。
5. 当前生产 SQLite invocation_audit 的 Agy resume 记录确为 status=failed、outcome_kind=failed、failure_source=provider、failure_code=provider_error、error_summary 空。
6. 工具在 Catalog 仍保留五个 Canonical ToolID，包括 agent.resume 和显式 Profile Binding；工具 metadata 的 unavailable 标签只是说明，不禁止旧调用方尝试。

### 方案比较及推荐

- B0 全局透出 err.Error：会泄露未知 Plugin/Agent 底层错误，禁止。
- B1（推荐）在 Agent Delegation Provider 的已知错误映射处以 errors.Is 精确匹配 ErrResumeUnavailable，直接返回现有的 Failed(tool, tool_error, 固定安全说明)，例如 Session continuation is not supported for this Agent. No new task was created; do not retry the previous prompt. 可复用 ResultGuard、Audit、MCP isError=true，完全不需要新增 Task 业务状态或数据库迁移。
- B2 增加新 capability_unavailable 等机器可读 FailureCode：需要同时扩展 Invocation Domain、Audit 验证、CLI、协议与存储；暂不值得只为这一项改变闭合错误枚举。
- B3 删除 agent.resume 记录：现有 Profile ToolBinding 仍引用它，直接移除会破坏 ToolID/授权历史；不建议。

尤其注意：目前通用 InvokeTool 不接受 Executor 在成功调用通道直接返回 Rejected，强行返回 Rejected(invalid_arguments) 会导致新的内部错误。选用现有 Failed(tool,tool_error) 更符合当前架构和审计约束。

### 安全验收

- 对无续接能力的 Agy，以及受限 Host ACP / 缺可信完成证据的其他 Agent，请求必须返回固定、安全、可读的错误，不能新建 Task、启动 CLI、重放原 Prompt 或改写 old unknown。
- ErrForbidden、跨 Agent / 跨 Profile TaskID、恶意任务标识仍应保持原有门禁，不泄露原生 SessionID、Prompt、凭据和真实文件路径；未知底层错误继续保持泛化脱敏。
- 可合法续接的 Host ACP 且已证实终态和 Session 能力不应被一刀切禁用。
- 审计与 MCP kind/source/code/message 保持一致；ToolID、Profile Binding、AgentGrant、Policy 审批原样保留。
- 原生 SDK CallTool 对 isError=true 的结构化结果做测试，再在已连接的 ChatGPT Profile 观察应用是否保留清晰消息，不能只凭本地测试断言客户端展示完全一致。
- 可选后续：不可用 Agent 工具保留在 Catalog 管理与审计中，但经可靠的 Agent 后端能力判断，避免作为模型可执行工具被推荐。不能只过滤 tools/list，而遗漏 mender_search_tools、mender_describe_tool 或旧 ToolID 调用，也不能将任意第三方字符串 tag 当成通用授权规则。

## 推荐实施顺序

先 B1：局部、安全、无迁移的已知错误诊断。
再 A2：修改现有 MCP 快照缓存版本判定，先增加失败的集成测试、再处理并发和无关 Catalog 更新。
然后按实际协商能力决定是否加 tools/list_changed，不新建事件总线或重启队列。

## 官方参考

- MCP 2026-07-28 Tools：https://github.com/modelcontextprotocol/modelcontextprotocol/blob/main/docs/specification/2026-07-28/server/tools.mdx
- MCP TypeScript SDK Notifications：https://github.com/modelcontextprotocol/typescript-sdk/blob/main/docs/servers/notifications.md
- MCP SDK Error Handling：https://github.com/modelcontextprotocol/typescript-sdk/blob/main/docs/servers/errors.md

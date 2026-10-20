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

## 已实施修复与测试结果（2026-10-20）

### P1-B — 明确拒绝无法续接的 Agent 原生会话

- 提交 \`a7f4885\`：\`backend/internal/agent/infrastructure/delegationprovider/plugin.go\` 在 \`Service.Resume\` 完成既有身份、任务归属和授权检查后，只对 \`errors.Is(err, agentdomain.ErrResumeUnavailable)\` 返回现有的 \`Failed(tool, tool_error, 固定安全消息)\`。其他失败仍由原有 Invocation 脱敏通道处理，不透出不可信的 \`err.Error()\`。
- 新 \`resume_failure_test.go\` 验证：Agy 的不可续接返回明确 \`kind/source/code/message\`；没有创建新 Task；跨 Agent TaskID 仍被拒绝，无法探测其他会话。
- 提交 \`2ca9bb9\`：在真实的隔离 MCP Client Session 里，使用已显式绑定的 \`agent.resume\` 检查返回 \`isError: true\`、\`failed/tool/tool_error\` 和固定消息，并核对 Task 数量不增加。保留 Host ACP 具备证据时的正常续接测试。

### P1-A — Profile 不变时的 Catalog Schema 热刷新

- 提交 \`02068a9\`：保留现有的 \`managedProfileMCPHTTPHandler\`、mutex 和 atomic handler publish，快路径额外核对 Catalog Revision。慢路径按当前 Profile 已绑定的 ToolID 计算**不包含时间戳**的元数据指纹（ID/来源键、输入输出 Schema、标题/描述/名称/标签）。
- 当前 Profile 的工具元数据没变化：只推进“已核对 Catalog Revision”，保持原 MCP Handler，不重复激活 Provider；工具元数据确实变化：重建新的不可变 MCP Adapter，经 Profile + Catalog 版本前后检查一致才发布。
- 如果 Catalog 读失败、Tool Schema 不完整或构建期间被其他 Runtime 修改：HTTP 503、保留旧快照待下一请求重试；不使用旧快照做“看似成功”的元数据回复。旧请求可以用已获取的视图完成，但 Invocation 继续强制实时检查 Profile 授权。
- 初始 MCP Server 启动也在 Tool 投影构建前获取 Catalog Revision/指纹，构建后核对版本，阻止初始视图与版本声明不一致。保持既有 Listen 地址/Tunnel 进程/ToolID/Profile Bindings 不变。
- 集成测试 \`mcp_catalog_hot_refresh_test.go\` 先建立真实 HTTP MCP Client Session，再经独立 SQLite Catalog Writer 修改已绑定 Tool 的输入、输出 Schema、描述、标题、标签，使用**同一个 MCP Session** 验证 Dynamic/Hybrid 的 \`mender_describe_tool\` 和 Direct/Hybrid 的 \`tools/list\` 读取新信息，不修改 Profile Revision 或重启服务。
- 单元测试覆盖无关/重复 Catalog 写入不重复构建、32 路请求只发布一个版本、Catalog Revision/指纹失败时拒绝旧结果、加载及重建期间 Catalog 并发写入不得发布撕裂版本、Catalog Revision 回退拒绝。

### 通过的校验

- \`go test ./backend/...\`：全部通过。
- \`(cd apps/desktop && go test ./...)\`：通过。
- \`go test -race ./backend/internal/bootstrap ./backend/internal/agent/infrastructure/delegationprovider -run 'TestManagedProfile|TestManagedMCPHTTPRefreshesCatalogMetadataWithoutChangingProfile|TestAgentDelegationThroughMCPClientSession|TestUnsupportedNativeSessionContinuationReturnsSafeToolFailure' -count=1\`：通过。
- \`pnpm go:lint\`：0 issues。
- \`pnpm exec vitest run --maxWorkers=4 --reporter=dot\`：53 个测试文件，325 项通过。
- \`pnpm --filter @mender/desktop typecheck\`、\`pnpm sdk:check\`、\`pnpm desktop:bindings:check\` 均通过。
- 本轮仅更改并提交仓库源码和测试；未对用户现有生产 MCP 实例进行重启、修改 Profile、Agent 任务、Policy、Credential 或数据库迁移。

### 部署后的限定验收与不变量

1. 用户下次构建并重启 Desktop 后，管理端对已存在的 Agent 点击刷新工具，在 **不手动重启 MCP Server** 的情况下，重新通过 ChatGPT \`mender_describe_tool\` 核对 \`timeoutMinutes\` 等新 Schema（可以先在隔离来源上改一项无副作用的工具元数据）。
2. \`agent.resume\` 已存在历史工具绑定的场景，确认安全的 \`isError:true\` 分类与固定消息可见。ChatGPT 产品 UI 是否完整展示消息仍可能受宿主封装影响，不能仅凭 MCP SDK 测试断言 UI 必然按原样显示。
3. 暂未实现 MCP \`notifications/tools/list_changed\` 主动推送：**服务器端下次请求已是新 Schema**，但不主动促使那些永久缓存工具列表且不再次请求的外部客户端刷新。后续在已验证 SDK 与客户端订阅能力后再评估通知，不新增事件总线。
4. Provider 显示名称等**不是 Catalog Tool 元数据**的字段变化，不在本次新增的触发范围；原有 Profile Revision / ToolSource 实时可用性门禁仍保持原设计。不得将该修复解释为所有外部 Provider 配置变更自动重载。
5. 未改变 Agy 原生 Session 恢复不支持的事实，也未替换既有 \`unknown\` 生命周期逻辑。禁止自动重放旧 Prompt。

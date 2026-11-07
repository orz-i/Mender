# 阶段 A：Tool / Agent / Workspace / Sandbox 归属与引用盘点

> **状态**：静态代码审计完成；仅记录 `main@ce924e0` 的代码结构（在治理分支实施文档）。**未扫描、复制、迁移或修改真实用户 SQLite 数据。**
>
> 目标契约：[ADR-0064](../architecture/adr/0064-tool-provenance-and-execution-placement-hardcut.md) · [迁移预检合同](tool-hardcut-phase-a-migration-contract-2026-11-07.md) · [总实施规划](tool-workspace-sandbox-agent-hardcut-2026-11-07.md)

## 1. 源 → Tool → 供应 → 执行关联（现状）

`Configured Provider (external) → Catalog ProviderID → Catalog ToolID → Profile ToolBinding → Invocation → Provider activation/executor`

`Agent Resource (AgentID, ACP Backend) → agent_tool_facades → internal ProviderID → Agent's five ToolIDs → Profile ToolBinding + AgentGrant + AgentWorkspaceGrant → Invocation → Agent Task/Session`

`Sandbox Resource (SandboxID, Backend) → sandbox_tool_suite_bindings → internal ProviderID → four workspace ToolIDs → Profile ToolBinding → Provider-keyed workspace target alias → sandbox.acquire + Profile capability grant + lease → typed Workspace read/search/edit/shell`

`SSH Sandbox Resource → sandbox_job_suite_bindings → internal ProviderID → five jobs ToolIDs → Profile ToolBinding + Sandbox Job grants/admission → remote SSH Job backend and evidence-based recovery`

**问题点**：Catalog 内部仅识别 ProviderID，使 Agent/Sandbox 作为 Tool Source 时必须先投射一个内部 Provider。业务归属虽然在 Agent/Sandbox，但搜索/供应/API/删除依赖另一份身份。Workspace 逻辑项目也没有独立统一身份。

### 1.1 现有所有权与未来归属矩阵

| 对象/能力 | 当前权威 | 现状观察 | 目标归属 | 必须保留 |
|---|---|---|---|---|
| Tool 契约、ToolID、SourceKey、FTS | Catalog | `Tool.ProviderID()` 必填 | Catalog，`SourceRef(provider/agent/native)` | 独立 Provider 工具身份、Schema、索引、搜索 |
| MCP/OpenAPI/Model Tools 外部实例 | Catalog Provider + ProviderPlugin | Provider config 和 PluginID/安装 pin | 独立 Provider | 连接配置、Credential refs、Provider Activation |
| Agent as Tool | Agent/Task/Session | AgentID 外加 Agent Provider facade | AgentID 是来源，Agent 模块持有生命周期 | 5 种操作、TaskID/Session、ACP v1、approval/resume |
| Sandbox Workspace 功能 | Workspace + Sandbox | 每个 Sandbox 注册一份 Workspace Tools facade | native workspace Tool + WorkspaceRef + Sandbox placement | typed operation、Policy facts、Profile grant、lease |
| SSH Jobs 能力 | Sandbox/SSH plugin | 独立 Jobs facade；本机持有 obligation/recovery evidence | native jobs Tool + Sandbox 所有任务边界 | 不确定作业防重复、插件持有 JobID/日志、观测和取消 |
| Agent workspace path | Agent | 默认 WorkspaceRoot + agent_workspaces alias + agent grants | 公共 WorkspaceRef + Agent 使用授权 | 当前 cwd/ApprovalPolicy 安全验证、快照 |
| Sandbox Root/Run | Sandbox | local_root 或后端专属根 + run generation | Sandbox 实际执行空间/placement binding | 真正可访问性、隔离、状态、owner |
| Workspace Tool alias | Sandbox 数据库表，Provider keyed | `workspace_tool_targets` | 公共 WorkspaceRef + 目标 placement | 严格别名限制/批准关系、后端能力检查 |
| Tool Suite | Curated host registry | 把资源能力产出一组内部 Tools | 可信能力定义，native Catalog 注册一次 | `RequiredContracts`/`ExecutionMode`/隔离限制 |
| Profile / Skill / Delivery | Profile | ToolIDs + ToolBinding + Skills；MCP delivery 单独 revision | Profile 仍持有选择及限制 | 不隐式选择/授权、MCP 交付版本和导出 |
| Policy / Approval | Policy / Agent | ProviderID/ToolID/Fact/Action 选择 | Policy 匹配 typed SourceRef 与已审目标事实 | most restrictive、deny/approval、无原始 secret |
| Audit | Audit / owning service | 旧 ToolID/ProviderID/资源/Task evidence | 原始历史不改写，添加仅审计用映射 | 可追溯性、不会把历史调用当新授权 |

## 2. 代码定位与迁移范围

### 2.1 Catalog / Provider

- `backend/internal/catalog/domain/tool.go`：`Tool{providerID, sourceKey, id}`，`NewTool/RestoreTool` 和访问器。
- `backend/internal/catalog/application/sync_provider_tools.go`：按 ProviderID 替换当前 Tool Snapshot，`(ProviderID, SourceKey)` 保持复用 ToolID；外部 Provider 同步语义应继续。
- `backend/internal/catalog/infrastructure/sqlite/migrations.go`：Catalog schema v2；`providers`、`tools(id,provider_id,source_key,...)`、`UNIQUE(provider_id,source_key)`、`tools_fts`、revision triggers。
- `backend/internal/catalog/infrastructure/sqlite/{tool_repository,browse,paged_search}.go`：按 Provider 发现、同步、检索、分页；新的 SourceRef/FTS 约束影响这里。
- `backend/internal/providerplugin/configstore/migrations.go`：Provider Config schema v4，`provider_configs` 中 PluginID/Config/Installation/Credential refs，须仅保留真实独立 Provider。
- `backend/internal/providerplugin/manager.go`：ProviderID → Plugin/Executor routing；外部 Provider 需继续，内部 Agent/Sandbox 路由退役后不能误杀它。

### 2.2 Agent as Tool

- `backend/internal/agent/infrastructure/sqlite/migrations.go`：Agent schema v19；`agents`、`agent_tasks`、`agent_grants`、`agent_tool_facades(agent_id PK,provider_id UNIQUE,suite_id)`、`agent_workspaces`、`agent_workspace_grants`。
- `backend/internal/agent/infrastructure/sqlite/{tool_facades,workspaces,tool_placements}.go`：Facade Claim/Release、Workspace alias 及 Profile Grant、ToolPlacement 管理。
- `backend/internal/agent/infrastructure/sqlite/{tool_placement_migration,approval_policy_migration,execution_migration,native_resume_migration}.go`：`agent_tool_placements` 保存 WorkspaceRoot/SandboxID/ToolIDs JSON；`agent_approval_policies` 按 WorkspaceKey/Root；`agent_native_sessions`、attempts/events、continuations 独立 Task 证据。
- `backend/internal/bootstrap/agent_tools.go`：`EnsureAgentTools/CreateAgentWithTools/DeleteAgentWithTools` 通过内部 Provider 注册/修复 Agent tools；有“已经创建 Agent 但 tools 不 ready”的半配置管理路径。
- `backend/internal/agent/application/{tasks,resume,approvals,approval_policy}.go`：Task 准入、准确 WorkspaceRoot/AgentGrant、native session 和审批目标。**不能用 Sandbox Router 代替 Task/Session。**
- `backend/internal/agent/domain/tool_placement.go`：既有 opt-in Agent 工具供应绑定 Sandbox/ToolIDs，迁移时要保持严格权限边界。

### 2.3 Sandbox / Workspace / Jobs

- `backend/internal/sandbox/infrastructure/sqlite/migrations.go`：Sandbox schema v19；`sandboxes`、`sandbox_runs`、`sandbox_profile_grants`、`sandbox_execution_leases`，Workspace Suite v12、SSH Job Suite v14、Workspace targets v19。
- `backend/internal/sandbox/infrastructure/sqlite/{tool_suites,job_tool_suites,workspace_targets}.go`：`sandbox_tool_suite_bindings`、`sandbox_job_suite_bindings`、`workspace_tool_targets(provider_id,workspace_key,sandbox_id)`，以及关联的寿命保护。
- `backend/internal/sandbox/infrastructure/router/router.go`：已经存在只读 `Resolve` 和受 `sandbox.acquire` 管辖的 `Acquire`，检查 backend binding、run generation、ready state、capability、durable lease，无执行环境的 Host fallback。
- `backend/internal/workspace/{domain,application,infrastructure}`：ADR-0061 的 typed request/lease 与 Sandbox bridge；`infrastructure/plugin/targets.go` 从 ProviderID/alias 解析目标，依据 `sandbox_id` Governance Fact 防止预检与实际目标不一致。
- `backend/internal/bootstrap/{sandbox_tool_suites,sandbox_job_tool_suites,workspace_targets}.go`：内部 facade 的准备/管理 API，切换后应删除其业务路径。
- `backend/internal/toolsuite/infrastructure/curated/registry.go`：`agent.delegation` 5 操作、`sandbox.workspace` 4 操作、`sandbox.jobs` 5 操作；`sandbox.browser` 尚 `unavailable`，不允许伪可用。

**SSH Job 特别说明**：`sandbox_ssh_job_obligations`、`sandbox_ssh_job_recovery_events`、`sandbox_ssh_job_admission_controls` 是 Mender 的归属/恢复证据；远端 JobID、日志、结果由 SSH 后端/插件持有，不能假定本地 SQLite 保存了完整 Job Task 数据。迁移不得伪造“已完成”或自动重投。

### 2.4 Profile / Policy / MCP / Audit

- `backend/internal/profile/infrastructure/sqlite/migrations.go`：Profile schema v7，`profile_tools(profile_id,tool_id,alias,title_override,description_override,policy_constraint,concurrency_constraint,credential_refs_json,...)`。
- `backend/internal/profile/infrastructure/sqlite/migration_delivery_v7.go`：`mcp_delivery_configs` 与 `mcp_delivery_direct_tools(profile_id,tool_id)`；Hybrid 直接投影是独立约束/修订。
- `backend/internal/policy/infrastructure/sqlite/{migrations,store}.go`：Policy schema v3，`policy_rules.match_json` 包含 `providerIds`、`toolIds`、`resourcesAny`、`actions` 等；不能仅通过字符串替换做语义转换。
- `backend/internal/bootstrap/provider_reference_guards.go`：跨 Context SQLite triggers：Provider Config 归属、Profile Tool 引用、Policy Tool/Provider 引用、Provider retirement；其 SQL 明确 JOIN `tools.provider_id`，硬切必须整体重建。
- `backend/internal/management/infrastructure/querysqlite/tool_source_owners.go`：只读 JOIN `agent_tool_facades` / `sandbox_*_suite_bindings`，新投影不再通过此 JOIN 找归属。
- `backend/internal/mcp/infrastructure/server/{server,meta}.go`：MCP Source 的 `sourceId=ProviderID`、Source-first grouping、Cursor fingerprint、Direct/Hybrid 导出名 `provider.Namespace + tool name`；新 SourceRef 的版本与名称迁移必须联动。
- `backend/internal/bootstrap/runtime.go`：目前 Catalog/Profile/ProviderConfig/Sandbox/Agent Store 分别启动和迁移；跨 Context **原子**的硬切迁移必须安排在正常业务 Store/Listener/Runner 进入读写之前（须调整启动协调），不能直接复用各 Context 的多个独立迁移事务。

### 2.5 API / SDK / Wails / CLI / UI

- `contracts/api/openapi.yaml`：包含 `/providers`、`/tool-services/{providerId}/workspaces`、`/agents/{agentId}/tools`、`/sandboxes/{sandboxId}/tools`、`/tool-suites` 等路由。**必须枚举和更新所有消费者，不能只删除 Go 处理器。**
- `backend/internal/management/httpapi`、`backend/internal/desktop/binding`、`backend/internal/cli`：跨 HTTP/Owner/Wails/CLI 的相同业务事务边界。
- `packages/sdk`：生成的客户端/API 类型；`packages/app/src/domains/tools/services/tool-services-page.tsx` 将 Provider/Agent/Sandbox Facade 混排。
- `packages/app/src/domains/tools/tool-source-label.ts`、`packages/app/src/domains/profiles/{profile-source-label,profile-tool-source-picker}.tsx`：目前以 ProviderID 作为来源导航主键。
- `packages/app/src/domains/{agents,sandboxes,profiles,plugins}`、`packages/app/src/shell/management-shell.tsx`：资源归属和供应视图调整；不需要增加第二套全局导航或新 ToolService CRUD。

## 3. 数据关系审计矩阵

| 历史表/引用 | 键/外键 | 读取或写入场景 | 迁移处置 |
|---|---|---|---|
| `providers / provider_configs` | `provider_id` | 真实外部服务、内部 facade | 外部保留；严格识别内部 facade 后清退 |
| `tools / tools_fts` | `tool.id`、`provider_id`、`source_key` | Catalog/Search/Invocation | 建立 typed `source_kind/source_id`、唯一性、FTS 修订；native 新 ToolIDs |
| `agent_tool_facades` | `agent_id` ↔ `provider_id` | Agent 工具注册/删除/引用保护 | 由精确 AgentID Tool SourceRef 取代；严格唯一归属 |
| `sandbox_tool_suite_bindings` | `sandbox_id,suite_id,provider_id` | Workspace Tools 提供方 | native workspace Tool，Sandbox 作为能力执行目标 |
| `sandbox_job_suite_bindings` | `sandbox_id,provider_id` | SSH Jobs Tool 提供方 | native jobs Tool；原 SSH 数据与恢复权威不变 |
| `workspace_tool_targets` | `provider_id,workspace_key,sandbox_id` | Workspace alias 选择 | WorkspaceRef/PlacementBinding；逐关系验证 |
| `agent_workspaces` | `agent_id,workspace_key,workspace_root`（当前升级后的形态） | Agent 任务 cwd/范围 | 复用 WorkspaceRef，但保留 per-Agent grants |
| `agent_workspace_grants` | `agent_id,workspace_key,profile_id` | 受限委派 | 精确迁移，不因共享 Workspace 广播授权 |
| `agent_tool_placements` | `agent_id,workspace_key,sandbox_id,tool_ids_json` | Governed Agent tools | 映射 native ToolIDs、核验同 root/同 Sandbox |
| `profile_tools` | `profile_id,tool_id` | Profile 供应+展示/约束 | native 多对一必须新增 restrictive target scope |
| `mcp_delivery_direct_tools` | `profile_id,tool_id` | hybrid export | 检验名称变化/同名/保留 selected 关系 |
| `policy_rules` | `match_json` | ToolID/ProviderID/Action/资源匹配 | 语义迁移；Provider activation 不转为 Tool 政策 |
| `sandbox_profile_grants` | `sandbox_id,profile_id,capability` | Sandbox admission | 保留，**不能替代旧 ToolBinding target 限制** |
| `sandbox_execution_leases` / `sandbox_runs` | `sandbox_id,generation` | 生命周期/并发 | 保留 CAS、运行代际和租约 |
| `agent_tasks / native_sessions / approval / continuation` | `agent_id/task_id/generation` | Session/Task 恢复 | 不自动继续/重播，保留所有证据 |
| `sandbox_ssh_job_*` | `sandbox_id` 及归属状态 | Jobs recovery | 生命周期 fencing 不动；阻断不完整迁移 |
| Audit | 原始 ToolID/资源ID/Principal | 历史查询与解释 | 只读旧→新对照，不让旧 ID 可执行 |

### 3.1 跨 Context 写入守卫须重建清单

现有 `bootstrap/provider_reference_guards.go`、`bootstrap/sandbox_provider_binding_guards.go`、Agent/Sandbox SQLite migration 中的 triggers 防止删除/重绑，依据 Provider facade 归属而设计。硬切必须替换为对应 **Tool SourceRef / Workspace placement / Agent ownership** 的 commit-point guard，并验收“引用获取 vs 资源退休”的并发竞态。不能仅移除 trigger，否则会损失此前已经治理过的原子引用安全。

## 4. 高风险授权组合（阶段 B–E 必须测试）

1. **单目标**：Profile P 仅绑定旧 Sandbox A 的 `workspace.read`，同时通过别的来源已有 Sandbox B 的 `file.read` grant。合并为 native Tool 后，P **不能**通过 B。
2. **双目标差异**：P 同时绑定 A/B 的 `workspace.edit`，但 A 是 `require_approval`、B 是 `deny` 或不同 Credential/Concurrency/alias；无法用一个新 ToolBinding 无损表示时 **阻断**，不能取并集或静默覆盖。
3. **政策匹配**：`providerIds=[oldFacadeA]` 且 `actions=[tool.invoke]` 的 deny/approval 必须随 Tool/Target 映射；`provider.activate` 不能被机械翻译成 `tool.invoke`。
4. **Agent 同名**：同 Backend 两 Agent，不能因相同 Tool SourceKey 合并为同一 Agent 工具；其 AgentGrant 和 Task 权限必须保持。
5. **多工作区同路径**：两个 Agent 各有同根目录的 alias 不意味它们具有相同 Profile 授权或审批策略；共享 Workspace identity 不自动并权。
6. **未知任务**：存在 running/unknown Agent Task 或 SSH Jobs recovery obligation 时，不能凭迁移升级或 ToolID 改写推定其已失败可重试。
7. **目标突变**：预检授权 Sandbox A，Acquire 时 Placement Alias/Generation/Grant 已变；执行必须拒绝。
8. **导出名字**：同 Profile 既有 per-Sandbox tool A/B 导出不同名字；Native Tool 多对一后命名和 Hybrid subset 无损转换不了时阻断，等待人工审阅。

## 5. 静态盘点完成与尚缺证据

**已完成**：代码、SQL schema/索引/已知关键 trigger、主要 HTTP API、Profile/Policy、MCP Tool Source、Agent/Sandbox/Workspace ownership 的静态引用梳理。没有修改上述任何代码/数据。

**尚需阶段 E 的只读 dry-run 补齐**：真实 DB 的每个表行数、ToolID 和来源关联、Profile 的具体 ToolBindings/Target/Policy 冲突清单、Session/Job 未终态数量、备份与回滚演练。真实数据库的归属不得从静态代码猜测，所有“无冲突/可升级”声明都必须等预检证明。

另见 [迁移预检合同](tool-hardcut-phase-a-migration-contract-2026-11-07.md) 中的阻断分类和报告格式。
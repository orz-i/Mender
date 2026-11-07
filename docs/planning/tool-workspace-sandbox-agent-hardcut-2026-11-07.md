# Mender 工具 / Agent as Tool / Workspace / Sandbox 归属与执行边界硬切治理

**状态**：阶段 A–E 已提交；阶段 F 的 F1、F2a 与 F2b 必需的现有 Invocation→Sandbox Router 原子租约衔接/路径事实已实现，**实际 Native Executor/SourceRef Policy-Audit 生产装配、F3 公共 MCP/HTTP/CLI/SDK/Wails 合同和 F4 唯一 Runtime 硬切仍未完成**。未增加新治理阶段、无双执行系统；真实数据库未迁移、新执行链未启用。Profile 导出审核、远端 SSH/Jobs 证据、Bootstrap 全量 Race 仍是阻断。

**仓库基线**：`main` / `ce924e0`，Mac mini `/Users/mini/Coding/Mender`，2026-11-07

**原则**：文档先行、分段提交、单次前向迁移、最终不保留旧执行路径/兼容桥；无独立授权不自动扩大权限，不接管 Agent Loop，不直接修改真实用户数据。

## 1. 目标、不做什么

目标定位不变：Mender 是面向外部 LLM / Agent 的工具与资源聚合、治理和供应平台。

- **Tool**：定义一次可调用能力、Schema、Effect、可发现/可供应身份。
- **Agent as Tool**：一种特殊可调用能力，背后是一个固定的 AgentID / Agent Backend。Mender 管理委派、Task/Session、审批与结果观测，Agent 自行运行推理循环。
- **Workspace**：实际要作用的项目/目录/资源范围的稳定逻辑上下文。
- **Sandbox**：执行位置、真实能力、隔离、生命周期、租约与授权；本身不是工具来源。
- **Provider**：独立外部工具来源（MCP、OpenAPI、Model Tools 等），而不是所有工具的强制父实体。
- **Profile**：对外供应哪些 Tool/Skill、展示方式和限制条件；不拥有 Agent/Sandbox/Workspace 生命周期。
- **Tool Suite**：可信宿主审核的能力→工具操作定义；不是每个 Sandbox 必须创建的独立 Tool Service。

**不是**建立新的 Agent 编排平台、通用任务调度器、所有资源的统一 CRUD 表或每个 Sandbox 的 Provider 克隆。不强制把外部 MCP API 调用放到 Sandbox；只对真正需要执行环境的 Mender 托管工具进行目标匹配。

## 2. 已核对的代码事实和债务

| 证据 | 当前问题 | 硬切方向 |
|---|---|---|
| `catalog/domain/tool.go`、`catalog/infrastructure/sqlite/migrations.go` | 所有 Tool 被 `provider_id NOT NULL` 外键绑定；FTS 按 Provider 存储 | Tool SourceRef 改为类型化值对象，外部 Provider 仅是一种来源 |
| `bootstrap/agent_tools.go`、`agent_tool_facades` | 每个 Agent 先注册一个 Agent Delegation Provider，才有 delegate/status/result/cancel/resume | Tool 直接归具体 AgentID；Facade 不再拥有第二套资源生命周期 |
| `sandbox_tool_suite_bindings`、`sandbox_job_suite_bindings` | 每个 Sandbox 通过 Provider 发布 workspace/jobs | 只注册一次可信内建 Workspace/Jobs Tool 操作，Sandbox 是执行目标 |
| `sandbox/infrastructure/sqlite/workspace_targets.go` | Workspace alias 依附于 ProviderID，指向 SandboxID | WorkspaceRef/Placement 不归属 Provider |
| `agent_workspaces`、`agent_workspace_grants`、`Sandbox.LocalRoot` | 同一项目可能以 Agent alias、Provider alias、Sandbox root 重复描述 | 工作区身份复用；执行根及独立授权仍由相应运行边界保存 |
| `sandbox/infrastructure/router/router.go` | 已有无副作用 Resolve + 守卫后 Acquire、run generation、durable lease | 直接复用，不建设第二套 Sandbox Router |
| `workspace/{domain,application,infrastructure}`、ADR-0061 | 已有类型化 read/search/edit/shell 与 Sandbox bridge | 保持分层，扩展目标解析不恢复 JSON/执行混用 |
| `mcp/infrastructure/server/server.go`、ADR-0056 | sourceId=ProviderID、Tool 供应和 MCP 导出名称依赖 Provider | 迁移为 typed SourceRef，显式 Supersede ADR-0056 中该决策 |
| `profile_tools`、Profile Delivery、Policy | 绑定精确 ToolID，可能存在 alias/policy/concurrency/credentials/direct 差异 | 任何多对一 ToolID 合并前必须报告并解决限制冲突 |
| `tools/services/tool-services-page.tsx`、`profile-tool-source-picker.tsx` | UI 仍将 Agent/Sandbox 所属工具包装成 Tool Service | 管理按真实资源归属，Catalog/选择器按能力来源显示 |

## 3. 目标领域结构与工具身份

### 3.1 唯一来源，不制造新聚合

定义只读、可验证的 `SourceRef{kind,id}`（`provider` / `agent` / `native`），而不是新的 Source 数据库表和管理 CRUD：

- `provider:<ProviderID>`：独立配置的上游工具来源；同步 Tools、管理凭据/连接/激活，保留原 ProviderID 与 SourceKey。
- `agent:<AgentID>`：每个 Agent as Tool 实例的工具入口。两个 Agent 即使 Backend 相同也要保持不同可调用身份。
- `native:workspace`：一个受信 Workspace Tool 源，包含 read/search/edit/shell，不为每个 Sandbox 另建 ToolID。
- `native:jobs`：一个受信 Durable Job Tool 源，包含 submit/status/logs/result/cancel，明确要求可验证 Sandbox Job Backend。
- Browser 只在真实 `browser.session` 后端具备所需能力且通过验收时发布；不把尚不可用的定义标成可调用工具。

不引入独立 `ToolDefinition/ToolInstance/ToolSource/ToolService` 四层持久化对象。Canonical Tool 保持统一 ToolID；其 SourceRef 是可验证的来源身份；Execution Placement 是一次请求的执行决策，不是 Tool 的永久所有权。

### 3.2 Agent as Tool 特殊规则

- AgentID 与 Backend、默认模型、Task、Session、native ACP approval 各自已有明确所有权，保留现有 Agent bounded context。
- 继续对外供应 `delegate/status/result/cancel/resume` 操作（异步 Task 控制不能随意删掉）；UI 将其合称一个“可委派 Agent”而不是五个相同名字的 Agent。
- 精确唯一绑定的旧 Agent ToolID 优先原样保留；Tool 的 owner 从旧 Provider facade 改成 AgentID。每个操作仍进入 Invocation，委派进入 Agent Task admission，查询/取消按 TaskID + Caller 重验。
- Workspace 只负责 Agent Task 的合法 cwd/上下文；选择 Workspace 或 Sandbox **不等于** ACP 进程一定在 Sandbox 内。ACP v1 仍是已实现基线。只有后端实际支持进程 I/O、隔离、生命周期的受控 Sandbox 才能用于 Agent 进程放置。
- Agent 内部显式配置的 Mender Workspace Tools 仍按单次 Tool Invoke 的 Policy、Profile、Workspace/Sandbox grants、generation 和 lease 检查；不能让 Agent 自行因为“已授权委派”获得所有工具。
- 不自动重放 unknown Task、不替换 Agent Loop、不把 Agent 作为 Shell Tool 的别名。

## 4. Workspace 最小共享身份与执行环境匹配

### 4.1 最小 WorkspaceRef

- 一份共享的逻辑 WorkspaceRef（可对应具体 Project/Directory），由宿主明确登记/验证 owner、路径或远端逻辑根；多个 Agent/Tool 可引用。
- Sandbox 的 `LocalRoot`、Docker mount、SSH remote root 等仍是**实际**文件系统边界；新的 Workspace 与 Sandbox 之间使用经验证的 PlacementBinding 描述“这个环境可访问该 Workspace 的哪个实际根”，不根据同名目录猜测。
- 不自动复制文件、自动创建目录、自动挂载云端；多个 Sandbox 的数据未证明等价时，不视为可无损互换的副本。
- AgentWorkspaceKey、历史 Provider Target Alias 作为迁移输入/可选显示名，最终不作为 Tool Source/授权主体的全局身份。
- Workspace 可以先嵌入 Agents、Tools、Sandboxes、Profiles 的复用选择界面，**不强制新增一级导航**。

### 4.2 Workspace 选择优先级

1. 可信调用上下文中的已登记 WorkspaceRef（不是模型传入的任意路径/裸 SandboxID）；
2. owner/当前 Profile 显式指定的默认 WorkspaceRef；
3. 当且仅当一个 Workspace 符合现有 grants 时自动选择；
4. 零个或多个无默认候选：`workspace_required` / `ambiguous_workspace`，返回最小授权可见候选；不得猜测项目名称、最近使用项或默认 Host。

### 4.3 Sandbox 匹配

顺序：已登记 Workspace→可验证 Placements→Tool 的受信 `RequiredCapabilities` 和版本→隔离要求→Principal/ToolBinding/Workspace/Agent/Sandbox Grants→Policy/Approval→ready/generation/budget→唯一或 owner 固定的 Sandbox 目标。

- 无匹配 = 不可用；多个不等价匹配 = 歧义，需明确选择或已有固定偏好。
- 固定 Sandbox 不可用时失败；不自动降级到 Host、另一个 SSH 服务器、弱隔离或未授权目录。
- 对外部 MCP/OpenAPI Provider Tool，正常走 Provider Executor/Activation；它们通常不需要 Mender Sandbox。
- 对 Agent Delegate，首先解析具体 AgentID/Task Runner；工作区和其可用的托管工具是后续独立授权，不套用 file.read 的 Sandbox 路由。
- 对 Jobs，目标 Sandbox 与任务所有权须从不可伪造的 durable job record 解析，不能由模型填任意 JobID/SandboxID 取得他人日志。

### 4.4 执行时序与防 TOCTOU

`Resolve Tool + Scope → Validate Inputs → Resolve Workspace/Target → Freeze Governance Facts (tool/effect/workspace/sandbox) → Policy + Approval + Profile tightening → Runtime Budget → Revalidate target/ACL/generation/version → Sandbox Acquire (if required) → Typed operation / Agent Task / Provider Call → Release → ResultGuard → Audit`。

要求：
- 预检只读：不启动进程、Provider、凭据读取/云资源分配。
- 执行前重新核验目标 Workspace/Sandbox、backend pin、run generation、授权和相关修订；不可以 Policy 批 A、实际执行 B。
- 对发生副作用且结果未知的操作，绝不因为超时自动选择其他目标重试。
- 第三方 MCP `readOnlyHint`/`destructiveHint` 等 annotations 只是提示，不能替代 Mender 受信的 Capability Contract/Policy。

## 5. 权限边界（求交集、不自动授权）

| 检查 | 控制的含义 |
|---|---|
| Profile ToolBinding | 当前消费方能否调用哪个 Tool；presentation/policy/concurrency/credential 限制 |
| Profile / Workspace Grant | 此消费方可作用于哪些 Workspace，是否可 read/edit/shell |
| AgentGrant + AgentWorkspaceGrant | 此 Profile 可否委派此 Agent 使用此 Workspace |
| SandboxProfileGrant / sandbox.acquire | 此调用方能否在此 Sandbox 上执行特定 capability |
| Policy / Approval | 全局 deny/require approval、特定 effect/资源规则 |
| Runtime Budget / Lease | 并发、超时、结果预算、执行租约、隔离、run generation |

- 选择工具 ≠ 选择工作区 ≠ 创建 Sandbox ≠ Grant。
- 新建 Workspace/Placement 不得产生 Profile grants；Profile membership 也不反向创建 Target grants。
- 若旧多份 Sandbox workspace ToolBinding 有不同 alias、policy、concurrency、credentials 或 Direct 配置，迁移不得直接求并集：要么有**等价且不扩权**的新目标约束，要么 dry-run 阻断并要求显式治理。

## 6. 硬切数据迁移

### 6.1 映射对象与策略

| 旧对象/关系 | 新归属 | 默认处理 |
|---|---|---|
| 外部 Provider/Tool/Config/Credential | `SourceRef(provider, ProviderID)` | 保留身份及配置 |
| AgentToolFacade Provider + 五个 Tool | `SourceRef(agent, AgentID)` | 一对一可验证时保持 AgentID/ToolID；非法/重复 facade 阻断 |
| Sandbox Workspace Suite Provider + Tools | 单份 `native:workspace` 工具；执行目标由 Workspace/Placement 管理 | N→1 ToolID 会变化，产出旧→新完整清单；验证每个 Profile 授权与 Presentation 冲突 |
| Sandbox SSH Jobs Suite Provider + Tools | 单份 `native:jobs` 工具；Job target/运行数据属于 Sandbox | JobID、状态、恢复意图不变；无原生恢复权限则阻断 |
| `workspace_tool_targets` | WorkspaceRef + PlacementBinding | 只迁移明确的 alias→精确 SandboxID；不猜测别名等价 |
| `agent_workspaces`、默认 WorkspaceRoot | Agent 已绑定 WorkspaceRef / cwd projection | 保留独立 AgentWorkspaceGrant 和 root 验证；不擅自扩大工作目录 |
| `agent_tool_placements` | AgentID + WorkspaceRef + SandboxID + new ToolIDs | 明确映射已允许工具；不能拓宽 Tool 授权 |
| `profile_tools` + MCP Direct/Hybrid Tool IDs | 新 ToolIDs/交付映射 | 保留精确配置、policy、credential、alias、direct；不兼容多对一阻断 |
| Policy 中 ProviderID/ToolID 条件 | 对应的 Tool SourceRef/ToolID + Target 约束 | 不能无损转换时阻断，尤其 deny/approval 不可被清除 |
| Audit / Task / Session / Approvals / SSH Job | 原始不可变证据 + 仅用于审计显示的旧新 ID 对照 | 历史记录不删除；旧 ToolID 不允许运行时 fallback |

**关键事实**：一个旧 ToolID 不可能同时无变化地代表多个独立来源，而 N 份已绑定 Sandbox ToolID 合并成一个 canonical ToolID 时，并非所有 ID 都能保留。硬切允许明确的 breaking change，但必须带可审计 mapping/导出变更清单，不能悄悄破坏授权或假称“所有 ID 原样保留”。

### 6.2 预检与离线迁移

1. 从代码和真实 DB 的**只读**快照输出完整 inventory：所有旧/新来源、ToolID、Profile 选择和限制、Policy 规则、Credential allowlist、MCP 交付名、Agent/Workspace/Sandbox grants、Task/SSH Job/Lease/unknown 状态。
2. dry-run 生成 deterministic migration manifest，逐项标记 `keep/remap/merge/block`，阻断重复归属、孤儿 ID、不可表达的 Policy、Profile restriction 冲突、未证实的跨环境 Workspace 等价。
3. 实际切换需明确停止老 Runtime 的新写入和 Agent/MCP/Tunnel 启动，排空/隔离未决 Task、审批、租约和 SSH Job；绝不自动重放 unknown 操作。
4. 使用 SQLite Backup API 创建一致性备份，校验可恢复；不能对正在 WAL 写入的 DB 简单 `cp`。
5. 在所有 Context Store 初始化前，由**基础设施层单个跨 Context 迁移协调器**持有一个事务，重建 Catalog tools、SourceRef 字段、Workspace mappings、Profile/Policy 绑定、FTS/索引/触发器及 schema version，清理旧 facade 表和内部 Provider 数据。不能以多个 Context `migrate()` 小事务冒充一次原子升级。
6. 事务结束前核对 `foreign_key_check`、`integrity_check`、映射总量、Profile 授权无扩张、Job 和 Task 证据可读；失败整笔回滚，旧 DB 不变。
7. 新 Runtime 只接受新 Schema；旧 Runtime 对未来版本显式拒绝。需要回退时，停机恢复**升级前整库备份**并启动旧程序；不提供运行时双读、双写或兼容 URL。

历史 Audit ID 对照可作为只读展示信息保存，**不能**恢复旧 ToolID 的 Invoke 路径。旧版数据库 fixture/迁移测试应保留，旧运行时代码必须删除。

### 6.3 P0 blockers（出现任意一项禁止切换）

- 多个旧 ToolBindings 合并导致授予范围、审批、deny、并发或 credential 不等价，且无法安全转换。
- Policy 的 ProviderID/ToolID 规则不能保留相同或更严格语义。
- Workspace 的真实路径、属主、后端 root、run generation、Agent root 不一致。
- 未完成/未知 Agent Task 或 SSH Job 的新路径无法继续按权限观察/取消/恢复，或 migration 会误提交/重放。
- schema、FK、FTS、trigger 不一致；未来版本、并行写入、备份失败。
- 新执行路径需要 fallback 到旧 Provider facade 或跨隔离级别。
- MCP 直接暴露的工具名称冲突，或客户端交付差异无法准确列出。

## 7. 单次发行、分阶段提交（最终没有双模式运行时）

### A — ADR/盘点/迁移合同（Docs only，完成）
- [x] [ADR-0064](../architecture/adr/0064-tool-provenance-and-execution-placement-hardcut.md)：明确部分替换 ADR-0056 `sourceId=ProviderID` 与强制 Provider 归属；保留其余现有 Agent Task/Session、Workspace/Sandbox 运行安全边界。历史 Accepted ADR 不被重写，统一在新 ADR 与 index 标注被替换部分。
- [x] [跨表/代码归属盘点](tool-hardcut-phase-a-inventory-2026-11-07.md)：Catalog、Agent/Sandbox facade、Provider config、Workspace Targets、Profile/MCP/Policy、API/SDK/Wails、Task/SSH Jobs/Audit。
- [x] [迁移 dry-run 合同](tool-hardcut-phase-a-migration-contract-2026-11-07.md)：严格 TargetScope 收紧、旧新 ToolID 精确映射、manifest/冲突码、停机备份和一次性升级阻断门禁。
- **Gate**：仅静态审计/文档一致性验证，不代表真实数据库可迁移；不触碰生产数据。下一阶段 B 首先需在实现中固定 native ToolID Registry、TargetScope 类型与无扩权测试。
- **提交**：`docs(architecture): define tool ownership and target hard cut`。

### B — Typed SourceRef 和 Workspace 最小模型（实现完成，验收收口中）
- [x] Catalog Tool 只读 `SourceRef()` 与严格 `SourceRef(provider|agent|native)` 值类型；现存 Tool 保持 Provider-backed，尚不启用 Agent/native 新执行链。
- [x] Workspace `wrk_` 身份、只读 Placement 查询 port；不允许类型化 Workspace 层直接依赖 Sandbox 实现。
- [x] 九个固定 native Workspace/Job ToolID，宿主审核能力与静态登记（不向现有 Catalog/MCP 发布）。
- [x] `TargetScope` 默认拒绝，精确 `(WorkspaceRef,SandboxID)` allow-set 与迁移的子集校验；仍不挂入不支持持久化的当前 Profile Binding。
- [x] [阶段 B 实现与安全边界记录](tool-hardcut-stage-b-2026-11-07.md)；精确身份、无隐式扩大权限、只读查询、Registry 与分层测试。
- **Gate**：定向 Go/Race、后端架构、Go lint、完整普通 Go、前端/SDK/Wails 生成与构建均通过。全量 `pnpm check` 的 Bootstrap Race 10 分钟超时是待收口门禁，复验当时运行的 10 个 Bootstrap 测试在限制并发后通过。不得对外声称完整 canonical gate 已通过。
- **提交**：`refactor(domain): type sources and workspace references`。

### C — 工具执行目标解析（阶段性适配器与测试完成；未切换生产调用链）
- [x] Workspace 应用层：从已授权 Workspaces 中明确选择、使用经批准默认或唯一候选；多个目标一律拒绝模糊推断。
- [x] Workspace application 只读 Sandbox placement 匹配；Pin 目标不可用时绝不降级或改选其他 Sandbox。
- [x] [Guarded Sandbox Bridge](tool-hardcut-stage-c-2026-11-07.md)：基于现有 Router Resolve/Acquire 核对 Native ToolID、Profile TargetScope、Workspace Authority、真实 Backend Capability/Isolation、local root 和 run generation；保存 reviewed facts，在已有 Invocation 批准后核对准入凭据并复验再取得原生持久租约。
- [x] 独立 Agent Task/Session/ACP v1 路径保持不变；不引入新的 Agent 运行时或覆盖原生工具审批。
- [x] 负向测试：Policy 批 A 执行 B、未获 Invocation 准入、撤权/lease CAS 竞态、Backend 消失、目标 root/run 变化、无 Host fallback。
- **不能宣称**：Workspace Inventory、Profile TargetScope、Invocation receipt 的生产适配器及数据持久化（阶段 D–F）；跨 Docker/SSH 的 root 认证、实际生产 MCP 路径切换、真正 Agent Tool Facade 退役均尚未完成。
- **Gate**：定向 Go/Race、架构/lint、前端 512 项测试和双端构建通过；全量有界 Race 在 Bootstrap 长时未结束，已停止并记录非通过状态。生产硬切前必须单独完成全量门禁。
- **提交**：`refactor(runtime): resolve governed execution placement`。

### D — Catalog / Tool Source / Provider 归属模型（实现完成，生产硬切待 E/F）
- [x] Canonical Tool 内部强制 SourceRef，支持 provider/agent/native；为当前生产 v2 保留 Provider-only 构造/读取，不在未迁移的旧库注册 native/Agent 工具。
- [x] Provider Sync 明确使用 `provider:<ProviderID>`，旧 v2 Sync 与插件行为保持一致。
- [x] Invocation Executor Router 基于精确 SourceRef 选择；两个同后端 AgentID 不混淆，native ToolID/SourceKey 必须匹配宿主审定注册表。
- [x] 每个 Workspace/Jobs 操作一份固定 ToolID 和宿主审定的完整 Schema；不按 Sandbox 克隆新工具，不冒充 ACP Runtime。
- [x] 独立的未来 SourceRef SQLite/FTS 仓库与只读来源查询、事务快照校验、Provider SQL owner guards，且未被生产 Catalog 自动迁移调用。
- [x] [Stage D 实现、迁移前置条件及测试证据](tool-hardcut-stage-d-2026-11-07.md)。
- **不可提前实施**：删除旧 Agent/Sandbox Facade、重建真实 DB、启用 native Tools、Profile TargetScope/Policy/MCP/SDK/Wails 切换，必须和 E/F 的安全迁移及交付共同完成。阶段 D 的隔离新结构不是第二条运行中的工具系统。
- **Gate**：Catalog/Provider/Agent Native 元数据及路由回归，SourceRef SQLite FTS 与负向授权验证；全量 Bootstrap Race 门禁待最终治理。
- **提交**：`refactor(catalog): prepare typed tool source and native registry`。

### E — 跨 Context 一次性迁移（离线评估与受限候选升级已实现；真实切换待 F/H）
- [x] [阶段 E 实施与验收记录](tool-hardcut-stage-e-2026-11-07.md)：只读 deterministic `assess`、跨 SQLite Schema/全部表数据指纹、来源/ToolID/Profile/Policy/Workspace/Task/Job 的完整冲突清单。
- [x] SQLite Online Backup API：一致性备份、WAL 检查点、拒绝覆盖、0600 文件权限、备份/源摘要比较。
- [x] `stage-copy`：离线候选库单事务重建新 Catalog/FTS、9 个内建 Native Tools、审计 ID 映射、SQL 来源引用守卫；通过的本地 Workspace/无 Profile Facade 与无活跃任务 Agent as Tool 可安全转换；旧 Runtime 拒绝高版本。
- [x] 负向测试：Profile 合并约束冲突、MCP 来源变更即使 ToolID 未变、Policy 旧 Facade 匹配、未知 Agent Task、SSH 未获恢复证明、根目录消失/远端不确定、未知 Trigger、并行 Context 数据修改后的旧摘要、失败后备份/原库保全。
- [ ] **待 F/H**：需要人工审核的 Profile/MCP Direct/Hybrid 名称与 TargetScope 等价转换、Agent Workspace 授权迁移、SSH Job/远端真实证据、生产维护窗口与最终运行时发布；没有无损证明时本阶段禁止真实升级。
- **Gate**：仅可丢弃数据库离线验收；生产 `pnpm check`/全量 Race 门禁和 Wails 实机仍不得标记完成。
- **提交**：`docs(governance): specify stage E offline migration and release blockers`、`feat(migration): stage guarded one-way tool ownership candidate`。

### F — API/MCP/CLI/SDK/Wails 同步（F1/F2a 及 F2b 执行准入衔接完成；F3/F4 与最终装配待推进）
- [x] [阶段 F 实施合同与证据](tool-hardcut-stage-f-2026-11-07.md)：新 Schema 上 Profile ToolID/SourceRef 来源分组、AgentGrant、Native ToolBinding + Workspace/Sandbox Grants + TargetScope、Direct/Hybrid/Dynamic 供应投影、Source-first 分页、失效 Cursor 和导出名称碰撞拒绝，**不接入旧 MCP**。
- [x] 未来 Native Profile 工具的统一持久租约表中实现原子预留：ToolID + WorkspaceRef + SandboxID + 真实运行代际及独立授权的同 SQL 语句核对；并发撤权/错误批准事实负向测试通过；**没有独立执行引擎**。
- [x] 阶段 E 未来 Schema 补足独立 Workspace Profile Grants、Native 目标插入约束、授权撤销/添加后的 Profile/MCP revision 递增。
- [x] F2b 已复用唯一 Invocation→Sandbox Router 会话/租约链：Policy/Approval/预算与 Binding 成功后才附加内部审批凭据，`AcquireNative` 拒绝旧 Provider 租约 Store，`typedlease` 在同一次 SQL 预留中限制原目标；原有 Workspace 路径规则共用，Policy facts 必含规范化 `workspace_path`，冻结参数防篡改。
- [ ] F2b 最终装配：Native Tool/Agent as Tool 执行器与 Policy/Approval/Audit 的 SourceRef、当前调用参数和 Source-first 运行时一体接线（不能以仅有的适配器测试冒充正式可调用能力）。
- [ ] F3：统一 MCP/HTTP/CLI/SDK/Wails SourceRef、DTO 和 Direct/Hybrid 导出，拆除 ProviderID-only 来源 API，不允许双执行路径。
- [ ] F4：跨 Context 正式运行时一次性切换、旧 Facade 代码退休和生产等价迁移/远端证据、Wails 实机验收。
- **Gate**：F1/F2a 定向 Go/Race/架构和权限负向测试已通过；整个 F 的 HTTP/Wails/CLI parity 和生产门禁仍未完成，不得宣称阶段 F 或硬切已经结束。
- **阶段性提交**：`feat(profile): project exact typed tool supply`、`feat(sandbox): fence native workspace leases atomically`；F3/F4 另行分段提交。

### G — 用户 UI/UX 收口
- Tools：统一目录和**真正的外部工具来源**；Workspace/Jobs 各一份 native 工具；Agent as Tool 作为 Agent source 可发现但不能独立创建 Agent Provider。
- Agents：委派 Agent、Task/Session、Workspace/授权在 Agent 一处；Sandboxes：能力、隔离、关联 Workspace、运行/租约，没有“先 Prepare Tools 再有读写工具”的额外业务操作。
- Profile：来源→工具选择，实际 Workspace/Agent/Sandbox 授权独立且可理解；避免给普通用户露出 ProviderID。
- Settings/Marketplace 不增加虚假类别；Workspace 先用共享 picker/详情，暂不加一级页。
- **Gate**：1440px / 390px、Wails 实机、可达性、i18n、单屏密度、无隐藏危险授权/重复按钮。
- **提交**：`refactor(ux): organize tools, workspaces and execution environments`。

### H — 完整审计、清理、切换验收
- 删除旧 Provider facade 注册/维护逻辑、SQL 运行时表/查询、重复业务 DTO/CLI/Wails routes、dead code、旧 workflow JS；保留历史数据库升级 fixtures 与只读 Audit ID 对照。
- `pnpm check`、`pnpm security:check`、`git diff --check`、Go race / SQL migration crash/restart、SDK/Wails codegen drift 和 architecture checks 全通过。
- 在隔离数据库验收：两同后端 Agent、两个 Workspace、Local/Docker/SSH、Workspace read/edit/shell、SSH durable jobs、跨 Profile 授权/撤权/审批、Agent task status/result/resume/unknown、外部 Provider。
- Owner 控制下生产停机备份→dry-run 0 blockers→一次切换→重建 Wails→真实 UI/客户端验收，最后审计实际运行状态。**未经单独指令不执行真实升级或 push**。
- **提交**：`test(governance): prove hard cut and migration invariants`、`docs(governance): record migration and native acceptance`。

> 分段提交服务于审计和失败定位；中间阶段不得部署一半新旧数据模式。最终构建中，旧 Facade/Provider 运行路径和旧兼容 API 均不得存在。

## 8. 完成定义（全部通过才宣布收口）

- [ ] Catalog 一个真实 ToolID 对应一个可验证来源；AgentID 不再有独立 Provider facade；每个 Sandbox 不再额外产生 workspace/jobs ToolID。
- [ ] Agent as Tool 保持独立 Task/Session 和 ACP v1 边界，无 Agent Loop 托管或 unknown 自动重播。
- [ ] Workspace 是复用的工作上下文，Sandbox 是有能力/隔离/授权的执行环境；目标选择 deterministic、歧义失败、无 Host fallback。
- [ ] Provider 本身仍可按插件版本、凭据和 activation gate 正常发现/调用外部 MCP/OpenAPI/Model Tools。
- [ ] Profile ToolBinding、Policy/Approval、CredentialScope、Workspace/Agent/Sandbox grants 不被静默合并或扩权。
- [ ] MCP Direct/Hybrid、source-first search、SDK、CLI、Wails 不再假设每个 Tool 有 ProviderID；无旧双路径。
- [ ] 审计历史、Task/Session 和 SSH durable job 证据不丢，所有新调用都继续审计。
- [ ] 一次性离线升级及回退已在可丢弃数据库演练；真实库先备份，0 blockers 才切换。
- [ ] 全自动质量门禁及原生 Wails 真实操作通过，工作树无遗留兼容/死代码。

## 9. 参考来源

- [MCP Tools（2026-07-28）](https://modelcontextprotocol.io/specification/2026-07-28/server/tools)：Tool 作为可调用行为、Schema、按授权发现；第三方 annotations 不是可信执行保证。
- [OpenAI Agents as Tools](https://openai.github.io/openai-agents-python/multi_agent/)：委派工具和 handoff 有不同控制权语义。
- [ACP v1 Session Setup](https://agentclientprotocol.com/protocol/v1/session-setup)：cwd/roots 不是自动隔离，应由实际 Sandbox 执行保证。
- [Kubernetes Scheduling Framework](https://kubernetes.io/docs/concepts/scheduling-eviction/scheduling-framework/)：参考 Filter/Score/Bind 的责任分离，不引入通用 Kubernetes 调度器。
- [SQLite ALTER TABLE](https://sqlite.org/lang_altertable.html)、[Backup API](https://sqlite.org/backup.html)：原子表重建、FK 检查和一致性备份。

**下一步硬门槛：先完成 A 阶段 ADR 与 dry-run 映射规范，尤其审计多份旧 Workspace ToolID→单一内建 ToolID 的授权及 MCP 名称影响；未证明等价就不得真实硬切。**

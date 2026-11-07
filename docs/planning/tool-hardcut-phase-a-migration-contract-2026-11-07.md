# 阶段 A：Tool 归属硬切的一次性迁移与 dry-run 合同

> **状态**：合同已冻结（docs only）；并未实现预检命令、数据库升级或迁移执行。
>
> **决策**：[ADR-0064](../architecture/adr/0064-tool-provenance-and-execution-placement-hardcut.md) · **证据**：[归属盘点](tool-hardcut-phase-a-inventory-2026-11-07.md) · **阶段计划**：[总体规划](tool-workspace-sandbox-agent-hardcut-2026-11-07.md)

## 1. 适用范围与目标

本合同是阶段 B–E 的实现与验收约束，不是当前 Runtime 的功能声明。

- 从每 Agent/每 Sandbox 都创建内部 Provider 的工具模型，**一次性**迁移到 `Tool.SourceRef(provider|agent|native)`。
- Agent ToolID 在能证明旧 Provider Facade 与 Agent 一对一时继续使用；旧 Sandbox Workspace/Jobs ToolID 明确退役，映射到新的固定内建 ToolID 与**旧调用目标的范围约束**。
- Profile/Policy/Approval/Credential/MCP Source/Direct/Hybrid/ToolPlacement、历史 Audit/Task/Job 等引用必须一起迁移或明确阻断。
- **不允许**旧数据被静默丢弃、旧 Profile 权限被扩张、未知任务被重新执行。没有证明等价 = blocked，而不是 default allow。
- 不将旧 ProviderID 当作新的授权主体；旧新工具身份转换只能发生在离线迁移中，或历史 Audit 的只读解释中，不能运行时 fallback。
- 所有真实数据操作都等阶段 E 的测试、干跑报告及维护切换授权；本阶段不访问真实数据库。

## 2. 严格的授权等价约束

将一次可执行的业务操作抽象为：

`Attempt = (Principal, Origin, Profile/Owner Scope, Tool Operation, WorkspaceRef, SandboxID/AgentID/ProviderID, Effect, Input Bounds, Credential Scope, Policy/Approval, Budget)`。

有效准入须满足全部检查：

`Profile ToolBinding ∧ ToolBinding TargetScope ∧ Workspace grant ∧ Sandbox grant/acquire OR Agent grant + Agent workspace grant ∧ Policy/Approval ∧ Runtime Budget/Isolation`。

上述逻辑的逻辑分支应由执行类型选择，不能误认为 Agent 的 Tool 调用必须获得 Sandbox grant，也不能省略 Provider Activation 对外部服务的独立授权。

### 2.1 权限单调性

对每个真实 Principal/Profile/Operation/Target 的授权语义：

- `GrantedAfter ⊆ GrantedBefore`；任何新准许的 Workspace/Sandbox/Agent/Tool 组合必须在旧系统已经有同等调用路径和独立授权的证据。
- 结果约束不得变弱：`deny > require_approval > allow` 为更严格的判断优先级；新策略不能把旧 `deny`/`require_approval` 静默变为 `allow`。
- 任何新 TargetScope 是 **ToolBinding 的收紧条件**，不是新的默认 grant；必须与当前 Sandbox/Workspace/Agent Grants 再取交集。
- Profile 既有 alias/title/description、CredentialAllowlist、concurrency、MCP Direct subset 需保留或因无法合并而阻断；不得用最后一条覆盖第一条。
- 迁移可以明确收紧，在不被误当作无损兼容的前提下经操作人确认；不能因新 native Tool 具有更多可选 Sandbox 而扩大 Profile 原可达目标。
- 不能靠读取一笔当前样本策略得到“等价”；Policy Match 的 Source/Tool/Action/Origin/Scope/ResourceFact 选择条件必须逐类型转换，并覆盖最不利的 deny、require_approval、未来 Rule revision。

**实例：防止静默扩权**

旧 `Profile P` 绑定 `Sandbox A` 来源的 `workspace.read`，没有绑定 `Sandbox B` 的 workspace.read；但 P 通过其他路径已有 B 的 `file.read` grant。新系统即使只有一个 native workspace.read ToolID，**P 对 B 仍不可调用**。只有新 ToolBinding 保存 `AllowedTargets={A}`（含旧 facade 下真正批准的 secondary alias）并在 Invocation preflight 与执行前检查，才能维持限制；单独依赖 `sandbox_profile_grants` 不够。

若 Profile 曾分别绑定 A、B 的 workspace.edit，但 A/B 的 approval、presentation、concurrency、Credential refs、Hybrid direct 不等价，不能自动合并为一份 ToolBinding；必须阻断，进行显式受限重配置并记录影响。

## 3. ID 迁移策略

| 类型 | 身份策略 | 无法证明时 |
|---|---|---|
| 外部 Provider Tool | 原 ProviderID、ToolID、SourceKey 原样保留，SourceRef 指向 provider | 阻断，不重建 Provider |
| Agent as Tool | AgentID 不变；合法一对一 facade 的 5 个操作 ToolID 尽量保留，SourceRef 直接为 AgentID | 假同名/缺 pin/两个 facade 抢占/操作缺失阻断 |
| native Workspace Tools | 单一可信注册 `native:workspace`，每个操作一个**全新、由宿主固定并可预检的 ToolID**；旧 per-Sandbox ToolIDs 仅用于迁移映射 | ID 冲突或多对一限制不兼容阻断 |
| native SSH Jobs | 单一可信注册 `native:jobs`，每个操作一个**全新、由宿主固定并可预检的 ToolID** | Job 目标/未终态恢复无法保持阻断 |
| Source-first ID | 从 `sourceKind/sourceId` 稳定编码；ProviderID 只在 provider 类型保持 | 同名/不合法编码/交付名称冲突阻断 |
| WorkspaceRef | 稳定新逻辑标识；从经过 owner+canonical root 验证的历史登记映射，跨远端只能使用真实 Placement 证明 | 无法证明同一项目或 owner，保留独立候选，不自动合并 |
| SandboxID/Agent TaskID/SessionID/SSH recovery IDs | 原权威身份不变 | 不能转义/不能恢复则阻断 |
| 历史 ToolID | 用于报告和 Audit 溯源的只读关联 | 严禁 Invoke 时自动映射到新 Tool |

阶段 B 需要确定固定 native ToolID 清单和检查其与现有数据库碰撞；dry-run 在**未写库时**也必须能报告精确 `oldToolId → newToolId`。不接受在正式升级事务内随机生成且 dry-run 不可知的 native ID。

旧 Workspace Tool Service 可能配置了一个 default Sandbox 和多个 `workspace_tool_targets` secondary alias；映射 TargetScope 应由旧 ToolID 的**完整已注册目标集合**以及相关 grants 求得，不能只使用旧主 SandboxID。

## 4. dry-run 输入、安全边界与输出

### 4.1 输入与作用

建议实现隔离的 admin-only `tool-migrate assess --db <path> --format json`（命令名称为规范建议，阶段 E 可定名）。read-only SQLite consistent snapshot：

- 检查所有相关 `*_schema_migrations` 版本是否为已知支持基线；不存在未来 schema 或不完整历史升级。
- 从共享 SQLite 的一致性只读事务查询 Catalog、内部 facade、Provider Config、Profile + MCP Delivery、Policy、Agent Workspace/Task/Approval、Sandbox/Job/Leases、Audit 所需 metadata。
- 提供方连接、ACP CLI、Sandbox Driver、Credential Keychain、远端 SSH Job 不得因 **assess** 被探测、启动或写入。
- 对尚需活体验证的 Workspace 文件根或远端 Job 恢复能力，只能返回 `unverified` 与阻断项；不能因为代码静态声明 `ready` 即证明实际有权访问。
- 不返回 Prompt、明文 Secret、文件内容、原始 Agent Transcript、Job 日志或远端凭据，路径应以 owner-safe 的不敏感摘要展示。
- 不删除、合并、改变任何 Profile/Agent/Sandbox/Policy；report 可写在用户指定的独立输出文件，但不能写入目标 DB。

### 4.2 最低机器可读 Manifest

`reportVersion` 固定版本；字段类型和 ID 由阶段 E schema fixture 测试。示意（所有值为纯示例）：

```json
{
  "reportVersion": 1,
  "mode": "assessment",
  "status": "blocked",
  "snapshot": {
    "schemaVersions": {"catalog": 2, "profile": 7, "policy": 3, "agent": 19, "sandbox": 19},
    "databaseFingerprint": "sha256:<deterministic-metadata-digest>",
    "runtimeWriterQuiesced": false
  },
  "toolMappings": [
    {
      "oldToolId": "tool_example_workspace_a",
      "oldSource": {"kind": "sandbox_facade", "id": "prov_example_a"},
      "newToolId": "tool_native_workspace_read",
      "newSource": {"kind": "native", "id": "workspace"},
      "operation": "workspace.read",
      "allowedTargetRefs": ["sbx_example_a"],
      "action": "merge",
      "status": "blocked"
    }
  ],
  "workspacePlacements": [],
  "profileBindingDiffs": [],
  "policyRuleDiffs": [],
  "mcpExportDiffs": [],
  "agentTaskEvidence": [],
  "sshRecoveryObligations": [],
  "conflicts": [
    {
      "code": "BINDING_CONSTRAINT_COLLISION",
      "resourceRefs": ["profile_example", "tool_example_workspace_a"],
      "reason": "Two legacy bindings require incompatible policy constraints",
      "requiredAction": "explicit_owner_review"
    }
  ],
  "counts": {"toolsBefore": 0, "toolsAfter": 0, "unresolvedBlockers": 1},
  "manifestDigest": "sha256:<canonical-report-digest>"
}
```

示例中的 `tool_*` / `sbx_*` 为**非真实且非合法生产 ID 的说明性占位符**，不供任何迁移脚本照抄。正式字段应对身份对象进行严格验证。

Manifest 必须包含：

1. **Source 映射**：旧 Provider/Facade → 新 `SourceRef`，owner 证据、插件安装 pin、缺失/冲突原因。
2. **Tool 映射**：旧 ToolID/SourceKey/Schema hash → 新 ToolID/SourceRef/操作；保留/多对一/退役动作、目标范围与不确定性。
3. **Workspace 映射**：Agent 默认根、附加 alias、旧 Provider target alias、Sandbox root/真实 mount 的严格对应；不能把两个同名项目默认合并。
4. **ProfileDiff**：每个 Profile 的旧/新 ToolBinding、TargetScope、policy/concurrency/credential/presentation、selected Skill、MCP 模式/Direct 子集；零扩权证明。
5. **PolicyDiff**：规则 ID、`providerIds` / `toolIds` / `resourcesAny` / `actions` 的转换结果，Policy match 的语义一致性或收紧/阻断理由。
6. **MCPExportDiff**：每个 Profile 的原 `tools/list` 直接导出名、`mender_search_tools` 源键、new export name、碰撞、下游客户端 config snapshot 影响；变化须明确可见并经审核。
7. **Task/Job 证据**：Agent task/native session/approval/unknown/continuation、Sandbox leases、SSH obligation/recovery event 以及远端 job ownership 能否可靠继续读取；不可用时阻断。
8. **Schema/DB**：旧新行数/唯一索引/FTS/trigger、需要删除的 facade provider configs、外部 Provider 引用保持不变、版本/完整性预计结果；不可输出 secret。
9. **修复建议**：明确哪个资源/规则/授权需人工处理，不自动修改用户的 Profile 或 Policy。

同一数据库快照与相同 Registry 版本应生成相同内容/顺序/摘要（剔除报告生成时刻等非语义元数据）；升级前必须复核快照指纹，防止 TOCTOU。

### 4.3 状态和阻断分级

- `ready`：零 unresolved blockers，能证明权限单调性，报告无不可解释映射；**不代表无需停机/备份或已部署**。
- `blocked`：有任一 P0，系统不允许真实升级。
- `requires_review`：仅存在非 P0 的、需要操作人审阅/确认的破坏性外部名称或业务显示变化；若尚未确认，仍不可自动升级。批准前可单独对比，但不能写库。
- `unverified`：某项必要证据无法用只读 DB 得到，视为阻断，不能等同通过。

最低冲突码：

| Code | 阻断原因 |
|---|---|
| `SOURCE_OWNER_CONFLICT` | 资源→Facade 非唯一、欠 pin、跨 owner 或配置不可信 |
| `TOOL_ID_COLLISION` | 预分配 native ToolID 与任何已存在身份撞车 |
| `BINDING_TARGET_EXPANSION` | 新 ToolBinding 可到达旧绑定不允许的 Workspace/Sandbox/Agent |
| `BINDING_CONSTRAINT_COLLISION` | 多个旧 Binding 的 policy/alias/credential/concurrency/direct 不兼容 |
| `POLICY_SEMANTIC_DRIFT` | deny/approval/Provider activation 规则无法无损或更严格转换 |
| `WORKSPACE_IDENTITY_AMBIGUOUS` | Workspace owner/root/后端 Placement 不能严格证明 |
| `RUNTIME_PLACEMENT_UNVERIFIED` | 不支持能力/隔离、Target root 无法验证或 generation 已变化 |
| `TASK_STATE_UNCERTAIN` | 任务/会话/审批未终态且迁移会损失可信观察权 |
| `SSH_JOB_RECOVERY_UNPROVEN` | SSH Job 恢复/取消依赖被拟删除的 facade ID 或丢失后端凭证 |
| `MCP_EXPORT_COLLISION` | 新 Direct/Hybrid 导出名字冲突或不可审阅的重命名 |
| `SCHEMA_OR_REFERENTIAL_INTEGRITY` | 缺表、未知版本、无损映射失败、trigger/FTS/FK 不一致 |
| `BACKUP_OR_WRITER_UNSAFE` | 实际升级没有可恢复备份、仍有旧写入者或维护租约无法获取 |

## 5. 实际升级的强制流程（阶段 E）

1. 运营负责人选择维护窗口，禁止旧 Runtime 新提交、Agent/SSH Job 任务启动和新 MCP 调用；已存在的 Task/Lease/Approval 由权威 owner 收口，不视为自动取消成功。
2. 确认 manifest 来源于最新一致性快照，`ready` + 人工审核的 export changes，且无 P0。必要时隔离远端未决作业，仍维持被允许的只读恢复路径。
3. 使用 SQLite 官方 Online Backup API 获得一致性整库备份并验证可恢复，包括 WAL 内容；备份与新版本的二进制版本/插件安装 pin 一同记录。原始数据库文件不替换丢失凭据后端内容。
4. 获取有界维护排他控制，重新核对 live DB `schemaVersion`/`manifestDigest`、旧写入进程和持久 Lease/任务状态；不匹配立刻拒绝，不以旧报告继续。
5. **所有业务 Store 开始正常初始化/执行之前**，在基础设施迁移协调器中开展**单一 SQLite 事务**：创建新 Workspace/Target 表、重建 Catalog Tool/SourceRef/FTS、迁移 Profile/Policy/MCP/Agent Placement refs、重建 commit-point guards、退役内部 Provider facades/配置/旧 target aliases，写入新版本记录。
6. 同一事务中执行/验证 `foreign_key_check`、重建后的关键引用/索引/FTS 与计数。`integrity_check` 可在提交前后进行只读检查并记录，若提交后失败不得运行新 Runtime，需恢复备份。任何 DDL/验证失败回滚。
7. 仅新版 Runtime 运行新来源和执行路径；旧版遇到新 schema 必须拒绝启动。**回退** = 停止新版 → 从升级前的整库备份恢复 → 恢复匹配旧二进制；不能依赖双版本模式或“降级迁移”。
8. 验证 CLI/HTTP/Wails/MCP：同 Profile 的实际可见 Tool、权限、直接导出名与 manifest 一致；真实 SSH Jobs/Agent unknown 状态的观测须与后端证据一致，绝不自动重试。

## 6. 验收测试矩阵与退出条件

| 测试 | 核心证明 |
|---|---|
| 空 DB 新建 | 只生成 native 固定 Tool Registry 和真实 Provider/Agent 来源；无内部 facade |
| 旧 Agent 1:1 | 原 AgentID/5 ToolID/Task evidence 保留；错 pin、重复配置被阻断 |
| 多 Sandbox 多 Workspace | 单组 native Workspace Tools；旧 Profile A 不能越权到 B；撤权/并发竞态失败关闭 |
| Sandbox Jobs | 停止/重连/未知/持久 JobID 恢复安全；无任务重放或 Host fallback |
| 不同 Profile Binding | 别名、并发、审批、credential、Hybrid direct 的多对一冲突被准确报告 |
| Policy | ProviderID → typed SourceRef 与 ToolID/Action/Fact 组合的 deny/approval 语义保留 |
| 同名 Agents 与源 | AgentID 精确；Catalog 源键和 MCP 导出不被覆盖 |
| 跨环境根 | Local/Docker/SSH 的同名路径不能被猜作同一 Workspace，需真实 Placement 关系 |
| 导出与 cursor | Source-first、Direct/Hybrid、MCP name、stale cursor、SDK/CLI/Wails parity |
| 迁移故障注入 | 任一中途错误数据库仍原样；重启不重建重复 native Tool 或 Profile grants |
| schema & concurrency | FK/trigger/FTS、交叉 Context 写入竞态、未决 Lease、未来版本拒绝 |
| 恢复 | 升级前完整备份恢复与旧二进制可启动，所有历史审计/Task/Job 元数据仍可解释 |

**退出条件**：`pnpm check`、`pnpm security:check`、相关 Go race/contract/parity、真实 Wails 客户端和独立临时 DB 端到端通过；真实运行库只有在 dry-run 零阻断、备份恢复演练成功且经过明确运维授权后才迁移。

## 7. 当前阶段 A 交付边界

这里记录的是未来设计合同，而不是实现结果。当前无 dry-run CLI、无新 SourceRef/WorkspaceRef DB、无迁移、新 MCP 接口、新运行时匹配、工具 ID 合并或实机验收。阶段 B 首先需要定稿真实 native ToolID 注册表和受限 ToolBinding target scope 的类型及安全测试，然后才能进入开发。
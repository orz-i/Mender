# Profile 统一能力套件与交付整合方案（2026-10-23）

- 关联架构：[ADR-0058（Proposed）](../architecture/adr/0058-profile-unified-delivery-root.md)
- 范围：Mender 对外能力供应，**不是** Agent runtime/harness/Agent Loop。
- 基线：`f0aec76` 后的 Profile 独立页面 + Source-first ToolBinding + Skill Library + Runtime-managed MCP。
- 规则：文档先行、逐阶段提交、端到端证据；不更改正在运行的真实 Profile/Tunnel、用户目录或数据库。

## 1. 用户问题

现在“做一个给 Agent 使用的套件”的流程跨三个入口：Profile 选择 Tools → Servers 发布 MCP → Skills 直接部署/导出插件。一个发布结果看不出它包含哪几个 Profile/Skill，也不能保证 CLI/Agent Plugin 从同一个边界交付。Skills 资源列表又混入了文件系统写入，与用户理解的“资源”概念不一致。

**用户结果：**“创建一个 Profile，选中指定来源的工具和具体版本的 Skills，选择交付到本机 MCP、项目 Skill 目录、CLI 或 Agent Plugin。随后能在同一个 Profile 查看连接方式、目标、版本、状态和更新。” 宿主和入口可不同，但访问权不得高于这个 Profile。

## 2. 审计证据（2026-10-23，仅源代码/既有验收文档；非本轮 Wails 截图验收）

| 证据 | 当前真实行为 | 对实施的约束 |
| --- | --- | --- |
| `backend/internal/profile/domain/profile.go` | Profile = slug/name/mode/tunnel/ToolBindings/revision | 缺少 Skill 成员关系；MCP mode 非通用 Profile 配置 |
| `backend/internal/skill/application/deployments.go` | 单个 Skill 安装直接 materialize 到 Target，记录 Skill DeploymentID/InstallationID/digest/path | 无 ProfileID，不能仅移动前端按钮 |
| `backend/internal/skill/infrastructure/agentplugin/exporter.go` | 导出仅含已选 Skills 的 portable `plugin.json` + `skills/`，至少一个 Skill | 目标应改为 Profile；MCP-only 插件仍待扩展 |
| `backend/internal/bootstrap/mcp_servers.go` | HTTP Server 为运行期内存实例；局域回环；Tunnel 生命周期从 Profile 绑定 | 不得把“启动成功”误报为外部就绪；目前重启不自动恢复 |
| `backend/internal/cli/tool.go` | Search/Call 可不带 Profile 走 Catalog/Owner scope；Describe 完全 unscoped | 导出 CLI 必须强制 Profile 约束，不能复用管理员自由访问行为 |
| `packages/app/src/domains/profiles/profile-detail-page.tsx` | 概览 / Tools；无 Skills / Delivery | 新增内容分层，不再塞回一个巨型编辑表单 |
| `packages/app/src/domains/skills/skills-page.tsx` | 安装、信任、启用和部署在一个页面 | 保留资源管理，迁移部署职责 |
| `packages/app/src/domains/servers/servers-page.tsx` | 通过独立页面选择 Profile 后启动/停止 Server | 收入 Profile 的 MCP 发布与实例视图 |
| `docs/planning/profile-native-acceptance-2026-10-21.md` | 原生 Profile 页面分拆已验收 | 不回退为多层弹窗或高密度大表单 |

本轮未确认运行中的原生窗口、网络端口或真实 Profile 状态，生产数据迁移需前置只读验收。

## 3. 目标数据关系（逻辑，不要求照字面新增万能表）

```text
Catalog Tools ───────┐
                      ├── Profile (exact ToolBinding + exact SkillInstallation refs)
Skill Library ────────┘          │
                                ├── MCP Delivery
                                │     ├── local HTTP Runtime instance (+ optional Tunnel)
                                │     ├── stdio client-launch instructions
                                │     └── managed remote HTTPS only where authenticated
                                ├── Agent Skills Delivery → explicit project/user location
                                ├── CLI Delivery → profile-scoped executable/config recipe
                                └── Agent Plugin Delivery → pinned portable package (+ optional MCP config)
Policy / Credentials / Grants / Audit remain execution boundaries, never copied into a plugin.
```

Naming recommendation: keep `Profile` as product name. User-facing section = “交付/Deliveries”; internal use adapter-specific deployment. Do not introduce additional "Harness","Publication","Release" entries in the navigation.

Suggested minimal identity: `ProfileID` as owner, `ProfileRevision` for desired selection, `SkillInstallationID` for each pinned membership, `DeliveryID` for materialized process/file/artifact where lifecycle really exists; `DeploymentID` can remain as the existing Skill exact deployment record if reused. Avoid parallel universal ID if no durable state is needed (e.g. one-off copied CLI command).

## 4. 用户体验与导航

### Profile 清单（`/profiles`）

每行显示 **名称**、**已选工具数量**、**已选 Skill 数量**、**交付概况**、查看详情。Slug 等实现细节弱化但可检查。不在清单中提供多个直接部署按钮。

### Profile 详情（`/profiles/:slug`）

- **概览**：说明、选定的 Tools/Skills 数量、已交付目标摘要、一个首要下一步（“添加资源”或“新增交付”）。
- **工具**：沿用 Source-first 来源→精确 ToolID 的读写职责及授权约束。
- **技能**：只从受信任/已启用的安装资源里选择指定版本；引用/移除不直接写 Agent 主机目录。能查看引用的原始 Skill 及版本。
- **交付**：列出该 Profile 的 MCP 服务实例、Skill 目标、CLI 使用方式、Agent Plugin 导出记录。**新建交付**为唯一主要入口。每条显示准确状态、目标、更新时间/版本以及其特定操作。

Profile 编辑页保留显式编辑和来源级工具管理；不把全部交付选项放进“新建 Profile”表单。

### 新建交付（`/profiles/:slug/deliveries/new`，独立页面）

1. **选择方式**：MCP、Agent Skills、CLI、Agent Plugin。每项一行简述“给谁用/在哪里用”，不展示所有协议字段。
2. **目标与配置**：仅按所选方式展示必要设置。MCP 再选本机 HTTP / stdio / 已配置远程路径；Skill 再选项目或用户级范围及目录；CLI 展示 Profile 限定的入口；Agent Plugin 选兼容目标/导出目录/是否包含可连通的 MCP 配置。
3. **核对并执行**：清楚标识 Profile、已选资源、精确版本、落盘位置或网络连接范围、凭据处理、写入/启动副作用，之后发布。用户取消不写目录、不启动进程。

**状态语义：** MCP 为“未启动/启动中/运行中/通道就绪或异常”；Skill 为“已部署/内容变化/缺失”；CLI 为“配置已生成/可复制”（不声称“在线”）；Agent Plugin 为“已导出/版本已过期”。一处概览可以统计交付，但绝不使用无证据的“全部运行正常”。

### Skill Library（`/skills`）

保留导入、校验、检查内容、信任、启用与版本信息。删除独立“部署 Skill”主流程；改成“添加到 Profile”，具体部署只从 Profile 发起。被 Profile 引用时无法静默删除安装；未归属的老 Deployment 在迁移前要明确显示待处理，不能消失。

### Server / Tunnel

Server 不再是一个用户需要预先掌握的独立发布步骤。MCP 运行状态和进程控制归到所属 Profile 的交付详情。Tunnel 可保留高级资源管理（连接配置和凭据），但用户从 Profile 选择已有 Tunnel；不把厂商配置复制入 Profile。

## 5. 交付模型与安全细节

- **Scope**：下游 MCP、CLI 与 Plugin 生成的 MCP 连接都须按确切 ProfileID 暴露 ToolID，并在调用时二次鉴权。管理员 CLI 的 Catalog Scope 只能出现在受信任本机管理命令，不能出现在生成套件。
- **Skill**：使用现有 `DeploymentSource` trust/enablement 与 digest；Profile 固定安装ID；批量部署需要全量资源预检和失败补偿，不能有一半文件已部署却显示“成功”。
- **更新**：Profile 对工具成员变更后 MCP 下一次请求动态刷新；文件型部署和已导出插件需要显式更新，显示旧版本标记，不自动覆盖用户目录。
- **Remote**：两类不同问题：(A) 远程 Mender Runtime 提供服务；(B) 将一个 Skill/CLI 安装到 Agent 所在远程机器。不要因存在 Tunnel 就宣称已支持 (B)；具体 Host/Sandbox/SSH 适配器要审核权限、路径、连接与回滚。
- **Plugin**：Portable Agent Plugins 1.0 是最先支持的明确导出目标。可借助 Mender CLI stdio 启动配置或真实可达的 HTTP MCP URL，但不能往包内写凭据、localhost 临时端口或 Tunnel API key。兼容特定客户端另行实现适配，不新增 Mender Provider 类型。
- **Host**：Desktop、Web、CLI、daemon 的入口能力不同。Web UI 不得拿远端服务器路径误表示为操作员 Mac 的本机目录；在权限缺失时明确不可用。

## 6. 分阶段实施与证据门槛

### A — 决策、边界与迁移盘点（当前提案）

审计 ADR/API/SQLite 数据和运行窗口；本提案只提交文档，不触碰生产数据，不改动现有 Servers/Skills 行为。达成数据语义、目标交互和无兼容桥接的不变量后进入编码。确认旧部署记录是否存在；无记录也要有非零数据迁移测试。

### B — Profile Skill membership

新增 Profile→exact SkillInstallation membership，独立关系持久化、revision/fence、引用保护、跨上下文只读投影；打通 Go application、Management HTTP、Wails、OpenAPI SDK、CLI、Profile detail/edit。分两次原子提交：后端合同/测试，前端选择与使用状态。不得在选择资源时部署。

覆盖：trusted/enabled 检查、disabled/upgraded Skill、删除引用保护、Profile revision 变化、并发更新、其他 Profile 不变、migrations。

### C — Profile-owned Skill Delivery

把单一 Skill materialization 上移为 Profile 交付用例；精确选择 Profile 当前 Skill 集、目标根目录、原有路径防逃逸/漂移；记录关系或部署清单，失败补偿；将 Skill 库中的“部署”操作硬切为“添加到 Profile”，不保留双写。先处理既有部署数据明确归属，再删除旧入口。

覆盖：项目/用户范围显式选择、冲突文件拒绝、多个 Skill 失败补偿、磁盘 drift、source trust 被撤回、已存在无归属部署、取消无副作用。

### D — MCP Server 移入 Profile

复用 Runtime Start/List/Restart/Stop 与 Tunnel 健康判据，通过 ProfileID 查询其 server 实例。Profile 详情增加交付选项与 Server 实例视图，原 Servers 顶层入口在完整 UI 验收后移除（旧路由直接转向 Profile，若不需要兼容则删除而非平行维护）。stdio 只给 copyable client-launch config，不假装管理一个可启停的 stdio daemon。

覆盖：一个 Profile/多个实例，地址冲突自动分配、运行中热更新、断开 Tunnel 的 degraded/failed、关闭进程和权限回归，Web/Wails 同行为。

### E — CLI 与 Agent Plugin 基于 Profile

导出的 CLI 配置必须强制带 Profile 或固定不可扩权的作用域；对 `tool describe` 增加 Profile-scope 限制，`tool search/call` 禁止发布态回退到 Catalog Scope。插件导出从 Profile 选取固定安装和可选 MCP 配置，校验 portable schema，支持 skills-only、MCP-only、组合包，拒绝不稳定/不可访问地址与内嵌凭据。

覆盖：跨 Profile 拒绝、动态工具更新、plugin schema 校验、内容 digest、故障恢复、被撤销 Skill 的旧包版本提示、客户端实际加载。

### F — 远程模式 + 综合验收

先验证远端 Runtime 与稳定受认证 MCP 入口；远程安装 Skills/CLI 仅在有可信 target-side adapter 时启用。检查真正的远程 Host 目录所有权和路径、网络故障、凭据隔离、版本/漂移、审计与可回退机制。

Go（目标包+Race Detector）、TS strict/i18n/lint/Vitest、frontend build、真实浏览器、Wails 最小窗口、真实 MCP stdio/HTTP 及一个受信任客户端插件加载；真实磁盘写入只用一次性测试目录。**不得把纯模拟/静态编译写成生产实机验收**。

## 阶段 B 实施及测试记录（2026-10-23）

已建立 Profile→确切 SkillInstallationID 的成员引用，持久化于 SQLite `profile_skills`（Profile schema v6）。保存依赖原有 Profile revision/CAS；Skill 内容与 Trust 仍属于 Skill Library。运行时在 Profile 与 Skill schema 就绪后配置跨连接 DB 保护，避免被 Profile 引用的 managed Skill 被外部 SQLite 客户端直接取消信任、停用或删除。内置 Skill 不存在于普通安装表，引用通过现有 Skill Library 精确身份解析；并未复制内置内容或引入第二种 Skill 身份。

对外已接入 `PUT /profiles/{slug}/skills`（required `installationIds` exact array）、OpenAPI 生成 SDK、Wails 绑定、`mender profile set-skills` 和 Profile 详情→Skills→独立编辑页面；变更为选择操作，不创建 Skill Deployment、Agent Plugin 或 MCP Server。引用不可用的既有安装在 UI 中继续可见，只有明确删除才改变成员资格。

范围和测试：
- `go test ./backend/...` 全部通过；Profile、Skill、Runtime、HTTP、Wails、CLI 的包测试覆盖新增行为，数据库独立连接删除/停用阻断有验证。
- `go test -race ./backend/internal/profile/... ./backend/internal/skill/infrastructure/sqlite ./backend/internal/bootstrap -run 'TestProfileSkill|TestProfileSelectsOnlyTrustedEnabledExactSkillsAndGuardsRemoval' -count=1` 通过。
- `pnpm exec vitest run --maxWorkers=6 --reporter=dot`：65 文件，411 项通过。 `pnpm lint`、`pnpm typecheck`、`pnpm i18n:check`（1509/1509）、`pnpm build`、`pnpm desktop:architecture:check` 通过。
- OpenAPI SDK 和 Wails 绑定生成可重现；文件与原仓库差异将在当前提交中一并纳入，提交后再运行一致性门禁。
- 未运行真实生产数据库 schema 升级，也未重启桌面进程、MCP Server 或 Tunnel；Wails 真机视觉验收与真实 Profile 写入待业主下一次受控部署。仅在一次性测试数据库和目录验证。
- 旧 `Skills` 页面当前 Deploy 控件、旧 SkillDeployment 不变；必须等阶段 C 新 Profile-owned 部署和存量迁移完成，再删除旧入口，防止不可管理或失联的数据。

## 阶段 D1 — Profile-scoped MCP delivery UI（2026-10-23）

在仍使用 Runtime 管理的 ephemeral MCP HTTP Server（ADR-0016）的前提下，交付管理入口移入 Profile：详情「交付」分区列出当前 Profile 的 MCP 端点及状态，`/profiles/:slug/deliveries/mcp` 承担启动、停止、重启、复制 Endpoint/连接配置及 Tunnel 当前状态。复用既有 `ServersPage` 的管理逻辑，并以 `profileSlug` 显式约束候选 Profile、列表及启动请求；不新增 Server 数据表、Profile 服务进程或替代 Runtime。顶层侧栏 Servers 已移除；原 `/servers` 路由仅转到 Profile 清单，避免遗留并行发布入口。

已验证：跨 Profile 不可在 UI 启动/停止/重启他人的 MCP 实例；同一个 Profile 多实例仍可列出；独立路径可返回配置档。组件与管理集成测试覆盖；`pnpm exec vitest run --maxWorkers=6 --reporter=dot` 66 文件 414 项通过，`pnpm typecheck`/`pnpm lint`/`pnpm i18n:check`/`pnpm build` 与 Desktop 架构检查通过。既有 Docker 定时刷新测试仅加异步安装等待，解决全量并行执行偶发 race；无生产逻辑修改。

**未完成/不冒充**：真实 Wails 对 Profile MCP 启停、真实 Tunnel/连接器状态、重建后 Native UI、远端 auth HTTPS、MCP stdio client config 尚需后续验收与收口。原运行中的 Wails/MCP/Tunnel 未触碰。下一步为 Profile 所属 Skill 部署语义与旧部署无归属数据处理，不可提前删除 Skill Library 旧部署记录。
## 7. 需避免的过度设计

- 不引入 ACP v2、Agent Loop 或新的 Session/Task 生命周期。
- 不创建“技能提供方”、“部署提供方”、“Harness Agent”作为额外资源层。
- 不按每个 Tool 生成 Skill，不为兼容旧 UI 保留两个相同发布入口。
- 不把 Profile 的所有 delivery 实现细节塞进一个巨大 DDD 聚合。
- 不默默为已有 Profile 启用自动启动、广域监听、远程 HTTP 或管理权限。
- 不对已有生产 Profile/Skill Deployment/Server/Tunnel 直接执行迁移验证。

## 8. 外部依据（2026-10-23）

- [Agent Skills specification](https://agentskills.io/specification)
- [Agent Plugins 1.0 specification](https://agent-plugins.org/specification)
- [OpenAI portable plugin packaging](https://developers.openai.com/plugins/build/plugins)
- [MCP 2026-07-28 transports](https://modelcontextprotocol.io/specification/2026-07-28/basic/transports)
- [OpenAI Secure MCP Tunnel](https://developers.openai.com/api/docs/guides/secure-mcp-tunnels)

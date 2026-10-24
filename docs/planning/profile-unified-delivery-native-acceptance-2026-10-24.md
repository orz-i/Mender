# Profile 统一交付复核与原生 Wails 验收（2026-10-24）

## 环境与原则

- 仓库：`/Users/mini/Coding/Mender`，HEAD `a531d9e`。
- 实机：Mac mini，`apps/desktop/bin/mender-desktop.dev.app`，原生 PID `27766`；前端 Vite `127.0.0.1:9245`；现有 Profile MCP `127.0.0.1:8765`。
- 直接通过 macOS System Events / Accessibility 查询真实 WKWebView 控件、点击导航/非持久表单，并由 `screencapture` 捕获真实屏幕；**不是在 Chrome/模拟器里验收原生 UI**。
- 数据安全：正式数据库 `~/Library/Application Support/Mender/mender.db` 始终只读核查。文件写入和 CLI 验收只在随机 `/tmp/mender-native-profile-qa.5tYE04/` 中执行。没有重启、停止或重新绑定已有 MCP/Tunnel，也没有持久更改生产 Profile 或授权。
- 本机截图证据：`/tmp/mender-profile-native-acceptance-20261024-{list,detail,deliveries,mcp,mcp-start-cancel,skills,cli}.png`。

## 原生 UI 的真实证据

| 路径 | 观察 | 结论 |
|---|---|---|
| 配置档列表 | 真实导航「配置档」，显示一条 `default`，动态模式、32 个工具、现有 Tunnel | PASS 只读 |
| Profile 详情 | 概览/工具/技能/交付四个标签，32 个工具，0 个 Skill，Tunnel 关联正确 | PASS 只读 |
| Profile → 交付 | 四个明确入口：MCP、Agent Skills、本地 CLI、Agent Plugin；现有 `127.0.0.1:8765/mcp` 显示 `running · ready` | PASS 只读 |
| MCP 交付 | 实际原生页列出运行中的 endpoint、隧道、复制/重启/停止按钮；「启动」对话框仅显示当前 Profile，监听地址 `127.0.0.1:8765` | PASS 查看/草稿；**未点击启动、重启、停止** |
| Agent Skills 交付 | 实际原生页显示「0 个已选 Skill」、无部署记录、目标选择和禁用的部署按钮 | PASS 空状态；生产未写 Skill |
| 本地 CLI | 原生页显示桌面真实 DB 路径，四组 `mender --db <DesktopDB> ...` 命令，Profile 范围正确、复制控件可见 | PASS 查看；复制粘贴结果未核对 |
| Agent Plugin 交付 | 源码及前端自动测试覆盖表单；原生 AX 自动化有间歇性空树和点击超时，未稳定完成打开/选择/取消的全过程 | **未完成原生 UI 验收** |

更正（后续复核）：MCP 启动对话框显示的 `127.0.0.1:8765` 是**期望地址**，并非承诺固定占用此端口。`bootstrap.listenManagedMCPPort` 在 `EADDRINUSE` 时会依次尝试最多 32 个本地端口；启动后返回实际监听端点，UI 帮助文本已说明此行为。`TestManagedMCPPortAutoRotationPreservesExistingListener` 真实网络绑定/重启/保留原监听的回归测试通过。因此前次 P2「缺少端口冲突处理」是误判，不追加重复端口治理逻辑。

## 真正的文件写入与 CLI 执行（完全隔离）

采用 `go build -o <TempDir>/mender ./backend/cmd/mender` 生成原生 macOS CLI，SQLite 仅在 `<TempDir>/runtime/mender.db`，无 mock：

1. `profile create qa-local` 成功，revision 0。
2. `profile cli qa-local` 实际输出 `--db <TempDir>/runtime/mender.db`、严格 `profile tool search/describe/call` 与 MCP stdio 配方。
3. 空 Profile 搜索返回 `tools: []`；对未选 Tool 的 `profile tool describe` 返回非零（exit 5）、错误 `outside Profile qa-local`，未扩大权限。
4. `skill list` 得到内置可信启用的 `mender-tools`，`profile set-skills qa-local <exact installation ID>` 成功，revision 1。
5. `profile export-plugin` 真正在隔离目录写入 `plugin.json` 和 `skills/mender-tools/SKILL.md`，产物 manifest 包含 Agent Plugins 1.0 schema、名称、版本。
6. `profile deploy-skills qa-local --target agents-project --project <TempDir>/agent-project` 成功，在真实文件系统写入 `.agents/skills/mender-tools/SKILL.md`；返回 `profileId`、revision 1、SHA256、`drift=current`。
7. `profile skill-deployments qa-local` 实际读取该记录。无权 Profile `other-profile` 尝试 `undeploy-skill` 被拒（exit 3）。
8. 正确 Profile `undeploy-skill qa-local <deployment ID>` 成功；原真实 Skill 文件不再存在且交付清单变空。隔离 SQLite `quick_check=ok`。

仅在本地 CLI 路径验证 Agent Plugin Skills-only 文件产物。**没有**通过真实客户端加载、验证 MCP-only/混合包远端连接或实际 OAuth；这些场景只在本轮前置自动化测试中覆盖。

## 原生验证后的生产状态

前后对比保持：profiles=1、profile_skills=0、skill_deployments=0、agent_tasks=75、tunnels=1、SQLite `quick_check=ok`。Wails PID `27766` 和 MCP `127.0.0.1:8765` 持续运行。没有触碰现有授权和实际用户的 Skill 文件。

## 剩余验收门槛

- Wails 原生 Agent Plugin 表单：真实目录选择器、取消、导出、错误提示（isolated Profile）；AX 查询在短期内有间歇性空树，不能标记已通过。
- Wails 原生 Skill 选择和部署：生产 Profile 空 Skill，不为测试向真实 Profile 写入；需使用独立配置环境验证 GUI 中的写入/撤销。
- MCP stdio/HTTP 真正的受信任 MCP 客户端握手、list/call、Agent Plugins 客户端安装；远程 HTTPS 的身份、auth、ProfileID/Tool 边界真实验证。
- Mender Desktop 原生生产构建和首次启动/重连后数据迁移验收。目前运行 `wails3 dev` 重构建的可用进程，但本轮没有重新构建重启。
- Accessibility 语义基本可用，未做完整 VoiceOver/WCAG 认证。

## 续验：真实 MCP 客户端与 Agent Plugin 标准验证（2026-10-24）

### 编译后的 CLI、真实 MCP Go SDK 客户端（PASS）

在 Mac mini 上使用独立编译的 `mender` CLI、临时 DB、独立上游 Streamable HTTP MCP Server，以及 `github.com/modelcontextprotocol/go-sdk/mcp` 的真实客户端。非 Mock 的本机 OS stdio 管道与 TCP 连接逐一核查：

| 路径 | initialize | tools/list | tools/call | 判定 |
|---|---|---|---|---|
| `mender serve mcp --profile enabled --stdio` | 2026-07-28 | `upstream__echo` | 实际上游返回 `phase-f-real-wire` | PASS |
| `mender serve mcp --profile empty --stdio` | 2026-07-28 | 空列表 | `upstream__echo` 拒绝 | PASS，范围隔离 |
| `mender serve mcp --profile enabled --http 127.0.0.1:<临时端口>` | 2026-07-28 | `upstream__echo` | 实际上游返回 `phase-f-real-wire` | PASS |

上游调用计数恰为 **2**（两次 authorized、一次 unauthorized 被拒）；测试执行输出为 `/tmp/mender-native-profile-qa.5tYE04/phasef-live-wire.log`，一次性真实 wire harness 为相同目录 `phasef_live_mcp.go`。以上全部只访问一次性 DB（由测试动态创建）及回环端口，不影响用户配置档。

### 现有 Wails HTTP 服务只读实连（PASS）

Mac mini 上没有重启的既有 Wails 原生 PID `27766`，127.0.0.1:8765 原有 MCP listener 在继续工作。独立 Go SDK MCP 客户端连接 `http://127.0.0.1:8765/mcp`，协商到 **2026-07-28**，`tools/list` 返回 **3 个 MCP 顶层入口**。只进行握手和列表读取，**没有任何 `tools/call`、Tunnel 重连、启动/重启/停止服务器动作**。

### 官方 Agent Plugins 1.0 格式验证（PASS）

在真实 CLI 中产生 Skills-only、MCP-only、Skills+MCP 三份完整文件包，采用从 `agent-plugins.org/schemas/1.0.0/{plugin,mcp}.schema.json` 当日下载的两个正式 JSON Schema，使用 Draft 2020-12 校验器逐份验证：

- Skills-only：`plugin.json` Valid，`skills/mender-tools/SKILL.md` 存在；
- MCP-only：`plugin.json` / `mcp.json` 均 Valid，只有一个 `streamable-http` 服务；
- Combined：`plugin.json` / `mcp.json` 均 Valid，并包含完整 Skill 资源。

对格式的 JSON Schema 合规性已证明；`https://mcp.example.com/mcp` **仅为示例 URL**，不代表该端点已存在、可访问或已完成 OAuth。尚未在实际 Agent Plugins 消费客户端中安装，不能声称端到端远程连接完成。

### 本轮原生 GUI 的硬性环境约束（BLOCKED）

续验时 macOS `IOConsoleUsers` 明确返回 `CGSSessionScreenIsLocked=Yes`。真实屏幕截图呈全黑，System Events 可见的 Wails 窗口数均为 0。额外启动的隔离 Wails 进程 PID `50469` 使用 `MENDER_DB=/tmp/mender-native-profile-qa.5tYE04/runtime/mender.db`，经 `lsof` 核实**只打开测试 DB**，但在锁屏期间同样没有可交互窗口。该隔离进程已正常退出，原本生产进程 PID `27766` 保留。

因此不能诚实地声称已经完成原生目录选择器、取消、Agent Plugin 导出反馈和原生 Skill 选择/部署 GUI 的新一轮实机点击。对应 Wails Binding、React 表单测试均通过，但它们不是锁屏情况下的原生 GUI 证据。

### 质量检查、保留风险

- Go 定向测试：MCP 监听端口轮转、Profile export、CLI/Binding/Exporter 场景 PASS。
- 前端定向测试：Profile Plugin、CLI、MCP Delivery、MCP 状态页 **4 文件 / 26 项全部通过**。
- 刻意不修改内置 Skill 真实内容：Profile Skill 只固定 Skill 资源，不重写脚本或强制限制 Agent 主机的独立 shell/owner CLI。内置 `mender-tools` 的管理员 `mender tool` 操作不能被误称为受到 Profile ToolBinding 自动约束；真正限定通道是 `mender profile tool ...` 或 Profile-scoped MCP。
- 截至复验后生产只读核查：profiles=1，profile_skills=0，skill_deployments=0，agent_tasks=75，tunnels=1，`PRAGMA quick_check=ok`；原 `:8765` 监听和 PID `27766` 都在。
- 缺项仍为：**解锁后的原生 Wails UI 完整交付流程**、支持 Agent Plugins 的真实消费端加载、具备真实身份验证的远程 HTTPS 端点，以及生产安装后的重建/重启验收。

**最终判定：本轮补齐了生产 Wails MCP 的安全只读握手、CLI 的真实 stdio/HTTP Tool 发现/调用/越权拒绝，以及三类导出包官方 Schema 合规验证；Phase F 的 GUI、远端真实认证、最终消费端安装仍未全部收口，不能标记完全通过。**

## 续验：真实 Codex CLI 插件安装、多 Profile MCP 消费端冲突修复（2026-10-24）

本节属于后续阶段，更新上文曾写的「真实消费端尚未安装」状态；前面各次验收所记录的历史进程 PID、端口状态只代表当时观测，不代表当前的运行状态。

### 三类 Agent Plugins 的真实消费者验收（PASS）

Mac mini 上已安装的 **Codex CLI 0.159.0**，使用仅位于 `/tmp/mender-native-profile-qa.5tYE04/` 的独立 `CODEX_HOME`、独立 `HOME` 和局部 `codex plugin marketplace add` 注册流程，实际对 Mender CLI 导出的三类包执行 `codex plugin add`：

- Skills-only：`qa.profile` 安装为 `installed=true`、`enabled=true`，在 Codex 插件缓存中有真实的 `skills/mender-tools/SKILL.md`。
- MCP-only：`mcp-only` 安装成功，在缓存中有 `plugin.json` 和 `mcp.json`。
- Mixed：`mixed` 安装成功，MCP + Skill 文件都真实存在；`codex mcp list` 能读取插件的 HTTPS MCP 配置，但并未尝试与示例远端建立连接。

### 真实多插件 MCP 名称冲突（已修复，消费端 PASS）

**复现：** 不同插件分别使用 `https://scope-a.example.com/mcp`、`https://scope-b.example.com/mcp`，但都拥有固定的 `mcpServers.mender`。两个插件在真实 Codex 中均安装成功且启用，`codex mcp list --json` 却只列出一个名为 `mender` 的服务（只保留 A 地址）。

**最小修复：** exporter 在 `mcp.json` 中使用已有名称校验的 `pkg.Name` 作为 MCP server 键，不新增数据库字段、兼容桥、导出参数或运行状态；更新 Bootstrap/exporter 回归断言，新增两包服务名称独立用例。

**修复后真实复验：** 重新编译 Mender CLI，从测试 Profile 真实导出 `qa.alpha` 和 `qa.beta` 两个版本为 `1.0.1` 的 Skills+MCP 包，再使用全新隔离 Codex Marketplace 安装。真实 `codex mcp list --json` 同时返回：
- `qa.alpha` → `https://scope-a.example.com/mcp`
- `qa.beta` → `https://scope-b.example.com/mcp`

两个映射均处于 `enabled=true`，没有相互覆盖。两个导出的 `plugin.json`、`mcp.json` 重新通过 2026-10-24 下载的 **Agent Plugins 1.0 正式 JSON Schema** 验证。这只证明端点配置已被消费端加载，示例 HTTPS 地址不是可认证生产入口，`auth_status=unknown`，**不得声称远程权限和业务工具调用通过**。

### 原生 Wails、运行环境及生产保护

- 使用 `MENDER_DB=/tmp/mender-native-profile-qa.5tYE04/runtime/mender.db` 实际启动过隔离 Wails 进程，文件句柄确认只打开测试 SQLite，且只对测试实例尝试 UI 浏览。原生 Accessibility 在多个同名进程并存时对窗口身份的识别存在歧义，后改为临时独立的包标识及可执行文件名；因 macOS 会话再次显示 `CGSSessionScreenIsLocked=Yes`，**没有执行任何原生 Agent Plugin 导出、Skill 目录选择/部署/取消的 GUI 写入**，不得虚报通过。临时 QA Wails 实例已关闭，临时编译源码已清理。
- 本机同时有 `wails3 dev` 热构建监听。复核过程中观察到原 Wails 开发实例的 PID 发生变化（`66941`→`83109`）；临时 QA 源码曾位于被 watcher 监视的 `apps/desktop`，可能触发热重建，**不可声称原生运行实例始终未重启**。本轮没有直接对生产实例执行管理 API 的 Start/Stop/Restart，也没有修改生产 Profile 或 Tunnel。
- **最新诊断：** 原 MCP 地址 `127.0.0.1:8765` 已无法 TCP 连接；原 Tunnel 客户端仍在运行并指向 `http://127.0.0.1:8765/mcp`。这与 MCP HTTP 实例只存于旧 Runtime 内存、不随进程重建自动恢复的设计相符。恢复真实 MCP 交付需要在原生 Profile→MCP 页面重新启动，不能由验收脚本偷偷操作生产 Tunnel 或授权。**这是当前真实使用可用性问题，优先级高于继续 UI 自动化。**
- 生产数据库全程只读核查：profiles=1、profile_skills=0、skill_deployments=0、agent_tasks=75、tunnels=1、`PRAGMA quick_check=ok`。仓库中的既存四个未跟踪目录未触碰。

### 质量门禁

- `go test ./backend/...` 全量通过；
- `pnpm go:lint`：0 issues；
- `go test -race ./backend/internal/skill/infrastructure/agentplugin ./backend/internal/bootstrap -run 'TestAgentPluginExporter|TestExportProfileAgentPluginPinsExactSkillsAndOnlyExplicitHTTPSMCP' -count=1` 通过；
- `git diff --check` 通过。

**阶段结论：** 实际 Codex CLI 消费端的三种格式安装、多个 Profile MCP 配置共存已通过；远端 HTTPS 服务身份/auth、真实可调用工具、解锁后的原生 Wails 导出/部署 GUI，以及开发 Runtime 重建后 MCP/Tunnel 的恢复仍未闭环。

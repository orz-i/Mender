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

注意：原生 MCP 启动对话框默认监听地址与已运行的 `:8765` 相同，尽管尚未点击启动。这可能造成新实例冲突/误操作，建议后续在 Profile 交付表单增加地址冲突提示或动态端口推荐（P2 UX），但本轮不修改监听策略。

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

**最终判定：已完成真实原生只读操作和隔离真实 CLI/文件交付的实机验收；Phase F 的真实远程连接、原生插件表单、独立数据目录下 Wails 写入以及最终 Agent 消费端连接仍是未闭环项。** 不应在发布报告中把阶段 F 标为全部 PASS。

# Phase 48 — Agent 审批中心与无人值守放行

Status: core approval center and unattended modes accepted in native macOS Wails, per-workspace isolation covered by regression; optional native secondary workspace picker confirmation pending (2026-10-17). Authority: [ADR-0053](../architecture/adr/0053-agent-approval-inbox-and-scoped-trust.md).

1. Docs/ADR and paused cleanup worktree fingerprint; commit only new docs.
2. Agent v12 policy row, atomic automatic approval audit, owner-only APIs, bounded pending list and negative tests. One full cut; no compatibility bridge.
3. Native Wails SDK/client, focused Agent permission settings and global approvals inbox with pending indicator. Default-off, no form wall on inventory, Chinese and English.
4. Synthetic ACP read/read/unreviewable/trusted/revoke and full quality gates, isolated DB and native UI acceptance. Do not silently change user Agent settings. Return to cleanup after owner acceptance.

Existing unfinished provider-v1 cleanup files are intentionally excluded from Phase 48 commits.

## 实施与自动化验收记录（2026-10-17）

- 完整质量门禁：最终 canonical pnpm check 返回 exit 0（收据 /tmp/mender-phase48-approval-final/check3.exit；详细日志 /tmp/mender-phase48-approval-final/check3.log），包含全部 Go 测试及 Race Detector、Go lint、OpenAPI/SDK/插件校验、Wails bindings/architecture、TypeScript、ESLint、46 个 Vitest 文件 / 267 项测试、i18n 1309/1309，以及 Web/Desktop 生产构建。较大的 Vite chunks 提示为非阻断建议；最新原生 Wails Go 可执行文件也已单独编译通过。
- 入口回归提交 ecb9ea3：真实应用路由下验证本地所有者能从顶栏待审批数量直达审批中心，并直接看到跨 Agent 请求，无需进入 Agent 详情或 Task；相关定向前端测试 12/12 通过。
- 后端提交 1a537a2：Agent v12 默认拒绝自动审批；逐次审批 / 工作区读取自动放行 / 24 小时显式信任；原始 AgentGrant、Profile 和 Policy 重新核对，SQLite 原子检查 Task generation、取消、工作区以及当前授权；每次实际允许仍返回 ACP allow_once 并生成不可变审批记录，绝不选择 allow_always。
- 前端提交 6659eba：独立审批中心 /approvals、顶部待处理数字提醒、跨智能体待审队列、按 Agent 与工作区设置授权。Host 高风险放行必须明确勾选风险确认；Web/远端 MCP 不能使用这些 owner-only 接口。组件测试五项通过，TypeScript/i18n 通过。
- 修复提交 cf7d271：客户端不通过读取浏览器当前时间判定信任有效性，由服务端返回生效模式，避免 React render purity 报错及本地时钟偏差。
- 安全审计提交 7422bee、299bb7d：新增 SQLite 不可修改的授权模式变更事件。授权设置、更新、撤销留下模式、AgentID、精确工作区及有效期，不持久化文件内容、凭据或原始 Agent 输入。Profile 原始授权在任务执行中撤销后，后续自动读取被拒绝。对应定向测试、迁移测试和 Go lint 通过。
- 使用 Mac 上一致性生产数据库的独立副本执行了 v11→v12 迁移。原有 58 张保护表的行数与 SHA-256 指纹比对中仅 schema-migration 表增加版本 12。保留 3 个 Agent、17 条 Task、16 条历史审批；新授权记录 0、新授权变更事件 0；quick_check=ok、外键异常为 0。真实生产库没有被迁移或修改。
- 独立签名 Wails QA 包已经构建并成功启动，只访问临时 v12 SQLite 与临时 Agent。但 Mac 被锁屏（登录界面），无法在未经解锁的情况下对原生 WebView 执行 GUI 点击验收；没有尝试绕过密码。QA 进程已单独停止，生产 Wails 与隧道仍运行。前端测试与 Wails owner API 的自动化通过**不等于**真实 GUI 操作完成。

### 解锁后的原生 Wails 实机验收（2026-10-17）

- **运行环境**：用户解锁后，重新构建了独立签名的 Wails 应用，使用全新一次性 v12 SQLite、临时 OpenCode/Grok Host Agent、独立工作区，不触碰生产 Agent/工作区。独立 QA 应用启动时的工作区策略和模式变更事件均为零。
- **导航与审批收件箱**：在真实 macOS Wails 窗口点击顶部铃铛直达 /approvals。接入真实 Wails Runtime 中的可控模拟 Task 后，待审批铃铛和收件箱计数显示 1，再同时显示来自两位 Agent 的 2 条请求及各自 Task、工作区、类型和可核对的目标路径；不需要进入 Agent 或 Task 详情。
- **风险告知与模式持久化**：原生弹窗切换“工作区内读取自动放行”并保存后，SQLite 持久化为 workspace_read，重新打开仍正确选中；切换 24 小时 trusted 时 Host 风险警告明确显示，未勾选风险确认则 Save 禁用，确认并保存后到期约 24 小时。原生界面切回 manual 后该授权被撤销，审计事件保留 set/workspace_read、set/trusted、revoke/trusted。随后 QA 可能再启用读取模式做下述测试；**最终必须恢复 0 条生效授权**。
- **真正的界面审批决定**：OpenCode 的 Read 通过全局收件箱“允许一次”后，Task=completed、审批 allowed_once/owner；Grok 的无安全预览 Execute 仅有“拒绝”，点击后 Task=completed、审批 rejected/owner。另一个故意超过两分钟未处理的 Read 正常 expired、Task=failed，验证失败关闭。
- **无人值守与等待请求放行**：预先启用 workspace_read 后新 Read 自动完成、审批 allowed_once/policy；再次设为 manual 并启动 pending Read，从收件箱选择“设置放行模式”并保存 workspace_read 后，等待中的 Task 自动继续完成，第二条审批为 allowed_once/policy。所有自动放行都只选择 ACP 的原始 allow_once，不写入 Agent allow_always。
- **模拟运行的边界**：为获取可控、可重复的真实 Wails 等待决策，QA 程序通过临时 Go -overlay 编译了**只存在于一次性 QA 包中的执行器**，由临时触发文件提交 Task 到 Wails 正在运行的 Runtime。它不调用真实模型、不读取或编辑用户文件，也不修改 Git 跟踪的生产源代码。这证明了桌面入口、所有者决策和 Task/SQLite 状态交互，**不是**对真实 OpenCode/Grok ACP 模型执行的重新验收，后者另由 Phase 46 的独立证据覆盖。
- **额外工作区**：原生弹窗实际展示注册的 secondary 工作区选项；macOS 原生下拉控件通过合成鼠标/键盘操作不能可靠确认切换，因此**没有宣称该项原生选择通过**。新增 React 测试验证切换 secondary 时默认 manual 且保存作用域固定为 secondary；新增 SQLite 测试验证正在授权的附加工作区必须先撤销策略才能解绑，均通过。
- **质量与数据状态**：新的工作区回归测试提交 `802bf74`，随后执行完整 pnpm check，exit 0（收据 `/tmp/mender-phase48-native-owner-acceptance-20261017/final-check.exit`），包括 Go Race Detector、Go lint、Wails bindings/architecture、TypeScript、ESLint、前后端构建、i18n。QA 期间审计汇总为 4 completed / 1 expected failed，审批为 owner allow 1、owner reject 1、policy allow 2、expired 1；剩余 QA-only 的 workspace_read 授权在最终清理时已通过独立测试数据库的 DELETE/审计触发器撤销，无持久可用的 trusted 授权。

### 交付边界

Phase 48 的**核心审批体验与无人值守机制已通过实机验收**。额外工作区下拉的实际原生选择仍属可在下一次人工 UI 检查中单点确认的项目；该选择的 React 状态管理、服务端授权隔离与防解绑规则已有覆盖。用户生产 Wails 必须在正常构建重启后才能加载 Phase 48 的新 API/schema；不得自动对现有 Agent 启用任何放行，也不能把 Host 原生审批视为强制沙盒隔离。

### 最终交接与实机验收收口

- 核心功能和实机验收按限定范围已收口：原生 Wails 铃铛直达、跨 Agent 排队与批准/拒绝、严格两分钟过期、预授权读取、保存后解除挂起、24 小时可信模式风险确认、持久化/撤销和不可修改审计。QA-only 可控执行器未启动真实模型，亦未修改真实用户项目文件；不能将模拟 ACP 请求冒充真实 OpenCode/Grok 任务执行证据。
- 私有最终验收收据：`/tmp/mender-phase48-native-owner-acceptance-20261017/native-acceptance-receipt.json`。临时测试库 v12，2 Agent、4 completed / 1 expected failed，审批 owner allow 1 / owner reject 1 / policy allow 2 / expected expired 1，最终 **0 pending、0 active authorization**，策略变更事件 7，SQLite `quick_check=ok`、无外键异常。独立 QA app 与测试注入二进制均已关闭清理。
- 完整 `pnpm check` **exit 0**，46 个 Vitest 文件、268 项通过，i18n 1309/1309；额外工作区切换有 React 单点回归和 SQLite 禁止带授权解绑回归，原生下拉选择因 macOS 合成点击限制仍只认定为**可见、未验证真实选中**。
- 当前生产库只读检查：Agent schema **v12**、3 Agent、17 Task、0 自动授权策略和 0 授权变更事件，完整性检查通过。**检查时未发现活动的桌面 Wails 或隧道进程**，不声称已在生产 UI 完成该版本的交互或正在提供连接。保留所有用户数据、旧审批和权限关系。Phase 47 仅在本轮正式收口后独立恢复。

### 边界与 Phase 47 暂停状态

- Host ACP 原生 read/edit/execute 放行是**协作式权限**，不能宣称限制了宿主用户文件系统或网络。受治理的项目读写应使用 restricted-host 及 Mender 的工具执行链。
- 原提交 Principal 的 Policy 在写入之前重新评估；Task、grant 与工作区等条件在 SQLite 中检查。跨领域 Policy 在这两个阶段之间同步改变的严格原子性尚未证明，不能对外宣传为全局不可竞争的策略撤销。
- Phase 47 Provider-v1 清理暂不实施。待恢复的补丁安全保存于 Mac 私有临时路径 /tmp/mender-phase47-paused-before-approval.patch 以及 /tmp/mender-phase48-paused-phase47-worktree.patch，均未混入 Phase 48 的提交或生产数据。

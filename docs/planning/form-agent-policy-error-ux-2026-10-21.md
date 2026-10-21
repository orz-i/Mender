# Mender UI/UX 表单交互收口：Agent 创建与 Policy 保存 — 2026-10-21

## 目标与审计证据

接续任务优先表单、帮助密度和动态 Schema 治理。本阶段只处理新查证的 P1 交互阻断，不进行页面重做。

- **P1 Policy 操作失败不可见（代码确认）**：`packages/app/src/domains/policy/policy-page.tsx` 中 `saveDefault()` / `saveRule()` 的异常统一写到页面级 `error`；两个编辑 Dialog 仍保持打开，Overlay 遮盖页面告警。用户无法在当前操作位置确认是否保存失败，也缺少重试反馈。这是代码级可复现风险，不能在生产策略上诱发失败。
- **P1 Agent 创建缺少字段级解释（代码与原生 Wails 确认）**：`agent-create-page.tsx` 使用 `name.trim()` 和 `root.startsWith("/")` 决定“核对”按钮是否可用，但非法工作目录没有文字错误；字段填入相对路径时仍只是禁用按钮。提交错误则渲染在整个面板上方。
- **P2 信息密度（原生 Wails 截图）**：选择后端界面把“选择后端”在进度和内容标签中重复展示；配置界面在必填名称、工作目录和明确可见的隔离安全设置之间平铺低频可选的默认模型。

本轮真实原生只读截图保留在 Mac：
`/tmp/mender-ux-agent-create-select-before.png`、
`/tmp/mender-ux-agent-config-before.png`、
`/tmp/mender-ux-policy-real.png`。
所有操作仅打开 Agent 草稿、浏览 Policy 页面，未创建 Agent、编辑 Policy 或改变授权。

## 不变量

1. 继续使用共享 React UI、i18n 和现有路由；不改后端、持久化、Profile/Tool/Policy/Agent 身份及任何权限语义。
2. Policy 保存失败要在**当前弹窗内**以可访问文本解释，不丢草稿、不误报保存成功；取消/重新打开不能复用旧错误。原有页面加载与列表操作错误仍留在页面。
3. Agent 创建必须填写合法的名称和绝对工作目录，确认/创建门禁不放宽，根路径传参语义与用户看到的校验一致。
4. Agent 配置默认聚焦必需信息；默认模型放在可展开的高级设置中，不会因折叠消失、清空或改变提交值。主机隔离选择、风险解释以及 Review 阶段仍始终明确。
5. 不向现有生产资源执行任何创建/保存/审批/授权操作。测试通过与原生截图证据分别记录；390px 与页面级缩放需另行核验。
6. 只提交明确业务源码与文档，不纳入 `.agents`、`.pi`、`.commandcode`、`.intent-loop`。

## 计划

- D0：审计合同文档提交。
- D1：Policy 弹窗局部错误状态与原草稿保留；新增失败/重试/取消回归。
- D2：Agent 创建进度减重、可选模型折叠、工作目录校验与错误就地显示；新增中文/英文与数据保持测试。
- D3：lint、typecheck、i18n、Vitest、build；结合已有 Wails/HMR 进行只读/取消复核，记录边界。相关代码按域提交。

## 验收

- Policy 更新默认决策及创建规则失败后，弹窗仍可见、草稿仍在、同一弹窗可读到 `role=alert`，重试成功后关闭；页面错误不冒充弹窗错误。
- Agent 创建相对路径会明确提示绝对路径要求且关联输入，合法路径能正常核对；默认模型按需打开，关闭再打开不丢填写值；创建失败在 Review 操作附近可见，返回配置仍保留草稿。
- 在当前窗口与移动宽度无操作遮挡；如原生 Wails 构建晚于本轮源码或界面无法完整核验，明确标为待重建验收。

## 实施与验收记录

### 提交与用户可见变化

- `5fecf5f` — 本文档先行定义问题、边界与验收约束。
- `8191936` — Policy 默认决策和规则保存失败归属当前 Dialog：保留已选决策和规则草稿、当前弹窗直接告错并允许原位重试；打开新编辑器时清除历史错误，页面加载/列表错误仍归属页面。新增 3 项失败/重试/取消测试。
- `9bb7730` — 规则保存错误固定在弹窗 Header，并限制过长错误的高度，以免长规则配置滚动时看不到失败原因；增加 Header 所属断言。
- `2c0a807` — Agent 创建页面以步骤序号代替重复栏目标题；低频默认模型收进可展开高级设置并跨“核对/返回”保持输入；名称空格、相对路径和仅空格目录获得字段旁文字错误和关联的无障碍状态。Review 显示与最终提交相同的修剪后名称/目录/模型，创建失败在 Review 就地显示且保留草稿。进程隔离级别、Host 风险告知、Profile 授权不自动授予等保持原有语义。

### 验证证据

- **完整前端门禁 PASS**：`pnpm lint && pnpm typecheck && pnpm test && pnpm build`，日志 `/tmp/mender-agent-policy-ux-fullgate-20261021.log`，退出码 0；**63 个测试文件、392 项测试通过**；i18n **1483/1483**，Web、Wails Desktop 前端均构建通过。保留已有 JS 大分块警告。规则 Header 小切片另跑定向 Vitest/ESLint，均通过。
- **原生 Wails + Vite HMR 实际观察**：Mac `mender-desktop` 原生 WKWebView 页面展示新进度“第 2 / 3 步”，默认模型在“高级设置”内折叠，进程隔离选择与风险说明始终直接可见；真实键盘输入临时相对目录 `relative/project` 后，字段下方显示“请输入以 / 开头的工作目录绝对路径。”，不能继续核对。已通过“返回智能体列表”退出草稿并恢复原来的“审批”页面。截图 `/tmp/mender-ux-agent-config-after-hmr.png`、`/tmp/mender-ux-agent-relative-path-error-native.png`。
- **真实持久化检查**：生产 SQLite 仅以只读模式执行 `PRAGMA quick_check`，得到 `ok`；当前 `agents=4`、`policy_rules=0`、`profiles=1`、`credentials=3`。本次没有操作任何生产创建、保存、审批、授权、委派或删除。
- **证据边界**：Policy 后端写失败通过隔离的 React/Vitest stub 覆盖，**未**在真实生产策略上主动制造失败；Wails 验证仅为真实运行窗口的草稿与本地字段校验，不代表 Agent 实际创建成功。当前 Pi 浏览器因旧 journal 格式冲突不可用，未冒充 390px Browser / 完整 VoiceOver/键盘或 WCAG AA 验收；Wails 最终发行包仍需在后续正常打包时集成本轮前端源码。

### 外部可用性依据

- WCAG 2.2 3.3.1 错误识别：<https://www.w3.org/WAI/WCAG22/Understanding/error-identification.html>。
- WCAG 2.2 3.3.3 错误修正建议：<https://www.w3.org/WAI/WCAG22/Understanding/error-suggestion.html>。
- GOV.UK Design System 错误文本与字段关联：<https://design-system.service.gov.uk/components/error-message/>。

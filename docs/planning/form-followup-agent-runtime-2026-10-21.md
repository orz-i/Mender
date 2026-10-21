# Mender 表单治理续轮：Agent 授权、长表单和 Runtime 错误恢复（2026-10-21）

## 意图与证据

接续 `form-task-first-governance-2026-10-21.md`、`form-contextual-help-density-2026-10-21.md` 和 `form-density-qa-2026-10-21.md`，针对未覆盖的表单实施**有证据的收口**，不再机械减少所有说明文字。

源码复核（在 `74dc7e9` 基线）：

- `agent-workspaces.tsx` 的 `openGrant` 默认选择第一个尚未授予权限的 Profile，打开即允许点击 Confirm；授权目标应由用户明确选定。
- `agent-approval-center.tsx` 和 `agent-tool-execution.tsx` 的长表单用 DialogContent 根滚动，底部按钮会随内容滚动；应复用共享 `DialogBody` 固定 Header/Footer。
- `runtime-page.tsx` 在 `saveBudget`/`saveIsolation` 失败时仅渲染页面级 `ErrorNotice`，打开的 Dialog 遮挡了诊断；数值表单只依赖原生 number 约束，Save 没有范围校验。需要表单内部错误、明确可见的数值范围。
- `policy-page.tsx` 的规则向导同时在 Header 和 StepIndicator 显示步骤；可保留读屏说明、去掉视觉重复。含“空匹配所有”“最严格规则优先”等权限含义的信息继续显示。

## 不变量

1. 仅修改共享 React UI、翻译和定向测试；不修改 Go、Wails 绑定、持久化、Profile/Agent/Policy 资源、Agent Loop 或授权协议。
2. Profile 授权必须显式选择，无默认选中第一个授予主体；未选择时按钮禁用；已经授予的 Profile 不再提供候选。
3. 弹窗 Header/Footer 固定，中间内容滚动，取消保留不提交；提交失败保留输入及打开状态，错误就地可读，忙碌时不允许重复操作或关闭。
4. 必须直观看到 Host/YOLO/24h/写入与执行风险，沙盒与 Profile 授权仍分离；Policy 的通配作用、默认决策、审批失败封闭和规则作用域不能收进 Infotip。
5. Runtime 数值保留真实约束（整数、最小/最大值）；非法值不提交，提示范围。隔离模式的提升 Host 风险仍需要显式勾选确认。
6. 翻译/i18n 严格一致；复用现有 `DialogBody`、`FormInfo`/原生语义控件，不新增全局模式、状态存储或兼容桥接。
7. 生产 Wails 的 Profile、Credential、Agent、Tunnel、Policy 不执行写入验证；Web Fixture 与原生 UI 分别标注，不把测试通过误写为生产实机成功；保留现有四个未跟踪工具目录不入库。

## 阶段

- **D0 文档**：记录上述四个明确风险及验收边界，独立提交。
- **D1 Agent 授权**：修复 Profile 显式选择、目标确认、弹窗局部滚动和相关测试。
- **D2 Runtime/Policy**：配额范围和失败就地恢复、隔离固定操作区、规则向导重复说明收敛，定向测试。
- **D3 验证**：lint/typecheck/i18n/Vitest/build；优先用本机受支持浏览器捕获 1280×800 + 390×844，测打开、切换、禁用、错误和底部按钮；若本地 browser journal 仍不可用，标注原因并用隔离浏览器代替，不污染生产数据。保持代码小批量提交。

## 设计依据

- WCAG 2.2 3.3.1/3.3.3：输入错误应有文本标识及可执行修改建议；没有要求统一用单一的错误摘要组件。
- GOV.UK Error Message/Validation：保留用户已填数据；字段错误在字段附近，错误摘要有可跳转目标。
- Base UI：技术性可选解释可用支持点击/触屏/读屏的 Popover；必要安全/授权说明不能只放入浮层。

参考：
https://www.w3.org/WAI/WCAG22/Understanding/error-identification
https://www.w3.org/WAI/WCAG22/Understanding/error-suggestion.html
https://design-system.service.gov.uk/components/error-message/
https://base-ui.com/react/components/tooltip

## 本轮实施与可验证交付

在 `74dc7e9` 基线上依次小批量提交，未修改 Go/API/持久化/运行时权限模型：

- `cc80639`：本设计与安全验收约束。
- `2372d9e`：Agent 附加工作区授权不再自动选中第一个 Profile；显示授权目标，显式选择后才允许确认；Agent 授权模式与受治理工具编辑采用固定 Header/Footer、独立可滚动 DialogBody。保留 Host/YOLO 风险、分离 Profile/Sandbox/Policy 门禁。
- `a9718a5`：Runtime budget 使用独立字符串草稿、整数与上下限校验，非法值禁用保存；服务端失败在当前弹窗显示并保留草稿供重试；取消恢复服务端原值。Runtime isolation 保留 Host 风险确认并修复错误回显。Policy 规则向导删除重复的视觉步骤标题，把非关键的 Principal 技术说明迁移至可访问字段帮助，保留通配匹配语义。
- `f31d89d`：Runtime 数字字段范围以紧邻标题的紧凑形式显示，读屏保留完整范围；Policy 精确主体引用改为可选展开，已有引用自动展开，收起始终显示“不限制主体”或精确匹配数量，不改变规则数据。

### 自动化质量门禁

`pnpm lint && pnpm typecheck && pnpm i18n:check && pnpm exec vitest run --maxWorkers=6 --reporter=dot && pnpm build` **退出码 0**，56 个 Vitest 文件、353 项测试通过，i18n 1467/1467，Web 与 Desktop frontend 构建通过。运行记录：`/tmp/mender-form-governance-final-20261021.log`。保留原有 Vite bundle >500 KB 提示，不属于本轮表单行为回归。

首次未指定 worker 上限的 `pnpm test` 曾出现一次无关 Docker 5 秒轮询 timer 测试失败；该条定向测试及限制并发后的全量 353 项通过。本轮没有修改 Docker 代码或假定其定时机制已根治。

### 隔离浏览器交互与截图

Mac Pi 现有浏览器 Session 报 legacy journal validation error，未清理原有日志；使用现成的 Playwright + 安装的本机 Google Chrome，不安装新依赖、不变更项目配置，针对运行中的 Vite `127.0.0.1:5174` 拦截仅所需 API 并返回隔离 Fixture。所有测试均为导航、只读、空表单和模拟失败，不写入生产 Mender 数据。

- **Runtime 1280×800**：配额弹窗高 507.5px，底部 footer 在视口内，文档无横向溢出；核心范围位于标签侧边，帮助按需可用。
- **Runtime 390×844**：弹窗 y=16..828px、固定 footer 底部 y=827px；正文 `scrollHeight=672`、`clientHeight=523`，用户可滚动全部字段，操作区独立可见；根页面不横向溢出。空值保持空、`aria-invalid=true`、Save 禁用。模拟 HTTP 503 后表单内显示错误并保留原输入值 `48`。截图：`/tmp/mender-form-followup-runtime-1280.png`、`/tmp/mender-form-followup-runtime-390.png`、`/tmp/mender-form-followup-runtime-mobile-fresh-390.png`。
- **Policy 1280×800 和 390×844**：规则 Scope/targets 弹窗固定操作区，正文独立滚动；390px `bodyScrollHeight=838` / `clientHeight=571`，footer 底部 827px，documentWidth=390。精确主体默认折叠且有可见 wildcard 提示，点击可展开并显示独立的字段级帮助（首次无帮助内容，点击后显示，Esc 可关闭），无页面脚本报错。截图：`/tmp/mender-form-followup-policy-1280.png`、`/tmp/mender-form-followup-policy-390.png`、`/tmp/mender-form-followup-policy-390-expanded.png`。
- 突然改变 viewport 后立即截图出现过一次短暂叠影；重新等待界面稳定及以 390px 新开浏览器上下文，确认唯一 Dialog、无遮挡，丢弃瞬时截图，不把它作为已验收证据。

**验收边界：** Chrome + Fixture 只证明当前 Web 源码的布局、交互和错误恢复；没有声称后端生产创建成功、真实 Agent 授权成功、完整读屏/键盘 WCAG 认证或新 Wails Desktop 原生实机验收。当前 Desktop app 二进制是在此批改动前构建；需由使用者重新构建刷新后再做原生 UI 非破坏性核对。现有 `.agents/`、`.commandcode/`、`.intent-loop/`、`.pi/` 目录未加入提交。未触碰实际 Profile、Credential、Tunnel、Agent workspace 授权、Policy 或 Runtime 生产持久化数据。

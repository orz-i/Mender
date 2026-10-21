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

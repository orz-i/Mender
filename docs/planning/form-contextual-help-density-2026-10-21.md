# Mender 表单帮助与信息密度治理（2026-10-21）

## 当前事实与目标

重建后的 Wails `apps/desktop/bin/mender-desktop.dev.app` 于 2026-10-21 17:30 重构，当前 `default` Profile 已运行 MCP `127.0.0.1:8765`。用户截图证实「启动 MCP 服务器」只有 Profile 和 Listener 两个配置，却同时展示 4 个大块解释：DialogDescription、工具权限说明、Tunnel 说明卡片、地址自动轮换说明，以及高级设置内的多行段落。这不是缺功能，而是信息出现时机错误；同一模式遍布其他创建/配置表单。

统一目标：**默认只显核心任务/字段/状态，帮助紧随字段通过单个图标按需解释，避免将说明段落与正式表单内容同级堆叠。** 不新增 Basic/Advanced 模式开关，不重复已有 Advanced 业务设置，保持功能/权限完整。

## 分层规则（不做“所有文字全塞 tooltip”）

| 层级 | UI | 示例与边界 |
| --- | --- | --- |
| 一级（必须扫读） | 标题、字段名称、输入、必要的简短状态/风险标签、提交/取消、失败原因 | MCP Profile 若绑定远程 Tunnel，明确标识「启动时同时连接隧道」；Sandbox SSH 未隔离、root、高费用、授权范围必须显露；不隐藏校验错误、链接、不可用原因 |
| 二级（可选择了解） | `FormInfo` 圆形 Info 图标 + Hover/Focus/Click/Touch 的 Popover，位置紧贴对应字段或标题 | 何谓 Profile、端口自动轮换和 OS 随机端口、OAuth Client ID、凭据存储实现、Namespace/slug 规则、外部 Tunnel ID 来源 |
| 三级（专家设置） | 原有「高级设置」展开（不是新开关） | TCP 监听地址、MCP stdio 使用方式、Credential Namespace/Name、Provider Header 等，原可编辑项一项不删除 |

### 可访问性约束

- Base UI 官方明确区分 Tooltip 与 Infotip：**纯 Tooltip 在触屏上不可用**。Info 图标是独立操作目标，使用项目已有 Base UI Popover，Trigger `openOnHover`，并支持点击、键盘焦点/Enter/Space/Escape，拥有可理解 `aria-label`。内容不包含必须阅读的安全语义，不渲染敏感数据。
- 保留真实表单控件的 `aria-label` / label 关系，**不将 button 嵌入 HTML `<label>`**；在必要时局部使用分离的标题+按钮容器；不要以 placeholder 替代 label。
- 对支持屏幕阅读器的 Dialog Description 可保留 `sr-only`，但不让技术说明占据视觉首屏。重要、与提交紧密相关的动作效果通过简洁 inline 状态表达。
- 不要对列表状态、错误、会话/任务不可用、root 风险、SSH 主机身份核验、权限门槛、审批或费用做“只 hover 才能看到”。
- 品牌视觉仍复用 `@mender/ui` 及现有 token；不要新造 Alert 样式和二级帮助弹窗状态存储。

## 实施切片

1. 文档（本提交）/ Wails 实机截图基线。
2. 共享 `FormInfo` 组件+三种交互（hover/focus/tap）/移动宽度单元和浏览器验证；领域表单结构只用已有 Popover。
3. 优先表单治理：Server（当前截图）、Tunnel、Credential（Native/OAuth）、Profile Create/Tool Governance、Provider Connect/Create、Agent Wizard。对 Sandbox 删去纯工作流解释但保留风险与费用、确认/审查。
4. 以组件/领域用例/完整 `pnpm check` 检查；真实 Wails 在重构后只读导航与取消草稿、真实监听器及生产 Profile/Tunnel 不动。列出因工具或运行时限制尚未视觉验收的区域。

## 参考

- Base UI Tooltip 组件官方建议：Infotip 使用 Popover Trigger 的 `openOnHover`，而非 Tooltip，因为触屏和屏幕阅读器可访问性差异。
- W3C WAI Form Instructions：保留可理解的可访问 label、`aria-describedby`、错误引导，placeholder 不能充当标签。
- DWP / GOV.UK Design System 2026：提示应短而必要；重要信息不可藏于 hint/tooltip。
- `docs/design/mender-visual-identity.md`：一个视口主要只有一个信息类别。

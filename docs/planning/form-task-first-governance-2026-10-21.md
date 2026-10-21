# Mender 表单任务优先治理 — 2026-10-21

## 目标与来源

继 2026-10-12 弹窗视口治理、2026-10-19 UI/UX 简化之后，从**普通用户完成任务**而非内部资源模型出发，收口关键“创建 / 配置 / 验证 / 保存”表单。此前治理已覆盖对话框高度、固定底部按钮和部分列表密度，本轮不重复造轮子。

外部依据：W3C WCAG 2.2 3.3.1/3.3.2/3.3.3 要求可识别输入错误、明确字段说明和已知修正建议；GOV.UK Design System 的 Error Message 指南要求错误靠近相关字段且提交失败保留已有输入。
- https://www.w3.org/WAI/WCAG22/Understanding/input-assistance
- https://design-system.service.gov.uk/components/error-message/

## 不变量

1. Mender 仍是工具/资源聚合、治理和供应平台。**不改 API / 持久化 / Agent Loop / ACP / Provider、Profile、Credential、Tunnel、Sandbox 身份和授权语义**。
2. 普通用户的默认界面只需回答当前任务的问题；高级技术字段可按需展开，但不能隐藏必要授权、风险、费用或失败信息，也不能凭空新增“简单模式”等持久化产品概念。
3. 所有后端必填参数在提交前必须可见、可输入，或由**可见且明确告知**的、确定性安全默认值推导。不得虚构 OAuth/Tunnel 外部 ID、凭据或悄悄选择首个安全主体；用户仍能编辑高级字段。
4. 密钥值不在列表、日志、通知或错误详情中回显；错误诊断只显示安全可公开的字段与步骤。OAuth/SSH 的密钥校验、host-key/指纹确认、root 风险、Docker 隔离以及费用/授权审查不得降级。
5. 表单失败保留用户输入和当前步骤，明确就地显示原因和补救；取消/退出不提交草稿。无暗中添加工具绑定、提升授权、创建默认 Credential 或更改现有资源。
6. 继续复用共享 React/TypeScript strict、i18n、DialogBody、Field、Button、已有选择器与现有测试。页面首屏聚焦单任务，弹窗 header/footer 可见，正文限高；390px 与桌面宽度必须可达。无额外状态表、兼容层或新后端。
7. 本次使用 Agent 的执行为**真实代码改动与测试**，工具返回 TaskID 后只追踪原任务；即使 unknown / 请求超时，也不得不查状态就盲目二次委派。以仓库 diff、测试及运行截图独立验收其交付；不得擅自加入 .agents/.pi/.commandcode/.intent-loop 等非项目业务目录。

## 已确认的基线风险（代码证据，运行态待逐项核对）

| 区域 | 现状 | 本轮收口目标 | 责任范围 |
| --- | --- | --- | --- |
| 凭据创建 | 普通 Native/Env/OAuth 同时暴露 Namespace、Name、Display Name 和 Reference；OAuth 高级参数平铺 | 可理解的名称 + 类型相关核心输入；技术标识可展开，自动生成需满足真实标识约束且避免不透明碰撞；保留完整 OAuth 与 SSH 能力 | AGY：credentials-page、credential-form 与定向测试 |
| 隧道创建 | 默认选择 credentials[0]，技术名称 External Tunnel ID/Runtime API-key Credential 无上下文；创建失败存在页面级错误 | 明确外部 ID 获取方式、**显式选择**运行密钥、无凭据时可到凭据页、失败靠近表单并保留已填内容 | 协调者：tunnels-page + 定向测试 |
| MCP Server 启动 | Profile + Listen Address 与底层 HTTP/stdio 说明同时暴露 | Profile 为核心，明确将使用的本地监听地址，必要时展开可编辑网络配置；保持只允许 loopback | 协调者：servers-page + 定向测试 |
| Sandbox 向导 | 后端、配置、资源限制、审查各步骤均包含技术信息；已保留 SSH 主机身份确认及授权风险 | 仅对资源限制做按需展开/默认建议；清楚呈现隔离和费用，不压缩 SSH/Review 安全门禁 | OpenCode：sandbox-create-page + 定向测试 |
| 工具来源/Provider | Remote MCP 集成已把 Namespace 收入高级；通用 Provider schema 仍按插件动态生成 | 优先复用已有 schema 与默认值，审查是否有阻塞普通用户的重复字段；禁止凭空替用户填认证 | 后续核验，不抢占上述关键路径 |
| 配置档/工具选择 | 已来源优先及分页，拥有权限与动态/直连细分 | 当前阶段回归保存/取消与身份准确，不改变 Profile ToolBinding 语义 | 集成回归 |

## 实施分段

- **D0 文档与证据**：记录现状、责任边界和验收矩阵，文档先提交；不把曾经的 fixture 截图当成当前真实数据。
- **D1 核心表单**：AGY 负责凭据，协调者负责隧道和服务器，OpenCode 负责 Sandbox 资源限制；域文件写入互不重叠。Agent 不自作主张提交，协调者检查并分域提交。
- **D2 集成**：复核 i18n、React/TypeScript、对话框、授予/认证边界。运行 targeted Vitest + app typecheck + eslint/i18n/check + Web/desktop builds（视耗时），不在真实 Profile/Credential/Tunnel/Sandbox 上执行不可逆写入。
- **D3 实际渲染验收**：真实 Wails 已运行，普通 Web Dev :5174 可能因 API 代理不匹配显示错误；优先原生只读导航/弹窗，Web 用可访问的隔离 QA API/测试数据。检查桌面与 390px、保存按钮、校验、折叠展开、默认值以及焦点；把无法覆盖的项目清晰列为未验证。

## 验收门槛

- 用户能只填**任务真正必需**的信息完成创建，不需要理解 Namespace、ToolID 或 Schema；所有实际必需参数和专家设置可找到。
- 切换类型不残留旧密钥/凭据/权限；明确选择 Credential，不使用错误的首条默认值。
- 表单校验错误有文本提示并关联输入，提交失败保留值且在当前弹窗/步骤可见，按钮忙碌状态防重复提交。
- SSH host-key/指纹、remote root、费用、isolated vs non-isolated 明确，审查和用户显式授权继续执行。
- 1280x800、1440x900 和 390x844 无阻挡、横向溢出和不可见表单操作；模拟器、真实 Wails、测试 fixture 证据分别标注。
- 最终 git diff、commit、测试日志和 Agent TaskID 一一可追溯，未知任务**不自动重放**。

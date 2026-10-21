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

## 2026-10-21 实施、Agent 归因与隔离验收

### 交付提交与改动

- `2a8cf0f` — 文档先行、表单字段和授权不变量、验收范围。
- `bbc939a` — 四类关键表单及定向测试，11 个共享 React/i18n 文件，保留 Web/Wails 同源实现，不改 Go、持久化/API、Agent 生命周期或凭据值存储方式。
- **凭据**：新增默认 `personal` 命名空间、由用户可见名称推导内部 Name；保留 Namespace/Name/Reference 技术设置，冲突提示展开技术标识但不丢密钥；OAuth 可选 client ID/scopes 渐进展示，重新授权时显露已有参数；跨来源切换清空敏感输入；SSH 固定 namespace、安全校验和 keychain 语义不变。非拉丁显示名如果不能形成有效内部标识，会提示用户手动填写拉丁标识，不偷偷生成不透明 ID。
- **隧道**：任何入口（包括 Marketplace deep-link）均不自动挑选第一条 Credential；必须明确选择。外部 Tunnel ID 解释来源，缺凭据时提供导航；错误在当前弹窗出现并保留输入，未知第三方诊断不回显。
- **服务器**：多 Profile 时必须显式选定；一个 Profile 时允许按现有唯一对象默认。监听地址始终以摘要可见，低频端口配置留在高级设置，仍仅通过后端既有 loopback 安全门。启动失败留在弹窗，不擦除地址/选择。
- **Sandbox**：Docker/Daytona 资源步骤显示现有默认分配概要，所有 UID/GID、CPU、内存、磁盘/PID、TTL 等字段可完整展开，Review 和创建仍传原值。隔离警告、远端费用告知、SSH fingerprint/root/credential 和明确 Review 不变。

### 并行 Agent 真正交付核算

- **Antigravity** `atask_a726bcf2ddb2909ba99ed02cdf4bcbe5`：Mender Task 最终 `completed`，`result_text` 长度为 0，交付时仓库对应 Credential 文件没有任何修改；**不能记作已交付代码**。这并不证明所有会话续接可恢复（原生恢复能力未验证）。
- **OpenCode** `atask_61edd839a48cf66f47961e24bef73535`：Mender Task 最终 `failed`，固定诊断 `Agent permission request was rejected`，一次 `read` 审批记录 `expired`，无 Sandbox 文件改动。此前委派时使用的是 `opencode/minimax-m3.1-flash-free`，不能把已观察到的**审批失败**错误归因为模型不可用。
- 经使用者确认，**此后所有新 OpenCode 任务只考虑当前可用的 `free` 模型**：首选 Mender 当前配置的 `opencode/deepseek-v4.1-flash-free`；只有首选不可用时才考虑 GLM 5.3 Flash 对应的实际 `free` 模型 ID。禁止猜测模型 ID、擅自修改持久化默认、自动扩大审批权限或重派已有 Task Prompt。本次失败 Task 未复放，Sandbox 由协调者直接完成。
- 全程未修改已有生产 Agent grants、Credential、Profile ToolBinding、Tunnel 或 Sandbox 数据；未把 `.agents/`、`.pi/`、`.commandcode/`、`.intent-loop/` 纳入提交。

### 实际检查与证据边界

- **代码质量门禁：通过**。执行 `pnpm lint && pnpm typecheck && pnpm test && pnpm build`，退出 0；Vitest **53 个文件 / 329 项通过**；`i18n:check` **1450/1450**；Web 和 Desktop frontend builds 成功。保留原有 Vite 大分块提示，没有新建后端服务或运行时协议。
- **隔离 Web UI 1280×800、390×844**：使用临时 QA Vite `:5193`，API 仅指向已有的隔离 `menderd :18080`；Credential 表单读取真实隔离 API 的空清单，桌面/390 首屏和 390 高级展开都已截图与检查。390px 的展开态内层可滚动（`scrollHeight 661/clientHeight 549`），Dialog 高度 812px，底部按钮位置 827px，小于 844px 视口；根文档无水平溢出。
- **Tunnel/Server**：隔离后端没有完整的 Tunnel/Plugin 管理端点，也没有可用 Profile，因此仅对 `GET /api/v1/tunnels`、`GET /api/v1/plugins`、`GET /api/v1/profiles` 使用明确测试 Fixture，不写数据库；分别核对了空 Credential 按钮禁用/创建入口，以及双 Profile 时没有隐式选择、选中后的高级地址可编辑。Fixture **只证明渲染和本地交互，不等于真实 Tunnel/Server 执行验收**。
- **安全边界**：运行态全部为浏览、展开、切换或只读检查；未在生产或隔离后端提交创建、删除、授权或启动请求。临时 QA Vite 已停止、临时仓库配置已删除；截图保留在本机 `/tmp/mender-forms-qa-*.png` 并导出到当前对话。
- **尚未验证**：重建后的原生 Wails 实际全表单交互、Sandbox Docker/Daytona 视觉状态和真实创建、Tunnel/Auth 的后端成功/失败端到端、读屏/键盘全部路径与高对比度；不把 Web Fixture 或代码测试冒充 Wails 原生验收。

### 下一次非破坏性验收

待使用者重建/刷新 Desktop 后，在**不更改生产存量数据**情况下，只读打开四类新表单，检查原生高度/滚动/默认值/标签及授权风险告知；涉及真实创建与失败恢复，须使用独立测试资源且提前明确授权。

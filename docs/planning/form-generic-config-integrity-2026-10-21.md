# Mender 动态 Schema/JSON 表单草稿一致性治理（2026-10-21）

## 范围与证据

承接前几轮表单治理，不扩展 Provider / Sandbox 的持久化模型或插件协议。本轮审计基线 `5a0ded1`。

现有共享 `GenericProviderConfigForm` 用于 **Provider 创建/编辑、Provider Rebind 和外部 Sandbox 创建**。其 `JSONArrayEditor` / `JSONValueEditor` / `RawObjectEditor` / `AdditionalPropertiesEditor` 自持 `text` 草稿，只在 JSON 成功解析时调用 `onChange` 更新父级配置。失败时仅展示自身错误，父级 `genericConfigValidationError(schema, value)` 仍只验证上一个合法 `value`：潜在结果是用户界面显示不合法未保存的草稿，但父级提交或 Review 仍可点击，可能提交旧配置。

同时，局部 `text` 初始化自 props 而没有可靠的“外部值变化”同步：Provider Rebind 换目标安装 / 切换同一个 schema 的不同资源、Sandbox 插件重载、同层其它字段导致更新时都需检验是否保持或重置正确草稿。不能把用户正输入的有效 JSON 在每次父级 re-render 时重格式化、清空光标或覆盖输入。

复核的具体文件：
- `packages/app/src/domains/providers/generic-provider-config-form.tsx`
- `packages/app/src/domains/providers/providers-page.tsx` 中 Provider 创建/编辑及 Rebind 的 submit 门禁
- `packages/app/src/domains/sandboxes/sandbox-external-create-page.tsx` 的 configure → review → create 路径
- `packages/app/src/domains/providers/generic-provider-config-schema.ts` 现有默认值和 Schema 真正支持的能力

## 实施边界

1. **非法本地 JSON 草稿绝不能被父级保存或提交为上一个合法值**：提供仅供表单使用的局部草稿有效性通知，并组合原有 Schema 校验；创建、编辑、Rebind、Sandbox Review / create 均要阻止。
2. 显式语义：必需 JSON 值只按既有 Schema 校验，不因新 UI 假设而自行改变后端字段含义；可选字段清空按已有语义处理；`false` / `null` / 标量的 Schema 规则仍由后端契约决定。
3. 用户输入非法/部分 JSON 时保留原文字、错误紧邻编辑器，标记 `aria-invalid` 和关联的文本错误；仅当真正获得新的外部权威 `value` 或切换资源时才复位局部草稿。正常输入等价的有效 JSON 后，不应因反向 props 同步跳光标/重新排版。
4. Provider 重绑 target / create plugin 切换仍使用现有显式 `key` 与基于安装的配置选择规则；旧的错误不影响新目标。相同值的父级重渲染不应擦掉当前非法草稿。
5. Provider 保存失败保持已有输入及错误当前表单可见；不回显 Secret、未改动 CredentialRef 授权逻辑；真实提交使用的 config 对象与屏幕显示的合法草稿一致，不能悄悄回放旧数据。
6. **不引入**新后端、兼容桥接、全局状态/草稿数据库、两套表单、复杂的自动修复。复用 React state/hooks、已有表单通用组件、翻译和测试。
7. 本轮禁用生产端 CREATE/UPDATE/REBIND/DEPLOY/GRANT 等写操作，只读与取消检查；绝不以生产凭据、Provider 或 Sandbox 做实验性失败测试。Wails 原生、隔离 Web fixture、单元测试证据分别标识。

## 验收矩阵与顺序

- D0：文档与最小可复现测试先行，确认问题再做修复。
- D1：共享 JSON 编辑器同步及草稿校验状态，分别测试 array、any JSON、raw object、additional properties；有效等价输入、无效文本、父级 value 重置、切换 key、无效→合法恢复。
- D2：Provider 创建/编辑、Rebind 的 Save disabled + handler guard；Sandbox 外部插件 Review/create 全流程门禁；本地失败就地可见。
- D3：lint/typecheck/i18n/定向 Vitest/全量 Vitest/build；在无需安装外部插件、无需提交业务数据条件下，对隔离 Web 1280×800、390×844 测试编辑器错误与按钮；Wails 仅限当前可达的只读/取消表单截图。
- D4：分段提交和文档收口；记录不能通过真实环境验证的路径。保留既存未跟踪的四个 Agent 工具目录。

参考：https://react.dev/reference/react-dom/components/textarea （受控输入值与本地草稿同步）；https://design-system.service.gov.uk/components/error-message/ （就地错误、保留用户输入）。

## 实施记录、回归与验收结论

### 提交

- `94462c9`：设计与安全不变量先行。
- `66fa4ca`：共享 JSON 编辑器用字段路径区分无效草稿；聚合多字段的解析错误并向外通知，标记 `aria-invalid`、`aria-describedby`、对应错误文本。输入合法 JSON 但从父组件回显同一规范化值时，保留用户手工格式；真正有不同权威配置传入时刷新文本并清除过期错误。
- `156b7a2`：Provider Create/Edit/Rebind 和 External Sandbox Review/create 接入局部草稿有效性门禁；再次保护 submit handler，不依赖按钮外观。Provider 编辑/重绑失败原因改为当前弹窗内可见，Rebind 长表单固定 Header/Footer 与独立滚动区域。
- `dba83f3`：修复可选 JSON 数组清空后被错误变回 `[]` 的状态同步；Sandbox 切换外部插件时重置子表单 identity；Provider 的 Installation ID/Package Version 收入默认关闭的“安装详情”，其它长技术标识可换行、窄屏自动单列，不删原能力。

### 回归复现

在 D1 代码变更前，新加的共享 JSON 草稿 4 项测试 **4/4 失败**，证明旧实现未标记非法草稿、未合并多字段错误、未同步外部新值及非对象原始 JSON 校验绕过。修复后含以下覆盖通过：

- 原始任意 JSON 对象、数组、任意 JSON 值、附加键值编辑器：无效 JSON/错误类型，父级保存禁用，已填文本保留，恢复合法后可提交。
- 同时存在两个非法 JSON 字段时，不因修复其中一个而清除另一个错误。
- 编辑器合法 JSON 不被父级回显重排；无效 JSON 中途修改同级字段仍保留草稿；真正加载不同配置时恢复权威值并重置错误。
- 可选数组“未设置”与显式 `[]` 区别保持；切换 Rebind 目标安装将旧局部无效草稿清除而保留已确认的外部配置。
- Provider 创建、编辑、Rebind 不提交旧配置；编辑/重绑服务端失败错误留在弹窗里，当前有效配置不丢失。
- 外部 Sandbox 非法 JSON 时不能进入 Review；修复后 Review 可达，创建调用仍需明确用户提交。没有变更原 Schema/API/CredentialRef/授权行为。

### 隔离浏览器渲染验收

2026-10-21 使用本机现成 Google Chrome + Playwright，浏览器请求拦截 `http://127.0.0.1:5174/api/v1/**` 返回明确 Fixture。生产 Provider、Credential、Sandbox 库没有写入。

`/tmp/mender-generic-config-visual-qa.mjs` 运行两种视口：**1280×800** 与 **390×844**；动态 Schema 表单显示真实 React 组件和边界，未对生产 API 发出任何请求。

- 初始配置填写完成后可保存；输入非法数组 `[1,` 时原样留在文本框，`aria-invalid=true`、字段错误出现、Provider 创建按钮禁用。
- 输入合法 `[1,2]` 后恢复提交能力；模拟 POST 被浏览器网络拦截并返回 HTTP 503，当前弹窗显示安全失败提示、JSON 草稿不丢失，未向后端实际写入。
- 1280px 弹窗受限在视口内（宽 576px，底部 784px）；390px 弹窗宽 358px、底部 828px，固定 Footer 底部 y=827px，根文档宽 390px，无横向溢出。
- 截图发现此前技术 ID/包版本在 390px 两列挤压，后按合同折叠为“安装详情”；复测默认收起后没有重叠，仍可展开查看完整版本与安装标识。
- 图片 `/tmp/mender-generic-config-1280-invalid.png`、`/tmp/mender-generic-config-390-invalid.png`、`/tmp/mender-generic-config-390-failed-submit.png`（均为 Mac 上浏览器 Fixture 截图，不冒充 Wails 原生）。

### 完整质量门禁与生产数据

`pnpm lint && pnpm typecheck && pnpm i18n:check && pnpm exec vitest run --maxWorkers=6 --reporter=dot && pnpm build` **全部退出 0**；59 测试文件、**369/369** 测试通过，**1471/1471** 中文翻译覆盖，Web 与 Wails frontend 产物构建通过。日志：`/tmp/mender-config-draft-final-gate-20261021.log`。Vite 既有大 bundle 体积提示仍非阻断。

前后对生产 `$HOME/Library/Application Support/Mender/mender.db` 仅 `mode=ro` 核对：providers=12，credentials=3，sandboxes=7，agent_tasks=75，`quick_check=ok`。没有创建/重绑/更新任何真实 Provider/Sandbox/Credential，没有 Agent 委派、授权或清理业务数据；原 `.agents/`、`.commandcode/`、`.intent-loop/`、`.pi/` 均未加入提交。

### 明确的验收边界

- 动态外部插件需要适当的已安装测试资源才能原生可交互验证；当前本轮没有在生产安装或配置测试插件。**Wails 原生 JSON 编辑表单和真实后端写入失败路径本次未实测**。隔离浏览器只证实渲染与在 Fixture 中的交互。
- 实际项目下次正常重新构建/刷新 Wails 后，可对已有通用插件进行只读、无保存的 JSON 字段检查；如需端到端写入测试，应使用独立沙盒与明确批准的测试 Provider，而非生产配置。
- 完整 VoiceOver、400% Zoom、外部动态 Schema 多层嵌套并未宣称全面覆盖；本轮测试针对有证据的 JSON 草稿一致性和响应式问题收口。

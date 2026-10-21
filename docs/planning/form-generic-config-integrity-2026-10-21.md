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

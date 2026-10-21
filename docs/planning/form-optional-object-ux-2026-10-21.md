# Mender 表单治理：可选嵌套配置组的显式启用与移除（2026-10-21）

## 目标、证据与非目标

本轮延续 `form-schema-field-validation-2026-10-21.md` 已交付的动态 Schema 字段治理。初始 HEAD 为 `f68e4ca`，共享 `GenericProviderConfigForm` 被 Provider Create/Edit/Rebind 与 External Sandbox Create 复用。

源码证据：`SchemaField(type === "object")` 对可选且 `value === undefined` 的对象仍会渲染整个 `ObjectFields` 树。子字段填写后，`onChange(nextValue)` 将整组对象写入父级；清空最后一个子字段仅得到 `{}`，不能回到 **未配置（key 不存在）**。因此可选组无法被显式移除，用户看见一屏还未启用的高级字段，在编辑/Rebind 时也无法用普通表单移除旧配置。根本问题是“未设置”与“已配置为空对象”不能区分。此问题不应靠字符串魔法或后端数据迁移补救。

通用表单可访问性参考 W3C WAI `Grouping Controls`：相关字段保持 `fieldset`/`legend` 语义；没有启用的可选组不应呈现为一堆可输入但非必填的字段。https://www.w3.org/WAI/tutorials/forms/grouping/

## 不变量

1. 对 **可选对象字段且 value 为 undefined**，仅显示短预览（名称、可选状态、说明与“添加…”动作）；不存在时不在 DOM 中渲染子控件。用户主动添加后才建立新对象。
2. 对 **已存在的可选对象**，显示原有完整字段、可见的“移除…”动作。移除必须删除父级 key（`onChange(undefined)`），不能转换为空对象 `{}`，并且不能触发任何即时后端保存。
3. 对 **required 对象**，始终显示原有字段及验证，绝不提供“移除”动作；不得放宽 Schema 必填、URI、数值、Credential Reference 或权限校验。
4. 添加时只复用 `genericConfigDefaults(schema)` 建立符合插件已声明默认值的新对象。不从旧已移除草稿恢复，也不自造字段或兼容桥接。没有默认值但包含必填孩子的组，在添加后立即按原必填约束提示。
5. 移除含有非法子 JSON 草稿的可选组必须同步释放草稿错误，恢复外层 Submit/Review 的正常门禁；父级其他字段的错误不可被误清除。再次添加得到全新初值。
6. 递归任意现有支持深度均使用同一个组件/状态模型；未启用的嵌套组进一步收敛首屏信息密度，展开后的分组语义在键盘/读屏可辨识。
7. 不能对真实生产 Provider、Sandbox、Credential、授权、Agent Task 做写入测试。Wails 原生可做只读页面检查，端到端动态对象交互通过隔离 fixture 及 Vitest；安全边界必须如实记录。
8. 文档先行、分批提交、测试（TypeScript strict、i18n、ESLint、Vitest、Web/Wails 构建）和 1280px/390px 实测；不触碰 Go、API、ACP、持久化和全局 UI 模式。

## 交付阶段

- F0：设计合同提交、编写并运行基线失败测试，包括：未配置组默认不展示子控件、添加后默认值、移除 key、嵌套启用、含错误 JSON 组移除后的提交恢复、required 组不可移除。
- F1：只在共享动态 Schema object field 分支实现 Add/Remove 控件和可访问分组，复用既有验证和 `genericConfigDefaults`；保留其他字段类型不变。
- F2：Provider/Sandbox 集成回归验证最终提交配置不携带被移除的 key；新功能与既有 Schema 规则保持一致。
- F3：质量门禁及 1280×800、390×844 的无写入浏览器 fixture 验收；原生环境仅做只读/取消。
- F4：文档收口、单独提交及明确真实插件端到端验收的遗留边界。

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

## 实施与验收记录（2026-10-21）

### 基线证据与提交

旧实现对应的可选对象生命周期测试 5 项中 4 项失败，记录在 /tmp/mender-optional-object-baseline-20261021.log。视觉 Fixture 揭示另一个问题：可选对象即使没有自己的默认值，也会因为内部子字段声明默认值而被自动创建；该场景在 /tmp/mender-optional-defaults-baseline-20261021.log 单独复现了失败。

分段提交：
- da3d307：设计及安全边界先行。
- 56be2d3：不存在的可选对象只显示精简预览与添加动作；已存在时用 fieldset/legend 分组，允许显式移除整个 key；required 对象不提供移除。
- a3442d5：Provider Create/Edit 与 External Sandbox Review/Create 集成测试，核验移除后真实待提交的 config 对象不携带该 key。
- 3f88ef7：纠正 Schema 默认值传播；只有对象自己显式定义 default、为 required 或用户明确添加时才建立该组。添加时仍应用其子字段合法默认值。移动端短操作按钮保留完整 aria-label，说明独占下一行，不把长标签挤压成窄列。

### 质量门禁与响应式验收

pnpm lint、pnpm typecheck、pnpm i18n:check、pnpm exec vitest run --maxWorkers=6 --reporter=dot 以及 pnpm build 均通过（/tmp/mender-optional-object-final-gate-20261021.log），61 个测试文件、385/385 测试通过，1480/1480 条中文翻译覆盖。Web 与 Desktop 前端构建均成功，仅保留历史上的非阻断大分块提示。

隔离 QA 脚本 /tmp/mender-optional-object-visual-qa-20261021.mjs；验收结果 /tmp/mender-optional-object-qa-result-20261021.json。本机 Chrome/Playwright 使用拦截全部本地 API 请求的 Fixture，不安装测试插件、不调用真实后端写入。1280×800 与 390×844 均验证：默认未配置时子字段不渲染、Add 后才按默认值初始化、缺必填时禁用 Save、JSON 子草稿非法时阻断、Remove 后清除错误及整个 key、再次 Add 不恢复旧 Token、嵌套 Preferences 独立添加/移除；均无 pageerror 或写入请求、无横向溢出。初次自动化读取按钮早于 React 状态稳定曾出现瞬态，等待后重测全部通过。390px 弹窗宽 358px，底部 813px，Footer 底部 812px；文档宽 390px。

截图保存于 Mac：
- /tmp/mender-optional-390-default.png
- /tmp/mender-optional-390-added.png
- /tmp/mender-optional-390-invalid.png
- /tmp/mender-optional-390-removed.png
- /tmp/mender-optional-1280-added.png

### Wails 与生产安全验收边界

Wails Dev 进程 PID 83099，原二进制构建于 2026-10-21 22:20:50。macOS 原生辅助功能验证「工具」只读页面正常，随后恢复用户原先的「服务器」页面。没有安装测试动态插件或改动生产配置。因此动态可选 Schema 的真实 Wails 保存/失败处理未进行端到端验收，Chrome Fixture 的 PASS 不能替代。

对真实 SQLite 数据库只读 mode=ro 核查前后维持 providers=12、credentials=3、sandboxes=7、agent_tasks=75、agent_workspaces=0、policy_rules=0，PRAGMA quick_check=ok。没有生产资源配置、授权、任务或文件写入，四个已存在的未跟踪工具目录仍未入库。

后续在有合适非生产插件安装时，可专项验收深层 uiSchema、原生动态表单及真实文件系统写入。此轮不新增 Schema 解释器或历史兼容桥。


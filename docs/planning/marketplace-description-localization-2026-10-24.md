# Mender 第三轮 UI/UX：受控来源文案本地化（2026-10-24）

## 审计证据与用户目标

延续前两轮原则：普通用户首先看到能理解的功能，而非混杂的内部英文句子；所有 API 原始内容、权限、插件生命周期仍完整保留。

- `packages/app/src/domains/plugins/plugins-page.tsx` 的卡片和详情直接展示 `plugin.description`，对应宿主 `PluginCatalogueEntry`、受审查 `IntegrationOffering`。两类数据中存在宿主固定的英文说明，也存在本地安装第三方插件的原始说明。
- `packages/app/src/domains/providers/providers-page.tsx` 的卡片直接展示 `provider.description`；宿主为 Agent / Workspace / SSH Durable Jobs 自动注册的三类受管来源生成了固定格式的英文说明。用户自行创建的 Provider 描述不是宿主文案。
- 已有 `getLocale/setLocale` 会切换 `document.documentElement.lang`，并有完整的 `zh-CN` 字典与校验，不增加国际化框架。
- 上轮 2026-10-24 的原生界面复核确认该问题；当前第三轮开始时进程仍在，但 macOS 当前报告 Wails 0 个可访问窗口，因此不会使用旧截图冒充本轮实机视觉证据。

参考：W3C [WCAG 2.2 的语言部分](https://www.w3.org/WAI/WCAG22/Understanding/language-of-parts.html) 和 [HTML language declarations](https://www.w3.org/International/questions/qa-html-language-declarations)；专有名词可保持原样，已知宿主文案应使用现有语言环境，无法确认原语种的外部描述不应凭猜测加 `lang` 或翻译。

## 实施契约

1. **严格受控的宿主文本**：只对准确的宿主 `sourceKind=builtin` 且精确 `key` 匹配的插件描述，及受审查的特定 `integration` 的原版固定描述做中文化。必须同时匹配原文；若后端文案变更，回落原文，防止显示过时的含义。
2. **受管工具来源**：仅当 `toolSourceKind`、固定宿主 `pluginId`、来源名称与后端生成的原文都匹配时，转译三类受管来源描述。名称保留原值，原始 `provider.description` 不被重写，也不用于推断工具可执行状态。
3. **第三方与用户文案**：本地插件与用户创建的 Provider 描述原样显示；即便英文与宿主固定说明相同，也不以文本相似度作为来源判断。
4. **一致入口**：插件市场卡片与详情相同译文；工具目录保持现有 0 工具/有工具入口。搜索同时匹配原文和译文，使中英文检索均可找到相同来源。
5. **无协议变更**：不添加新后端字段、虚拟 Provider、内容语言猜测器、持久化模式、设置开关或翻译服务。不自动连接、同步、安装、删除任何资源。
6. **状态不变量**：现有启用、不可用、安装、Profile 授权及访问控制继续由后端判定；仅更改展示文本与页面查询命中。
7. **可访问性**：页面 `html.lang` 继续沿用既有 locale；受控翻译与当前语言一致。由于第三方内容没有可信语言字段，不宣称其满足完整的 WCAG 3.1.2。

## 验收

- 中英双语切换：宿主内置/审查集成描述翻译，用户插件和外部 Provider 保留原文。
- 原文不匹配、ID 不匹配与伪装为宿主的本地来源：不可误译。
- Agent、Sandbox Workspace、SSH Jobs 模板：中文可读，原始名称不变；自定义描述与第三方来源不被重写。
- 卡片与详情一致，中文/英文搜索均可找到目标；业务操作按钮和权限不变。
- Vitest 相关和全量、ESLint、TypeScript、i18n、Web build；尽可能桌面窗口验收；若窗口未恢复如实记录。

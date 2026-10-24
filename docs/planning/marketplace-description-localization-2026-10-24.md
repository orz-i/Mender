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

## 实施与验收记录（2026-10-24）

### 分段提交

- `7e99cb2`：文档先行，定义来源与原文精确匹配的本地化边界。
- `c4ce5a9`：插件市场、已审查集成与三类受管工具来源展示翻译，以及原文/译文搜索、页面切换和专用回归测试。

### 实机验证——原生 Wails DEV

运行实例：Mac mini 的 `mender-desktop.dev.app`，真实 Wails 桌面服务、`127.0.0.1:9245` Vite HMR，中文界面，真实工具来源和插件目录。原生窗口约 1239×851；原生窄屏验收到应用允许的 960×744 后已恢复 1239×851 / (340,85)。只进行访问与语言切换，没有创建、启动、同步、停止、删除任何资源。

第三轮开始时 macOS 的 AX 曾暂时报告零窗口，故最初几次整个桌面的 `screencapture` 拍到了无关窗口，**已否决，不作为 UI 证据**。后来窗口恢复，使用精确 Wails Window ID 的 `screencapture -x -l` 留下可辨认、无无关应用遮挡的截图，均保存在机器的 `/tmp/mender-ux-20261024-r3/`，未提交仓库。

| 步骤 | 实机行为与结果 | 已核对截图 |
| --- | --- | --- |
| 1. 插件市场（中文） | 直接查看内置 Agent / Docker / MCP / Model Tools / OpenAPI / SSH / Tunnel 的中文描述；AnySearch 受审查集成也显示中文描述；状态和入口不变。 | `11-marketplace-window.png` |
| 2. 插件详情 | MCP 卡片进入详情后保持相同中文描述；点击语言切换为 English，完整恢复 `Model Context Protocol provider`，切回中文恢复译文。没有调用后端配置变更。 | `12-mcp-detail-window.png`；语言切换经 AX DOM 明确验证 |
| 3. 工具目录 | Agent「Antigravity」「OpenCode」受管委派、沙盒「Rocky」工作区、SSH 持久任务等描述均为中文，0 工具来源仍提示未发现工具。 | `10-tools-window.png` |
| 4. 960px 桌面窄窗口 | 插件卡片排列、中文长句折行和菜单抽屉未出现明显裁切、重叠或横向溢出。 | `13-marketplace-narrow-960.png` |

### 自动回归

- Vitest **70 个测试文件、460 项通过**；新增对精确受控来源、13 条宿主/审查文案、三类受管来源、伪装来源、原文变更回退、中文/英文页面切换及双语搜索的验证。
- `pnpm i18n:check`：**1593/1593** 中文覆盖；`pnpm lint` / `pnpm --filter @mender/app typecheck` / `pnpm --filter @mender/web build` / `git diff --check` 均通过。
- 保留既有测试中的 Agent Router 日志警告、Audit fixture 重复 key 警告，Web build 大 chunk 警告；本轮无测试失败。

### 不变量与未覆盖

- 没有修改原有后端插件描述、数据库、服务 API、Catalog/Provider 状态、权限、Profile、Tunnel、Agent Task 生命周期。
- 插件安装包提供的英文及用户自定义说明保留原文，不擅自翻译或加不可信的语言标记。精确宿主原文变动后显示原文，不展示已失效的中文解释。
- 960px 只表示原生 Wails 桌面最窄已测视图，不代替 390px 的 Web 浏览器或手机验证；完整键盘焦点与读屏兼容性仍需专项覆盖，不能宣称整体达到 WCAG 2.2 AA。
- 代码已在 Wails DEV HMR 真实数据界面生效；正式发行包需要另行重新构建后使用。

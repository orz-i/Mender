# Mender Magpie 原生工作区布局 — 视觉与交互验收证据（2026-11-05）

## 参照与原则

参考用户提供的 **Magpie Agents、Add a provider、Routing（含嵌套）、Usage** 5 张截图和 [Magpie 的真实界面素材](https://github.com/yetone/magpie/tree/main/site/public/img)。本轮改造的判断基准是**整体页面布局、交互层级和内容组织**，不是色值、圆角或组件替换。

Mender 是工具/资源聚合、治理与供应平台，不是模型提供商代理与路由器；借鉴布局方法，不捏造 Magpie 的模型订阅、路由结果、余额、费率、供应商或 Agent 启停能力。Mender 的 Agent Loop、ACP 权限、Profile 工具供应以及安全治理不变量不变。

## 已实施的结构性改造

1. **单层全局 Shell**：移除原 228px 左导航、两层顶部导航和偏好切换，改为接近 Magpie 的 58px 居中横向 8 个常用入口；工具栏保留全局搜索、刷新、审批数量、设置与状态。低频 Tunnels / Credentials / Policy / Runtime 放在设置面板，手机使用全量抽屉。
2. **外观**：默认浅色以匹配所给参考，允许用户自行选浅色、深色、跟随系统，偏好本地持久化。
3. **Agent 列表**：全宽分隔列表；名称、实际 CLI 可用性、后端、工作区一行可扫读；无伪造启停开关。
4. **工具来源、Profile、Sandbox、Skills**：统一连续的资源行，而不是管理后台卡片墙和表格混用；对象详情与真实管理操作分开。移除多余的 View 按钮，保留凭据、同步、删除、权限与其他安全操作。
5. **插件发现**：按当前真实插件能力分类，紧凑多列 tiles，展示真实启用/可用状态，保留原始创建/安装/审查路径。
6. **运行时**：执行指标、全部当前预算限制、进程隔离策略同页展示；编辑仍为风险受控的独立表单，不再出现「四项指标后大片空白」。
7. **审计 Usage**：根据 API 返回的**最近记录样本**计算 14 天分布、完成/失败/工具数量，明确不是全量历史或精确商业用量。
8. **Profile Supply**：真实来源工具 → 选中的 Profile → 可配置交付出口的可视化解释，明确并非真实运行中的路由链路。

## 截图

**以下屏幕截图来自隔离 Chromium + Vite 测试会话中的模拟 GET API 数据，不是运行中的真实 Wails 数据，也不是实际业务服务状态**。仅用于比较布局与内容密度；不具备验证供应、审批或 Agent 执行的效力。

| 画面 | 证据路径 | 对标 |
|---|---|---|
| Agents 全宽列表 | [agents-desktop.png](agents-desktop.png) | Agents 内容组织与连接状态 |
| Agents 窄屏 | [agents-mobile.png](agents-mobile.png) | 无侧栏、紧凑可点击资源行 |
| 插件发现 | [plugins-discovery.png](plugins-discovery.png) | Add a provider 分类别紧凑 tile |
| 工具来源 | [tool-sources.png](tool-sources.png) | 原生资源/提供方行和管理操作 |
| Skills 桌面 | [skills-list.png](skills-list.png) | 技能信任与启停控件，整行详情入口 |
| Skills 手机 | [skills-mobile.png](skills-mobile.png) | 390px 中的列表操作分组和阅读顺序 |
| Runtime | [runtime-full.png](runtime-full.png) | 全部真实配置同屏，消除冗余空白 |
| Audit | [audit-usage.png](audit-usage.png) | Usage 指标和有样本边界的日分布 |
| Profile | [profile-supply.png](profile-supply.png) | Routing 的可视关系表达，但不冒充模型路由 |

## 自动化与可访问性

- `pnpm exec eslint apps packages`：通过。
- `pnpm -r --if-present run typecheck`：通过。
- `pnpm i18n:check`：**1,603/1,603** 必需中文文案齐全。
- `pnpm exec vitest run`：**75 个文件、494 个测试全部通过**。
- `pnpm -r --if-present run build`：Web 与 Wails Desktop **前端**打包通过。
- 浏览器 390×844：概览、工具、配置档、Profile 详情、Agent、插件、Sandbox、Skills、Audit、Runtime、Credentials、Policy 共 **12 个路由**，页面级 `scrollWidth === 390`。另外在 Skills 页面以 3 条隔离示例数据复查了桌面/移动内容与操作按钮。部分非真实服务路由可能有预期 API 错误，不视作业务流程验收。
- 浏览器 1480×850：单行居中导航、Agent、插件、运行时、审计、Profile 供应、工具来源均有截图与 DOM 快照。
- 浏览器 1024×768：中英文主导航无水平裁切，完整容纳 8 个入口；设置中浅色→深色→浅色切换与 `mender.appearance` 写入验证通过。
- axe-core 4.12.1：Agents 手机页面 WCAG 2 A/AA、2.1 AA：**0 个确定违规，1 个需人工检查，23 项通过**。
- 浏览器中出现的两个白屏问题起因是测试 API 模拟缺少 `issues` 数组和提供方 `description` 字段；已增加容错和独立回归测试，之后确认工具来源页面正常渲染，相关测试通过。

## 未达到的验收与限制

1. **Wails 真正实机**：Mac mini 中 `wails3 dev` 和 `mender-desktop.dev.app` 进程均在运行，但 Mac 当前处于**锁屏界面**，截图不能提供有效的应用窗口证据；Wails 原生窗口中的 UI 操作、审批、凭据、远程沙盒、Profile 交付及最小窗口宽度尚未完成实机核验。
2. Vite 预览使用隔离 GET API 模拟数据；实际管理 API 不能从该入口验证真实调用，不能据此宣称 Agent/Tool/审批等业务功能已实机通过。
3. 浏览器整体布局与用户提供的 Magpie 示意明显更接近，但不能称完全照搬 Magpie；功能、语义、对象类型不同，必须保持 Mender 自有产品边界。
4. 构建仍存在前端主包体积警告，该警告不阻断本次构建，但应作为独立性能治理任务，避免混入 UX 重构。
5. 本轮没有改后端、数据 schema、Agent 生命周期、ACP 或业务存储；不主动推送远端。

## 原生验收建议

用户登录 Mac mini 后，重新构建/刷新 Wails 开发窗口，再对比 Agents、插件市场、运行时、Profile 供应和 Audit 的**真实数据**截图与以上参考；同时检查顶部导航、设置/语言/外观、Cmd+K、审批角标及敏感操作确认。此之前不宣称整轮 Wails UI/UX 已完全验收通过。

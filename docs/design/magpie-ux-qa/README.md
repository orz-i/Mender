# Mender 结构性 UI/UX 治理 — 浏览器验收与差异说明（2026-11-05）

## 对照来源
- [Magpie GUI Shell](https://github.com/yetone/magpie/blob/main/docs/subsystems/gui-shell.md)
- [Magpie Agents screenshot](https://github.com/yetone/magpie/blob/main/site/public/img/agents-light.png)
- Magpie themes/source: `internal/gui/assets/app.css`.
- 前轮视觉 token 与原生表层基线：`docs/planning/magpie-inspired-ui-2026-11-05.md`。
- 本轮任务路径与验收目标：`docs/planning/magpie-ux-structural-redesign-2026-11-05.md`。

## 结构改造成果
1. **一级导航**：任务顺序 Workspace → Connections → Advanced；所有项目默认可见，减少默认隐藏分组造成的发现成本。保留侧栏折叠、移动端抽屉、命令搜索、权限化审批入口。
2. **布局切换**：侧栏／顶部两种布局由同一套路由驱动；顶部按分组与组内链接两级组织，不把全部 13 个目的地挤在一排。偏好写入本机 localStorage `mender.navigation.layout`；改变路由时，顶部导航跟随真实当前组。
3. **位置上下文**：所有详情和创建/编辑路由有真实可返回的 breadcrumb；Profile、Agent、Sandbox 与插件可展示已加载对象名称；移动端折叠历史路径，仅保留返回上一级与当前页面，优先保障标题可读。
4. **工作区导航**：Agent 七分区、Profile 四分区、Sandbox 四分区、插件三分区、Profile 编辑两分区使用共享 DetailWorkspace：宽屏左轨＋右主内容，窄屏标签横滑；业务状态与计数在分区上可见。
5. **列表→详情**：Agent、Sandbox、Profile 清单的对象名称成为直接入口，去除无意义重复“查看”操作列，不影响原有新增、搜索、过滤、状态和列表分页。Agent 概览删除与分区重复的“管理工作区”“查看工具”操作。
6. **首页**：同时呈现运行时/资源概况、来源清单、配置档和近期调用；取消活动与资源相互隐藏的标签切换。无调用时优先展示资源，只保留一条简短的审计空记录提示，不用大量留白或装饰性空状态。
7. **新建流程**：保留原有业务任务步骤及授权与确认逻辑，创建页面通过面包屑返回对应列表，Profile 创建后的工具编辑仍由原有流程承接。没有新增重复 Wizard 或假状态。

## 浏览器屏幕证据
**全部为 Vite + Chromium + 隔离 GET API fixture 的可视化示例**。数据仅存在自动化浏览器的网络拦截器中，不是生产数据库、实际 Agent 状态或外部插件验证；不可用的管理 API 不代表 UI 功能通过。

| 截图 | 路径 | 关注点 |
| --- | --- | --- |
| 顶部布局首页 | [overview-top.png](overview-top.png) | 两级分组、无记录时资源优先、单行审计入口 |
| 侧栏布局首页 | [overview-side.png](overview-side.png) | 全部 13 个一级入口与任务顺序 |
| Agent 详情桌面 | [agent-detail.png](agent-detail.png) | 对象面包屑、左侧主题轨和主内容、分区计数 |
| Profile 编辑手机 | [profile-editor-mobile.png](profile-editor-mobile.png) | 390px 详情路径简化、编辑分区控件、单列内容 |

## 质量验证
- `pnpm exec eslint apps packages`：通过。
- `pnpm -r --if-present run typecheck`：UI、SDK、App、Web、Desktop 均通过。
- `pnpm i18n:check`：**1,587/1,587** zh-CN 必需翻译齐全，清理了停用功能文案。
- `pnpm exec vitest run`：**74/74 测试文件、492/492 测试**均通过；新增 breadcrumb 与 DetailWorkspace 单测，既有管理流程测试更新为新的可发现入口。
- `pnpm -r --if-present run build`：Web、Desktop **前端**生产构建通过。
- Chromium 390×844 对照：概览、插件列表/详情、工具、技能、Agent 列表/详情/新建、Profile 列表/详情/编辑、Sandbox 列表/详情/新建、审计、策略、运行时、凭据共 **18 条路由**，body `scrollWidth === 390`。部分深层 API 未模拟时显式显示失败状态，不能视作业务流程验收。
- 真实浏览器交互：侧栏/顶部布局切换与 localStorage、⌘K 快速导航、对象位置路径、响应式 local topic rail、返回父级；均有自动化/截图证据。
- `axe-core 4.12.1` Profile 编辑移动页 WCAG 2 A/AA、2.1 AA：**0 个违规，0 个待复核，22 项通过**。
- 仍存在打包体积警告（Desktop 主 JS 包约 1.26 MB），未混入本轮架构变更。

## 限制与验收残项
1. 开发 Vite 代理实际目的地 127.0.0.1:8080 当前由 `otunnel` 监听，不提供 Mender 的管理 API；因此本轮**没有**完成真实 Profile/Agent/Sandbox 操作提交、策略/授权实时验证或 MCP/Agent 供应链验收。模拟浏览器截图不能取代真实操作。
2. 需在重建的 Wails Desktop 中实机查看窗口标题栏、最小窗口宽度、深浅模式、复杂长表单、审批、Profile 工具管理、凭据和 Sandbox 创建，之后才能标记完整的原生交互验收。
3. 此轮只有前端与 UX 结构改动，没有修改后端、schema 或生产业务数据。不新增不必要兼容层。
4. 视觉目标参考 Magpie 的结构和交互方法，仍保留 Mender 自有品牌、术语和业务边界，并非复制 Magpie UI 源码。

## 结论
**阶段性代码改造、桌面/移动浏览器视觉回归、自动化质量门禁通过；真实 Wails/管理 API 行为验收尚未完成。** 不应宣布完整的产品实机验收通过。

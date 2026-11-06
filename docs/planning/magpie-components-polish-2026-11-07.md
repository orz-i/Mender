# Mender · Magpie component refinement (2026-11-07)

## Goal and reference

The user wants a second-level fidelity pass across Mender's existing desktop/web screens, referencing the Magpie desktop UI screenshots they supplied: one horizontal global navigation, pale canvas, simple white surfaces, compact connected resource rows with recognizable icons, restrained inline status and minimal controls.

This is an **existing application**, not a prototype or a clone. Maintain Mender's distinct tool/resource governance behavior and all configured data. Do not invent model routing, usage balances or new backend contracts.

## Baseline (isolated Web browser, 1440×880)

- Shared `DataPanel` repeats decorative icon glyphs in every heading; 20+ pages inherit this, creating noise unlike Magpie's text-first sections.
- Empty `ResultPanel` often wraps an `EmptyPanelState` and a dashed `EmptyState`; compounded whitespace and borders obscure next actions.
- `DetailWorkspace` still allocates a fixed 172px left topic rail on desktop under an already horizontal global navigation; it compresses working content and diverges from the compact section navigation of Magpie.
- `Profiles` and `Sandboxes` have similar but independently styled resource rows; one uses a counter, the other colored badges and path, so spacing and actions vary.
- The common search field lacks an explicit one-click clear affordance and Escape-to-clear.
- Tools collection has a multi-button right edge that competes with the row's main resource action; needs hierarchy without hiding important management functions.
- Baseline routes `/`, `/tools`, `/profiles`, `/sandboxes`, `/runtime`, `/plugins`, `/skills`, `/audit` rendered without page-level overflow (fixture data). This is not native Wails verification; Mac mini's Wails window unavailable at audit time.

## Design system / acceptance

1. **Surfaces**: one outer card per section; border `var(--border)`, background `var(--card)`, radius ~14-16px; panel headings text-first at 13–14px, no decorative icon unless caller specifically requests one.
2. **Lists**: connected 76–80px rows with 36–40px icons, strong object name, one muted contextual line; quiet inline status (dot + text) instead of filled status pills, consistent keyboard focus and hover.
3. **Local navigation**: a single responsive, horizontally scrollable compact subnav above detail content (not another permanent left column). `aria-pressed`, section count and explicit labels remain accessible.
4. **Empty/loading/error**: one surface, clear sentence and concrete next action where the page has one; no nested dashed cards, no fake data.
5. **Search**: one search affordance with clear action and Escape, as well as keyboard labels; reset without losing pagination/query context.
6. **Responsive**: 1440×880 and 390×844; no body horizontal overflow; no clipped actions, accidental text overlap or hidden labels.
7. **QA**: preserve current page route/API/permissions and statuses; regression tests of shared primitives and representative Profile, Sandbox, Tool and detail navigation; lint, TypeScript, i18n, Vitest, web + desktop frontend build; browser screenshots and interaction proof.

## Staged work
- [x] A. Refine shared panels, detail navigation, empty states, search.
- [x] B. Apply consistent resource row/status anatomy to frequently used collections.
- [x] C. Check legacy management screens for overflow and compound toolbars, fix evidence-based problems (Tool service duplicate CTA, loud filters and audit statuses; existing management routes responsive).
- [x] D. Browser visual QA, responsive/accessibility, full regression and staged local commits. Wails-native validation remains separately pending because its window is unavailable on the Mac mini.

**Boundaries**: no backend, database, ACP, Agent task state, permission or user-data mutation. No blind global CSS override. Use only existing explicit UI data. Do not push without explicit user request.


## 实施与验收记录（2026-11-07）

**实现范围（仅前端）**

- 共用 `ResourceList`/`ResourceRow` 对齐配置档、沙盒的 76px 资源行：独立对象名称、次级说明、必要的右侧上下文和轻量状态圆点。未修改资源状态语义、后端存储或任务生命周期。
- `DetailWorkspace` 从桌面固定 172px 左轨改为顶部单行横向主题导航；在移动端仅内部横向滚动。`aria-pressed` 和数量计数保持可读；正文恢复全宽。
- `DataPanel` 不再自动为每个标题注入技术图标，保留明确由业务指定的图标；`ResultPanel`/`EmptyPanelState`/`EmptyState` 减少无效高度、重复边框和虚线边框。
- 搜索框支持清除按钮和 Escape 清空；已经在 Profiles 页实际验证“Work”过滤后点清除恢复 2 条资源。
- Tool services 的对象名称即查看入口，右侧不再有重复主按钮；编辑、认证、重绑、同步和删除继续由独立可访问菜单提供。Skills 的来源和启用状态改为文字+状态点，原有信任/启用/删除功能不变。
- Audit 表格状态改为内联圆点文字，详情诊断字段拆成简单的定义行；错误提示与活跃筛选的背景对比降低装饰性但不隐藏异常。UI 组件 `Badge` 的成功状态文本改用语义前景色，修复浅色主题下对比度不足的问题。

**浏览器 / 自动化证据**

- 隔离 Web dev 浏览器测试，桌面 `1440×880`、手机 `390×844`；API 返回模拟数据，无真实用户操作、没有写生产存储。
- 9 个主要路由：`/`、`/tools`、`/profiles`、`/agents/agent_demo`、`/sandboxes`、`/skills`、`/plugins`、`/audit`、`/runtime`，均验证 `document.documentElement.scrollWidth === 390`。
- 另测 `/credentials`、`/tunnels`、`/policy`、`/agents`、`/profiles/new` 的手机路由，页面没有横向溢出。`/policy` 的模拟环境缺少 API 响应，显示明确请求错误；不计作策略业务功能通过。
- 浏览器实际操作：Profile 查询→清除按钮→恢复两条资源；Agent 详情主题导航状态和内容确认；深色偏好保存且没有横向溢出。
- WCAG axe 4.12.1：Profiles 移动端 `0 violations / 0 incomplete`；Agent 详情浅色 `0 violations / 1 incomplete`、深色 `0 violations / 3 incomplete`。Incompletes 为自动工具无法判定的对比度目标，应作人工视觉复核，**不代表完全无障碍认证**。
- `pnpm exec eslint apps packages`、`pnpm -r --if-present run typecheck`、`pnpm i18n:check`（1639/1639）均通过；Vitest **76/76 文件、512/512 测试**通过；Web 与 Wails **前端**生产构建通过。保留已知 JS 主包超过 500kB 的体积警告，独立优化。
- 截图位于本机 `/tmp/mender-magpie-components-before-*.png`、`/tmp/mender-magpie-components-after-*.png`、`/tmp/mender-magpie-components-detail-mobile.png`、`/tmp/mender-magpie-components-dark-agent.png`；为隔离测试 fixture，无生产身份数据。没有把截图复制入 Git 仓库。

**保留边界**

用户 Mac mini 上的 Wails 窗口在本阶段无法访问，故本轮完成的是浏览器组件 QA 与前端构建，不宣称原生 Wails 点击验收或后端功能验收。未推送远端，未触碰数据库、ACP、任务状态、权限与敏感信息。

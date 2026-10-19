# Mender 全页面品牌视觉收口 — 实施合同（2026-10-19）

## 目标与边界

以 `docs/design/mender-visual-identity.md` 和用户最新的 Wails 截图为依据，收口产品的视觉基础与**全部现有管理页面**。Mender 定位保持“面向 LLM/Agent 的工具与资源聚合、治理和供应平台”。此轮是 **UI-only**，不改 SDK/API、持久化、Agent 生命周期、权限、隧道或沙盒语义。

**已保留的交互规则：** 一个视口只聚焦一类信息；列表/详情默认只读预览，进入新增/编辑/删除需显式操作；查询、分页、跳转、语言切换、侧栏折叠、空/错/加载状态不能退化；避免用颜色推断未知 Task 的状态。品牌 logo 与 Mendy 用法以既有设计规范为准，不盲目到处增加插画。

## 当前截图暴露的问题

- 约 1280px 宽的 Wails 窗口里概览 4 项关键指标被拆为 2×2，挤压最新调用表格空间；用**容器可用宽度**判定布局，不依赖最外层视口断点。
- 侧栏导航清晰但区块、顶部品牌、选中态与主内容需做版式标尺和交互态一致性检查。
- 卡片/表格/工具栏/状态徽章过分依赖页面内部重复 class，且部分标签只有 10–11px；统一到 shared UI 语义式，避免分散强制颜色。
- 初稿 logo 已应用，Mendy 只在真实空状态呈现；保留现有语言与尺寸容错。

## 代码分工

**主线路（本次 coordinator）**：`packages/ui/src/components`, `packages/ui/src/styles`, `packages/app/src/shared`, `packages/app/src/shell`, `packages/app/src/domains/overview`，以及其余未委派页面的跨页面风格复核。优先统一 KPI、表格、卡片、工具栏、空状态、状态语义与 1280/1440/390 布局。

**AGY 第一批**：只改 `packages/app/src/domains/plugins/plugins-page.tsx`、`packages/app/src/domains/skills/skills-page.tsx`。两页卡片与筛选工具栏视觉收口，入口/行为/业务逻辑原样保留。不得编辑 shared、config、router、package、其他业务页、品牌资源；不得提交；协调者审核和提交。Agent 返回 TaskID 后只查原 Task，不因为 unknown/超时隐式重复委派。

**后续域**：工具与配置档、Agent 与沙盒、Runtime/Servers/Tunnels、Policy/Credentials/Audit，以及详情/新增子路由；共享组件优先解决跨域问题，页面特性只局部处理。

## 验收矩阵

1. **基础**：字体、logo、品牌色/状态色、阴影/圆角、按钮/字段/标签/表格以及 dark 明暗对比；所有重要文本不少于 12px（极窄视口容错除外）。
2. **导航**：品牌与折叠侧栏、命令面板、搜索、语言、在线状态，无障碍 label/焦点态。
3. **概览**：桌面常见 1280px/1440px 窗口 KPI 优先单行且不破坏布局；小屏合理换行；工作区主题单一；空态 Mendy 仅一处。
4. **页面覆盖**：Overview、Plugins、Tools、Skills、Agents（含 detail/create/approval）、Sandboxes（含 create/detail/activity）、Profiles、Runtime、Servers、Tunnels、Policy、Credentials、Audit，以及现存兼容路由。
5. **质量**：`pnpm typecheck`、`pnpm lint`、`pnpm i18n:check`、`pnpm test`、`pnpm build`；至少 1440×900、1280×800、390×844 三种尺寸（包括暗色），有真实浏览器/桌面截图或明确标注阻塞；检查溢出和主要控件交互。
6. **交付**：文档先行与分段提交；只分段暂存已审计文件；不可包含 `.agents/`、`.pi/`、`.commandcode/`、`.intent-loop/` 或修改不相关运行时工作。

## 状态

- 设计合同：已冻结于本文件；AGY 分发必须返回可追踪 TaskID。
- 实现和验收：以随后的提交及 QA 记录为准，不提前宣称全量完成。

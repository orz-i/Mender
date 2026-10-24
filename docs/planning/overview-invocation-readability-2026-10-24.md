# 概览近期调用易读性收口 — 2026-10-24

## 现状与证据

本轮在运行中的 Wails Desktop 原生窗口打开“概览 → 近期调用”。截图：`/tmp/mender-ux-20261024/10-overview-real.png`（不提交截图）。中文环境仍常驻英文列标题 Principal、原始 `service:profile:prof_…` 治理引用和英文 `completed` 状态。用户无法直观判断调用来自哪个已知配置档，且 ID 占据列表宽度。

当前展示实现：`packages/app/src/domains/overview/overview-page.tsx`；审计详情页已显示中文 Completed/Failed/Denied，底层 API 同时返回 `listProfiles`、`listAudit`。正常 service Principal reference 为 `profile:<ProfileID>`，同一现有概览请求已拥有 Profile 列表，故无需额外 API。

## 最小展示契约

1. 预览列标题为“调用方”，不改变审计里的正式 Principal 概念。
2. 仅当 `principalKind=service` 且 `principalRef=profile:<id>`，并能精确匹配已载入 ProfileID 时，展示 `配置档 · <displayName/slug>`。
3. 未匹配的引用不可猜测归属。展示简短类型与引用尾段，以便区分不同历史调用；鼠标标题始终保留完整原始 kind/ref，审计详情保留原样。
4. `local_user` 正常展示“本地用户”；未识别的 kind 显示其原文，不私自改写。原状态值保持不变，但视觉标签按已有 Completed / Failed / Denied 文案翻译；未知状态不自动归类为成功或失败。
5. 不修改 SDK、API、Profile 授权、Audit 数据、持久化和详情页，不引入新查询或状态机。
6. 回归：Profile 匹配、Profile 不匹配、local_user、中文/英文状态、title 保留原文、异常与加载四态；lint/typecheck/i18n/相关测试/Web build；原生 HMR 截图与页面交互检查。

其他页面继续以真实证据为基础逐步治理，不将本轮两个切片冒充全页面验收。

# Mender × Magpie UI 改造验收记录（2026-11-05）

## 原型与实施边界

视觉参考：[yetone/magpie](https://github.com/yetone/magpie)，重点对照其 `site/public/img/agents-light.png`、`add-light.png`、`usage-light.png` 和 `internal/gui/assets/app.css`。Mender 保留自身品牌、工作区/工具/资源治理语义、所有路由和已有能力，不引入 Magpie 的 Agent 管理业务逻辑。

新布局采用紧凑侧栏（展开 228px / 折叠 64px）、54px 顶部工具条、冷灰视窗和白色平面工作区、系统字体、8–12px 圆角、行高约 36–50px、微弱边框及靛蓝选择/焦点状态。OS 深色外观匹配 Magpie 的暗色中性面板与文字色。信息类状态为靛蓝，成功绿色，警告琥珀色，失败红色，状态保留文本表达。

## 本地视觉截图

以下均为**隔离浏览器网络模拟**的合成数据场景，绝非生产业务数据截图；无需用户凭据、未向业务数据库写入数据。参考 Magpie 图像未复制入仓库。

| 视图 | 截图 | 验证要点 |
| --- | --- | --- |
| 概览 / 浅色桌面 | [overview-fixture-light.png](overview-fixture-light.png) | 紧凑顶栏、最近调用列表、运行状态、表格密度 |
| Agent / 浅色桌面 | [agents-fixture-light.png](agents-fixture-light.png) | 状态文字、可用性、搜索与行操作、右列对齐 |
| Agent / 暗色手机 | [agents-fixture-dark-mobile.png](agents-fixture-dark-mobile.png) | 系统暗色、搜索/操作自然换行、两段式数据行 |
| 插件市场 / 浅色桌面 | [plugins-fixture-light.png](plugins-fixture-light.png) | 轻量插件项、状态标识、内容布局 |

## 代码验证

- `pnpm exec vitest run`：72 个文件，485 项测试通过。
- `pnpm exec eslint apps packages`：通过。
- `pnpm i18n:check`：通过（原有翻译覆盖保留）。
- `pnpm -r --if-present run typecheck`：通过（UI、SDK、App、Web、Desktop）。
- `pnpm -r --if-present run build`：Web / Desktop 前端生产构建成功。
- 浏览器（Chromium，模拟 GET API 数据）：概览、插件列表与详情、工具、技能、Agent 列表与新增、Sandbox 列表与新增、Profile 列表与新增、隧道、运行时、策略、凭据、审计共 16 条路由在 390px 宽屏上均未发生**横向溢出**。无效或未模拟的部分 API 显示原有错误状态，不计入业务流程通过。
- 实测 Agent 列表 1280px 桌面宽、390px 移动宽，表格自适应为窄屏数据行；命令快捷键 `⌘K` 正常打开全局快速导航；移动端抽屉可打开并展示完整一级路由。
- 浏览器媒体偏好切换到暗色时，根节点 `dark` 生效，实际 `--background:#1a1a1e`、正文 `rgb(237,237,241)`；浅色恢复 `#f4f4f6`。
- Agent 暗色移动页面 `axe-core 4.12.1`：0 项已确认 WCAG 2 A/AA、2.1 AA 违规，24 通过、1 项不确定需人工复核。

## 验收局限和后续事项

1. 当前浏览器调试入口 `http://localhost:5190` 的 Vite 代理指向 127.0.0.1:8080，该端口当前由 `otunnel` 服务监听，无法提供 Mender 管理 API；因此使用了浏览器隔离路由模拟来验证主要列表、展示、筛选和响应式。**真实业务数据交互、复杂详情页与表单提交流程仍需在真正的 Mender Wails 运行时验收**，不得以模拟视图冒充生产验收。
2. 本阶段未更改任何后端接口、数据库结构、Agent/Task 生命周期或 Profile 工具供应配置；未接触远端真实 Agent、MCP 服务或 SSH Sandbox。
3. 已有 Web/Desktop JS 单块约 1.26 MB，构建报告仍提示打包体积警告（历史问题，非本次 UI 功能失败）。可以在未来独立做分包优化，不应混入本轮视觉治理。
4. 视觉/UI 与可用性验收属于本轮完成的范围；原生 Wails 真机重建后需再次人工检查平台窗口行为及操作密集表单，特别是审批、凭据和远程 Sandbox。

## 回归优先级

在有真实 API 的本机应用中优先回归：概览调用记录、插件过滤与创建、Agent 任务/审批、Profile 工具选择与编辑、Sandbox 创建和详情、工具服务列表、凭据页、审计筛选；最后核对窗口缩放、浅暗主题与键盘导航。

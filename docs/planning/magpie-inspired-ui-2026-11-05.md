# Mender UI 视觉与交互全面改造 — Magpie 参考（2026-11-05）

## 目标与范围

以 [yetone/magpie](https://github.com/yetone/magpie) 的实际桌面 UI、`internal/gui/assets/app.css`、README 页面截图为视觉参考，在 Mender 现有 React 19 / Tailwind 4 / shadcn 体系中实现统一、轻量、可操作的管理界面。**不是拷贝 Magpie 的产品功能、Logo 或源代码。** 保留 Mender 既有路由、能力边界、数据模型、API 与本地化。

对齐重点：桌面工具般的紧凑布局、低噪音信息架构、浅灰窗口底、白色平面工作区、细边界、清晰的列表节奏、控件可见性、键盘操作、深色主题。优先减少无效容器与图标装饰，而非隐藏功能。

## 参照证据（2026-11-05 查阅）

- `https://github.com/yetone/magpie/blob/main/site/public/img/agents-light.png`：平整列表面板、低饱和导航、每行明确的状态/操作。
- `.../add-light.png`：功能分组清楚，选择项轻边框、短高度、可扫描。
- `.../usage-light.png`：轻量统计条，少装饰，不强行卡片化。
- `.../routing-light.png`：密集信息使用分隔线、对齐与留白，不用大量深色图标底。
- `https://github.com/yetone/magpie/blob/main/internal/gui/assets/app.css`：其主题语义色为浅色 `#f4f4f6` 底、白面板、`#e3e3e8` 分隔、`#1c1c21` 前景、`#4f46e5` 强调；深色 `#1a1a1e` 底、`#232327` 面板。此处是设计参照，Mender 使用自己的 token 实现。

## 设计约束与决策

1. **导航**：Mender 页面比 Magpie 多，保留分组侧栏与折叠、移动端抽屉、⌘K 页面查找及审批入口；视觉采用 Magpie 当前代码中的 compact sidebar 方向，而不是强行把十余个路由塞进顶部菜单。
2. **全局布局**：顶部工具栏收至约 50–56px，侧栏展开约 226px、折叠 60–64px；标题改为紧凑无衬线字号；多余图标背板清理。动作靠近其所属内容，避免重复一级操作。
3. **主题**：以冷中性灰/黑白为主、靛蓝只用于交互焦点及选择，状态使用独立语义色；保留现有 light/dark 切换机制。不得使用只靠色彩传达状态的表达。
4. **信息密度**：一级表格/列表行目标 44–56px，数据面板标题约 52–64px；数字概览不需要每格 108px 图标卡；输入保持易操作的 34–38px；表单说明依复杂度保留。
5. **架构**：先共享 token/primitives/shell，再共享 Page/Workbench，再验证 Overview、插件、工具、Profile、Agent、Sandbox 等页面。严格避免因全局 CSS 修改破坏特殊表单与 Wails 控件。
6. **动效/无障碍**：保留可见焦点、reduced motion、键盘访问、空/加载/失败状态；不能为了简洁隐藏状态、审批、错误或管理操作。

## 分段实施与验收

- [x] A. 建立设计规范与可回归基线（参考截图、基线页面、语义 token）。
- [x] B. 统一全局主题、基础控件、对话框、表格与徽标；确认浅/深色。
- [x] C. 改造桌面与移动端 Shell，导航、搜索、审批、语言与在线状态保持原行为。
- [x] D. 改造共享工作台与页面结构，重点页面按 Magpie 紧凑结构收口。
- [x] E. 全量 typecheck、lint、Vitest、Web/Desktop 构建；浏览器验证关键路由、响应式、键盘与控制台。

独立分段提交。未通过功能回归、实机/浏览器检查的部分不得宣称验收完成。改造完成前不更新后端协议或进行生产数据迁移。

阶段验收报告与模拟数据截图：[`docs/design/magpie-ui-qa/README.md`](../design/magpie-ui-qa/README.md)。真实 Mender 管理 API / Wails 交互待另行实机验收，详见报告中的限制说明。

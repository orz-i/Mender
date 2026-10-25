# Mender Agent Task 列表的可信名称与低密度显示（2026-10-25）

## 现场审计与边界

- 原生 Wails 的 Agent → OpenCode →「任务」列表目前只显示 38 位 `atask_...` TaskID、状态、创建时间与「查看」。普通用户难以凭随机 ID 识别自己提交的不同任务；详情弹窗已在上一轮完成渐进披露，不再重排。
- `backend/internal/agent/domain/task.go` 明确约束：**不持久化输入 Prompt**，Prompt 只在运行器的短期内存中；保留此隐私边界。已检查 `AgentTask` REST schema 和 SQLite：当前没有可信可展示的任务标题，不能通过截取 Prompt、Agent 输出、错误诊断或 Session 推断任务名称。
- 研究依据：NN/g 的 recognition over recall（https://www.nngroup.com/articles/recognition-and-recall/），W3C WCAG 2.2 2.4.6（https://www.w3.org/TR/wcag/）。

## 本轮最小合同

1. **可选显式 Task title**：委派工具 `delegate` 增加可选 `title`（面向用户的简短非敏感任务名称，最长 80 个 Unicode 字符）。由调用方明确提供，Mender 不从 Prompt、输出、诊断或 Agent Loop 自动推导，不设置默认伪造摘要。必须在 Task admission 前验证，拒绝前后空白、换行、控制字符和超长值；空值在省略字段时仍兼容旧调用。
2. **Task 而非 Session 元数据**：`title` 属于这一次 Task 的持久化展示元数据，不影响执行 Prompt、工作区、准入、工具权限、取消/完成判定、原生 Session、恢复条件与 Agent Loop；在 `Task` 标准 JSON/Owner REST 中以可选字段返回。`delegate` 的返回和经授权的 `status`/`result` 可回显已提供的 title，未提供则省略。
3. **职责和隐私**：Task title 将持久化并可在拥有者 UI 上看到；调用方应只传非敏感、概括性的标题。不会持久化 Prompt，也不提供后台自动摘要、LLM 再次调用、系统主动读取 Prompt、内容敏感词扫描等复杂机制。已有的 Owner/精确 Profile Task 授权保持不变，title 不扩大可见范围。
4. **旧数据无损升级**：SQLite 加版本化单列迁移；旧 Task 默认 `title=''`，不能回填/重写历史 `unknown`、结果、审批和 Session。新 Task 的 title 只在 admission 原子插入时保存，终态不可改；从原生 Session 继续的新 Task 是独立 Task，不自动继承原任务的 title。
5. **列表和详情 UI**：有 title：列表第一行显示真实 title，第二行显示短 TaskID；无 title：显示明确的「任务 #末尾 8 位」以区分老记录，不伪装含义。完整 TaskID 始终留在 `title` 辅助信息和结果详情中。行高适中；状态、创建时间、查看/取消能力完整保留，按钮可访问名称包含当前任务的显示标题。任务结果弹窗有 title 则显示在 TaskID 之上，没有就维持原样。
6. **避免重复 UI**：不增管理页面、手动重命名功能、额外 Task 摘要存储、自由编辑 Task 标题、搜索索引或新 Agent 生命周期状态；后续真的需要可再基于使用证据评估。

## 实施和验收

- 文档先行，后端领域验证/SQLite v19 迁移/原子创建/工具 schema/开放 API SDK/前端列表详情，分阶段提交。
- 测试：空 title 旧任务兼容；80 Unicode 字符边界、控制字符/换行/前后空白拒绝；显式 title 与 Prompt 区别，SQL 重启后 title 保留且不得泄漏 Prompt；v18→v19 迁移保留现有 terminal/unknown Task 且 title 为空；严格工具输入不变，授权不变；原生继续 Task 不继承旧标题；Owner REST/SDK optional 字段；中英文列表和详情含 title 与回退编号，完整 ID 可访问；点击查看/取消仍命中原 TaskID。
- 运行 Go 精确/全量测试、TS SDK 生成校验、i18n、ESLint、Vitest、Vite Web/桌面构建；原生 Wails 使用既有历史真实数据验收，无副作用创建 Agent Task。若需要验证新 title UI，只使用 mock 客户端的隔离测试，**不在真实 Agent 上提交测试 Prompt**。

## 明确的限制

- 历史任务不含可信 title，前端只显示短 ID；本轮不尝试反向推断历史任务的内容。
- 只有未来通过已更新且重新发现/同步的 delegate 工具调用并明确提供 `title` 的 Task 才会出现人类可读的名称；未更新的工具 schema 不应被假定支持此字段。
- 实机桌面验收并不等于 390px 移动浏览器或屏幕阅读器完整合规验收。

## 实施记录与验收（2026-10-25）

### 分阶段提交

- `3e5215f`：文档先行；Task 标题单一可信来源、不持久化 Prompt、历史无名称不回填。
- `948cf13`：Agent 领域与 Submit admission、委派工具、SQLite v19、OpenAPI SDK 及隔离回归测试。
- `8d564d2`：更新只用于测试的历史 schema downgrade fixture，确保升级到 v19 后不绕过 v7/v18 安全迁移验证。
- `01750ff`：前端 Agent Task 名称/回退编号/全 ID 追踪与有名称的结果详情。
- `e4b5796`：仅对 Agent Task 表格行收紧垂直间距，不影响全局表格和操作按钮尺寸。
- 后续补充取消 TaskID 精确路由断言与本文验收记录，分别提交。

### 验收结果

| 验证对象 | 结果与证据 |
| --- | --- |
| Backend 全量测试 | `go test ./backend/...` 通过，95 个 test-bearing 包均为 `ok`，含跨模块 bootstrap 和 SQLite v18→v19 老数据迁移安全测试。 |
| 标题领域验证 | 省略 title 不改变历史和现有调用；限 80 个 Unicode code points；非打印字符、换行、前后空白及超长名称在 admission 前拒绝。 |
| 工具入口隔离测试 | 使用临时数据库与惯性/模拟 Agent 执行器从 `delegate` 提交显式 title，Owner Task 清单可读取；Task JSON 不包含执行 Prompt。 |
| 全量 Vitest | **71 文件、467 项通过**（含有名称/旧 Task 混合列表、精准查看、确认后按完整 TaskID 取消的 mock 行为）。 |
| 前端与契约检查 | `pnpm lint`、`pnpm --filter @mender/app typecheck`、`pnpm i18n:check`（**1594/1594**）、Web 和 Desktop 前端 build、`pnpm sdk:check`、`git diff --check` 均通过。保留既有的大分包警告。 |
| 原生 Wails 1239×851 | 使用当前运行的数据，OpenCode Agent 任务列表有 38 条历史任务，每页 10 条；名称未知的项目呈现 `任务 #xxxxxxx` 而不是长随机 ID；行距缩减后首屏可完整看到表格和分页。截图 `/tmp/mender-task-title-native/02-tasks-compact-1239.png`。 |
| 原生 Wails 960×744 | 列表列宽与按钮无横向裁切或重叠；分页位于正常纵向滚动区域，不能宣称无需滚动。验收后窗口恢复 1239×851、(340,85)。截图 `/tmp/mender-task-title-native/03-tasks-compact-960.png`。 |
| 旧任务结果 | 在列表点「查看 任务 #bef73535」实际打开发起对象的完整 `atask_61edd839a48cf66f47961e24bef73535`，状态、失败诊断及执行详情仍正确。截图 `/tmp/mender-task-title-native/04-task-detail-trace.png`。 |
| 明确 title 的未来任务 | 页面使用注入 `MenderClient` 的隔离 Vitest 证实 title 正确展示、完整 ID 留存、查看/取消路径准确；没有向真实 Agent 提交测试 Prompt。 |

### 发布与运行限制

- 本轮只在 Wails DEV 当前**前端 HMR** 中实际展示历史 Task UI。它使用的桌面后端进程未按本轮代码变更重新构建重启，不把此情况表述为原生后端已加载 `v19` 或能接受新增 `title`。
- 新 Task title 的后端持久化经独立 Go/SQLite/委派工具测试覆盖；在目标环境需要重新构建后端并启动以执行 v19 的 forward-only 迁移，再显式同步受管 `Agent` 工具来源以使用新输入 schema。**不要**将测试当作已经执行了真实库迁移或真实 Agent 委派。
- 历史 `unknown`、会话、审批、Profile 授权、原始结果及已知不可恢复结论保持不变。Wails 960px 测试不等于 390px 手机，也不是屏幕阅读器无障碍完整验收。
- Screenshots 保存在本机临时目录 `/tmp/mender-task-title-native/`，不是仓库文件、也没有写入历史数据。

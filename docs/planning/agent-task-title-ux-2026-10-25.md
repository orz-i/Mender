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

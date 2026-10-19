# Agent 任务期限、观测与能力真实性治理（2026-10-20）

## 意图和真实基线

生产环境曾在 10 分钟恰好截止时将 Agy Task 标为 `unknown`，即使 AGY 在截止前完成 Git commit 和输出了新的原生 progress；正确的 Task 业务终态并未收到，不能伪造 `completed`、重放旧 Prompt 或猜测 Session 可安全恢复。相关只读证据见 `docs/planning/agent-delegation-blockers-2026-10-19.md`。

Mender 仍是 Tool/Resource Aggregation、Governance、Supply，不引入 Agent Loop、后台 Scheduler、新的“未知已处理”业务状态、持久化 Prompts 或原生 stderr。

## 本轮可执行合同

**A — 真实可解释的 Agent 状态。** 在已有 v14 `agent_execution_attempts.last_progress_at` 上增加只读 `observation.lastNativeProgressAt?`，与 `lastObservedAt`（最近可信生命周期阶段）及 `lastWorkerHeartbeatAt`（仅 Mender 监督线程）分开。原有 agent task status `unknown` 保持不变。所有接口路径（Domain → SQLite → HTTP/OpenAPI/SDK → Wails bridge → Agent Task Detail → MCP task Tool result）一致；不存在的历史 Native progress 字段省略，绝不猜测。任务详情说明文件提交不等于原始 Task 已获原生成功回执。

**B — 有限且显式选择的执行预算。** 当前固定 600 秒只能容纳短任务。仅为**未来新委派**新增可选 `timeoutMinutes` 参数，允许精确 `10 | 20 | 30`，默认 10。交互无新全局偏好和必填项；超出值早期拒绝，不隐式舍入、不自动延长当前运行任务。Task 入库时持久化不可变 `timeoutSeconds`（600、1200、1800，旧 Task 默认 600）；Runtime 使用 Task 快照创建 context deadline，`resume` 新 Task 继承原配置，不继承旧授权/审批/Tool token。现有 10 分钟旧记录不可迁移为更长预算。独立跨 Runtime 过期对账不应按旧固定 12 分钟提前回收 20/30 分钟任务。`deadline` 可以由 Task `createdAt + timeoutSeconds` 可靠推导，Task 查询永不启动进程。保留 `DeadlineExceeded => unknown`（原生结果不明），不自动 retry。

**C — 准确公开后端续接能力。** Agy 的 headless `session/load` / `session/resume` 不可用；`resume` SourceKey/ToolID 需要考虑已经存在的 Profile 绑定，不能粗暴删掉 Catalog 元数据而造成悬挂 ID。阶段内至少对其工具说明、UI 入口明确标为“不可用”，在调用前保留后端拒绝；之后仅在兼容性/Profiles 绑定完全验证后考虑可发现性收口。ACP v1 的 capabilities 必须经过实时协商，Session ID 记录不等于恢复能力；已完成 Task 的上下文 continuation 不等于未完成原 Prompt 的结果恢复。不要扩展 native permissions。

## 分工与验收

- 后端/协议/持久化由协调方实现：`backend/internal/agent/**`、HTTP/SDK/Wails 映射与测试。Forward-only v18 migration，不改变 v17 已有数据/表意义；严格管理测试 fixture 的临时 downgrade，不能因升级失败更改本机生产 SQLite。
- Agent 详情 UX 可单独委派 Agy：仅 `packages/app/src/domains/agents/agent-detail-page.tsx` 与对应测试和 `packages/app/src/i18n/zh-cn.ts`。其余文件协调方维护。Agy 不碰 Go/SDK/generated 或规划文档，不重派未知旧任务。
- 通过含超时、参数边界、长预算恢复对账、native 与心跳区分、跨来源审批和原生结果无效/迟到之负向单元测试；`go test ./backend/internal/agent/...`、相关 runtime/Wails/HTTP/SDK contract、前端 TypeScript、Vitest、Lint、打包通过。只有单独复构建 Wails 后才能称真实生产实现验证。旧 `unknown` 仍不可强制转为完成。
- 既有未跟踪 `.agents/`, `.pi/`, `.intent-loop/`, `.commandcode/` 不操作、无 `git clean/reset`。交付以分段 commit + 可核查测试和未覆盖项为准。

## 标准参考

ACP v1 `session/load` 明确需要 `loadSession`，`session/resume` 需要 `sessionCapabilities.resume`；v2 baseline `session/resume` 仅恢复上下文。MCP Tasks (2026-07-28) 的 `working/completed/failed` 状态及 result/polling 与本地 AgentTask 的不确定性语义不同；不借用其扩展来伪造原生成功。

## 实施完成及质量门禁（2026-10-20）

### 实际交付

- `94d58cd`：从现有 `last_progress_at` 投影真实 Native Agent 活动时间，单独保留 `last_observed_at` 与 `last_worker_heartbeat_at`；OpenAPI/SDK/Wails/MCP Task status、result 类型贯通。
- `4bc3bff`：`agent.delegate.timeoutMinutes` 可选 `10|20|30`，默认 10；AgentTask 持久化不可变 `timeout_seconds`（600/1200/1800）。SQLite **v18 前向迁移**为历史记录只加 600 秒默认，不重判既有结果；进程 deadline 使用 Task 快照，执行超时仍为 `unknown`；跨 Runtime 对账保留独立安全余量，按每任务预算延后对账；`resume` 新生 Task 继承预算、仍须独立授权。持久化与历史数据、generation fence、Task/Provider canonical identities 测试通过。
- 同一提交同步真实工具描述：不支持会话续接的 Antigravity/非 Host Agent 仍保留历史 `agent.resume` ToolID 和显式 Profile 绑定，不假装可恢复，而标为 `Session continuation unavailable`、`unavailable` tag 并在调用时原有 guard 拒绝。Host ACP 只能在运行时已实际协商支持、原 Task completed/terminal/reaped 后安全继续，不保证每次恢复。Agent facade 的 Owner `EnsureAgentTools` 刷新新工具 Schema 时校验原 ToolID 未旋转，Profile 授权不变。
- `effd289`：Task 详情区分“原生 Agent 最新进度”和 Mender Worker 心跳；显示 admitted 预算和截止时间；遇 `unknown` 提示核验实际交付、不自动重派。没有增加新按钮、状态或 Agent Loop。
- `9a210c4`、`c9abe77`：真 runner Context Deadline 的 600/1200/1800 秒测试与 gofmt 收口。
- `3c71a5b`：Wails 返回的 `timeoutSeconds:number` 严格检查 600/1200/1800 才投影至 OpenAPI enum；不接受无效值，也不混合两个进度时间；新增 Desktop Bridge 正向/负向测试。
- Web 独立开发实际委派给 Antigravity Task `atask_efaa8b6bdb07484161177e37692d8e47`，该任务再次恰好 10 分钟 `unknown`，无持久化完成回执、没有真实前端改动。**未**重试或继续未知 Task；协调方完成并单独测试独立前端修改。该 Task 是一个普通不可变历史记录，未擅自修改其状态。

### 测试结果（本次）

- `go test ./backend/...` **全部通过**，包含 Agent / Bootstrap / HTTP / Wails 绑定；`pnpm go:lint` **0 issues**；`go test -race ./backend/internal/agent/...` **通过**。没有声称运行全部 Go 包 Race 的完整 `pnpm check`。
- `pnpm exec vitest run --maxWorkers=4 --reporter=dot`：**53 文件 / 325 测试通过**。首次并发 Vitest 测试因 Docker 页面定时器挂钩测试时序失败，单文件复测与降低并行度的完整回归均通过（未改无关业务测试）。
- `pnpm --filter @mender/app typecheck`、`pnpm --filter @mender/desktop typecheck`、`pnpm --filter @mender/sdk typecheck`、`pnpm lint`、`pnpm i18n:check`（1433 翻译）、`pnpm sdk:check`、`pnpm desktop:bindings:check`、`pnpm desktop:architecture:check`、`pnpm --filter @mender/desktop build`、`pnpm --filter @mender/web build` 均通过。
- 生产 SQLite **只读核对**仍为 Agent v17、默认 Profile 31 项原 ToolBinding、`PRAGMA quick_check=ok`、外键异常 0，未存在 v18 列。这证明目前仅仓库代码、模拟测试 Fixture 已实施迁移，**不是**已经修改运行中生产数据库。当前 Wails 和 MCP Tunnel 未被协调方重启/重连或执行任何生产工具预算冒烟。

### 需要在用户重新构建并连接后实机验证

1. 重建 Wails Desktop 后会由本机 Runtime 在既有数据上前向升级 Agent SQLite v17→v18。请先保留用户现有 DB 快照，再验收库的 `timeout_seconds` 旧行 600、Profile ToolBinding 及 Agent Task `unknown` 均未改变。
2. Agent 详情执行 **Owner 显式同步/修复 Agent Tools** 才能使已持久化的旧 Agent facade `delegate` 新 Schema 和 `resume` 后端能力说明刷新至 Catalog。不要自动增加 Profile 中第 5 个 Tool 授权，也不要静默重写旧 ToolID。MCP 运行服务器如果持有旧 Adapter 快照，需遵循其现有热更新机制或经用户同意的服务重启，而不是宣称刷新新 binary 即覆盖在运行的 Profile Tool Schema。
3. 用一个**全新隔离任务**显式提供 `timeoutMinutes:20`，验证 Profile 只读的 status/result 包含 `timeoutSeconds:1200`、两个独立进度时间；再核对进入终态的 Native `result`。不在原历史 `unknown` Task 重发 Prompt，不主动在生产环境等待 20/30 分钟、自动延时或扩大授权。
4. Agy Native Headless 依然不能无损续接在途 Prompt，也不能通过 Mender 接管原生 ACP approvals。新版改善**执行预算与可解释证据**，**并不意味着解决所有超时后的结果回收问题**；将来确有确定后端终态查询能力时才允许新增独立证据核对操作。

保留未跟踪 `.agents/`、`.pi/`、`.commandcode/`、`.intent-loop/`；不修改用户生产 Task、Profile/Policy、Credential 或已有 Agent 授权。

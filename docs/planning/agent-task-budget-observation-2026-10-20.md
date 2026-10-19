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

# Agent 生命周期恢复与 Session Resume：实施计划

- 日期：2026-10-18
- 状态：调研完成，ADR-0057 为 Proposed；运行时代码、生产 DB、授权及原生 Session 尚未修改。
- 工作边界：Mender 是工具/资源治理供应平台，不实现 Agent Loop。文档先行，分段验收提交，不增加兼容桥。
- 技术合同：[ADR-0057](../architecture/adr/0057-agent-session-lifecycle-recovery.md)

## 真实问题与源码证据

生产数据库只读检查：OpenCode 与 Antigravity 在 2026-10-18 各存在一条确切 unknown Task，均在创建 10 分钟后写入 `Agent outcome unknown after timeout`。现有证据不能证明原任务成功、失败或还有脱离主进程的副作用；不重放、不猜测修复。

- `backend/internal/agent/application/service.go`：固定 10m context deadline；goroutine 和 active map 不可重启恢复。
- `backend/internal/agent/domain/task.go`、`infrastructure/sqlite/store.go`：没有 SessionID、执行归属或进度，unknown 为 SQL 强制不可变终态；重启后 `ReconcileExpired` 无进程/会话探测。
- `infrastructure/acp/runner.go`：收到原生 `session/new.sessionId` 但未持久化，进程销毁即失联。
- `infrastructure/antigravity/runner.go`：尚未读取 `init.conversation_id`；NDJSON step_update 仅消费不形成持久活动证据。
- `runtimegovernance/infrastructure/processisolation/command_unix.go`：context 终止整个已知进程组，但这不证明原任务的业务结果。
- `infrastructure/acp/task_home.go`：Restricted Host governed ToolSupply 会使用并删除每次空临时 HOME；必须检验原生 Session 数据是否能在下一次进程存活。
- 本机只读版本/帮助核验：OpenCode v2.0.30，agy v1.4.2，Grok v1.0.62。

## 设计要点

| 能力 | 约束 |
| --- | --- |
| AgentSession | 持久化后台 opaque native SessionID、AgentID、Workspace、授权身份、实际协商的恢复能力和存储范围；TaskID 不等于 SessionID |
| AgentExecutionAttempt | Task generation、runtime owner、可信进程组身份/启动时间、终态原生证据、最后活动与审计摘要 |
| Observation | outcome/activity/session recoverability/recovery evidence 独立；失去网络或时间到期不等于失败或成功 |
| Resume | 新 canonical `agent.resume` Tool，参数仅 `taskId`、`prompt`；查绑定的原 Session，经过授权与静止证明，新任务、新 TaskID、同一原生上下文 |
| Read-only reconcile | 尽量通过现有 status/result 展示检查信息；owner 的主动 reconcile 不重新提交旧 Prompt |
| Timeout | 不以延长 10 分钟作为唯一解决方案；先捕捉 last observed/event/proof，再设计有上限的可配置任务预算 |
| Security | 不以 SessionID 作授权凭证；原 ACP Approvals/MCP Task bearer 不继承；原 Workspace/Isolation/Policy 必须重新验证 |

## 分段实施与完成条件

### A — 冻结合同（本轮文档）
记录用户反馈的 Resume 工具、跨后端持久化差异、生产真实复现和硬性安全边界。不改生产任务、数据库、Agent/Profiles/Trust 或运行时代码。ADR-0057 处于 Proposed 状态，待实现与验收后更新。

### B — Session/Attempt 证据基础
Forward-only SQLite schema 引入 AgentSession、ExecutionAttempt、bounded evidence/last activity。ACP 必须在新 SessionID 入库成功后才写 `session/prompt`；Antigravity 在可见首个 `init` 中 ASAP 写入 conversation_id。强制 generation/owner/identity fencing；在进程启动、写 DB、prompt 发送、ACK、terminal/result 持久化每个边界做 crash 注入。隐私禁止保存 raw prompt/token/reasoning/全部 diff。

**完成条件**：可将 Task 精确关联到会话及进程尝试；早期失败不假装 completed；无 stale writer 覆写；所有普通老 Task 可读，历史 unknown 原样保留。

### C — 活动观测与证据核对
持久化有边界的进度摘要、lastObservedAt 和 OS 启动身份，区分 active、quiescent_proved、uncertain；只读状态查询绝不启动 CLI、执行 prompt 或绕过权限。Owner 显式 reconcile 可请求纯观测能力，但结果必须是原执行的权威证据才可提供 resolved outcome。无 SessionID 的两条生产 unknown 必须显示无法验证，禁止隐式扫描本机全部会话寻找“最像的一条”。

**完成条件**：SIGKILL、断开管道、Runtime 重启、PID 重用、任务结果持久化边界、安全取消的故障注入通过。

### D — Native Session Resume 和新 Tool
ACP v1 运行时协商 loadSession/resume/list；OpenCode 先在 disposable workspace 演示同 Session 跨进程继续。Antigravity 仅指定精确 `--conversation <id>`，禁止 `-c` 最近会话缓存。Grok 仅在当前 ACP session 能力实证后启用；CLI `--resume` 不是可自动替换 Mender ACP 执行器的理由。新工具在原 Agent managed facade 上新增独立 `resume` SourceKey，旧 4 个 ToolID 不动；Profile 不自动获得第五项。新 Prompt 作为新任务，包含 continuedFromTaskId，禁止自动重发原 Prompt、并发重复执行、未静止强行继续、跨 Profile/Agent 访问。

**完成条件**：3 种 backend 分别给出可恢复或受控不支持的证据。尤其受治理 OpenCode 的 private HOME 被删除以后如何可安全恢复须实机证明；若不可满足，不能标记为支持，也不能放开原用户 HOME/Seatbelt、拷贝凭据或复用旧临时 Tool bearer。

### E — UI/API 与生产验收
Agent → Tasks 保持紧凑列表；Task detail 才显示 last activity、执行证据、原生 Session 关联及明确的“核对状态 / 继续会话”操作。旧 ToolID/Source-first Profile 选择/原 AgentGrant 保持不变；只有显式选择第五个 resume ToolID 才能通过 MCP 调用。UI/HTTP/OpenAPI/SDK/CLI/Wails 一致，不能生成自动授权。完整测试通过后，单独由用户重建 Wails 与 MCP 刷新，再做真实 OpenCode/Antigravity/Grok Resume 小型验收。

**完成条件**：零兼容桥、独立正/负向集成测试、跨 Agent/Profile 拒绝、拒绝旧审批/令牌继承、错误 Session 不会自动新建、正确保留历史 unknown；`pnpm check`（Go race、lint、TypeScript、SDK、UI）通过。

## 非目标和实际风险

ACP v1 `session/resume` 只恢复上下文，不能证明原 in-flight prompt 持续运行；resume 不等于 retry 或原任务成功。不要引入 Temporal、统一 RemoteTask 或新 Agent Loop。MCP Tasks（2026-07-28 扩展草案）以后可映射到同一内部持久化记录，不能取代本轮生命周期治理。Grok CLI 不同版本存在 `--session-id` “创建会话”语义；OpenCode run 会在缺少 ID 时可能创建新会话；agy -c 可能回退到新会话；这些都不可用于无声恢复。

## 来源

- https://dev.opencode.ai/v2/docs/cli/acp/
- https://antigravity.google/docs/cli/headless/
- https://antigravity.google/docs/cli/commands/resume
- https://docs.x.ai/build/cli/headless-scripting
- https://github.com/agentclientprotocol/agent-client-protocol/blob/main/docs/protocol/v1/session-setup.mdx
- https://github.com/agentclientprotocol/agent-client-protocol/blob/main/docs/protocol/v2/migration.mdx
- https://tasks.extensions.modelcontextprotocol.io/specification/2026-07-28/tasks
- https://docs.temporal.io/design-patterns

## B/C implementation checkpoint — 2026-10-19

Scope: **Agent execution evidence and safe observation only**. Native resume, in-flight stream reattachment, evidence-backed terminal resolution, operator-run backend reconciliation, and changed Task deadlines remain **not implemented**. This is not a claim that historic unknown tasks can be recovered.

- Agent SQLite v14 forward migration creates immutable native SessionID bindings, Task-scoped execution attempts, up to two separately accounted pre-prompt process launches, and bounded categorical audit events. Native IDs, PIDs, raw output, prompt and tool arguments do not enter Task JSON or Profile-facing Tool results. Upgrading old DB rows does not infer any missing SessionID or mutate historical completed/failed/unknown results.
- Before launching an Agent process, Mender atomically reserves an attempt for the admitted TaskID/generation/Runtime nonce. ACP persists the exact negotiated `session/new` ID before `session/prompt` and observes native updates; Antigravity stores `init.conversation_id` as soon as it arrives, after stdin submission. Native completion evidence and actual child process reaping are separate events. A lost durable evidence write after prompt dispatch is `unknown`, never a fabricated success.
- New bounded `TaskObservation` returned from existing authorized `status`/`result` and owner Task APIs includes execution phase, last native observation, separately recorded Runtime worker heartbeat, SessionID-recorded bool, recoverability `unavailable`/`unverified`, leader-reaped bool, and `local_active` only on the current owning Runtime. No post-restart process reattachment, OS PID-based cancellation or opaque session identifier disclosure. A new `Refresh status` action in Task detail is a read-only lookup, not a restore/replay button.
- Agent `TaskTimeout` remains 10 minutes. Expired old `unknown` and new unknown Task records remain immutable; only a future separately authorized inspect/recovery implementation may append evidence. In particular, the recorded native session or a fresh worker heartbeat **does not prove the Task completed**, and leader reaping **does not prove detached descendants stopped**.
- Restricted-host governed OpenCode uses a disposable private HOME per process which is deleted afterward. A stored native ID alone therefore **does not enable session resume**. Implementing the fifth `agent.resume` Tool, isolation-safe session storage and policy/admission remains a separate stage D, requiring fresh disposable real-CLI cross-process evidence. No new Profile ToolBinding or Agent approval/trust grants were created by this checkpoint.
- Migration tests that intentionally construct v7–v13 fixtures now remove the v14-only synthetic tables *inside test fixtures* before downgrading. No compatibility branch was added to production migration code; new v14 DDL fails on ambiguous existing objects rather than using `IF NOT EXISTS`.

Acceptance to report separately: targeted Go/SQLite/ACP/Antigravity/Bootstrap tests, full `pnpm check` including race, Wails and SDK generation, and no changes to the owner's production SQLite. The work requires a later explicit owner rebuild/deploy to activate in Wails. The four existing Agent facade ToolIDs remain unchanged; existing management and MCP discovery semantics remain intact.

### B/C code quality gate — complete (2026-10-19)

- Git implementation: `8ef7bb4`; architecture checkpoint: `bf1ee18`.
- Complete `pnpm check`: **exit 0**, log `/tmp/mender-agent-lifecycle-check-20261019.log` (local Mac mini). Includes all backend Go tests and full `go test -race ./backend/...` (Bootstrap race package 298.473 s), 0 golangci-lint issues, SDK and plugin contracts, OpenAPI generated types, generated Wails desktop binding check, desktop architecture, TypeScript, ESLint, 1394 i18n messages, 47 Vitest files / 286 tests, and Web/Desktop production builds. Vite emitted its existing non-fatal bundle-size warning.
- Real fault-injection tests include missing native Session persistence before ACP prompt, post-send durable evidence loss, Antigravity `init.conversation_id`, pre-prompt ACP fresh-process retry, bounded/unforgeable SQLite event fence and historical unknown preservation. Real inference and cross-process `resume` were **not exercised**.
- Repository staging respected `.agents/`, `.pi/`, `.intent-loop/`, `.commandcode/`; these remain untracked and untouched. Production Wails/Tunnel process, Agent grants, Profile ToolBindings, policy and production Task/Session database were not modified or restarted by this implementation. **Deployment and real native-session recovery are separate acceptance gates.**

## D implementation checkpoint — 2026-10-19 (host-only explicit continuation)

Scope: Native Session continuation under **original Host isolation only**. This is a bounded new Task, **not** automatic recovery of an uncertain in-flight Task. Unknown/failed/unreaped prior Tasks cannot be resumed. Persistent restricted-host private HOME remains intentionally unsupported for Resume; no Token, old ACP approval or credential carries forward.

- Agent SQLite **v15** forward migration adds an immutable child Task → parent Task → exact native SessionRef lineage, and adds separately audited Policy action `agent.task.resume` without changing earlier admissions, source ToolIDs, Profile grants or Task results. A concurrency-fenced admission transaction rechecks exact Original Task terminal proof, execution leader reap, native SessionID + Backend + Owner/Scope + Workspace + current Agent mode + Profile Agent/Workspace grants. A concurrent duplicate continuation fails. A partial ACP/Antigravity prompt write is treated as **outcome uncertain**, because some bytes could already have reached the Agent; it cannot license a retry. An explicit retry is permitted only when the preceding child **failed before any prompt dispatch** and has no unproven running process; historical `unknown`, cancelled, completed and post-prompt failures block a second continuation.
- Existing Agent facade retains `agent.delegate`, `agent.status`, `agent.result`, `agent.cancel` SourceKeys and ToolIDs. Add a **fifth** `agent.resume` canonical Tool with only bounded `{taskId, prompt}` input; the Runtime resolves opaque native SessionID internally. It returns a **new** TaskID and `continuedFromTaskId`; the original Task is never reclassified or replayed. Old Profile ToolBindings stay as-is: a Profile must expressly select the newly discovered Tool before invoking it. Agent and Profile caller ownership, current Policy action admission and workspace authorization are checked independently of the ordinary `tool.invoke` path.
- ACP v1 backend reads actual `initialize.agentCapabilities.loadSession` / `sessionCapabilities.resume` rather than using Agent brand defaults. Call exact `session/resume` when advertised; otherwise `session/load` only if advertised; no `session/new` fallback, no untrusted raw Session selector, no automatic retry after setup choices. The effective admitted cwd and immutable model/effort are rechecked; new approval coordinator and fresh Task provenance apply. Grok and Codex require runtime-negotiated support and verified local backends; no unsupported badge.
- **Antigravity Phase D is deliberately unavailable:** although the installed CLI advertises exact `--conversation <UUID>`, the headless stream does not provide a verified pre-prompt session-identity handshake. A real **no-prompt** disposable-workspace `agy` probe produced **no `init` within 5 seconds**. As a result, Mender rejects Antigravity `agent.resume` before creating a Task, launching the native CLI, or sending any prompt; backend runner independently fails closed. We did **not** ship a speculative `--conversation` launcher or an init-preflight implementation that would falsely suggest working recovery. The backend can be enabled only after a proven side-effect-free native identity check (not `--continue`/most-recent guessing).
- A real **no-prompt** `/opt/homebrew/bin/opencode` v2.0.30 ACP probe with disposable cwd and private HOME established `initialize` + `session/new`, terminated the process, and reopened the original session with a second-process `session/resume` returning **OK**. No model inference, file modifications, native credentials or Task prompt were used. This proves only the bare host-compatible ACP persistence mechanism under that temporary test environment, **not** the complete Mender production Resume call or restricted-host governed ToolSupply.
- Task `status/result` and owner UI project only the `continuedFromTaskId` lineage, not native IDs or PID. No new tool is auto-supplied to any Profile. The historical production OpenCode and Antigravity `unknown` Task rows remain untouched.
- Synthetic acceptance covers exact prior completion and reaped+native terminal evidence, multi-turn continuation, failure-before-prompt explicit retry, no post-prompt retry, atomic concurrent reservation, wrong principal/Profile, revoked AgentGrant, cross-Agent, Policy deny, restricted-host blocking, ACP capability mismatch/RPC rejection/no silent session/new, Antigravity unsupported-before-launch negative guard, v14→v15 data/audit retention, and existing four ToolID retention + fifth Tool opt-in. Full lint/race/all-package/build quality results tracked in the final acceptance note below.

**Deferred:** Real Mender-delegated model-bearing OpenCode continuation in a disposable user-approved workspace, Antigravity actual pre-prompt session proof, Grok authenticated ACP capability, Codex ACP adapter, restricted-host persistent home architecture, in-flight stream reattachment, evidence-backed historical unknown resolution, UI authoring flow and production Wails/connector refresh. No claim of those capabilities is made by synthetic fixture tests.

### D final quality checkpoint — passed (2026-10-19)

- Source commits: `aa3446f` (native continuation and fifth canonical Tool), `6273c51` (backend/support contract), `be0e9aa` (partial prompt-write uncertainty + negative tests + updated CLI/HTTP suite expectations). The new canonical tool is offered as an explicitly bindable candidate; unsupported Agent placements currently reject at admission rather than executing, and a future source-first UX iteration should prevent misleading unsupported-tool presentation.
- Full `pnpm check`: **exit 0**, local log `/tmp/mender-agent-resume-d-final.log` (Mac mini). Go full tests, `go test -race ./backend/...` (Bootstrap race 297.013s), Go lint (0 issues), plugin and SDK checks, Wails generated bindings and desktop architecture, ESLint/TypeScript, 1396 Chinese i18n strings, 47 Vitest files / 286 tests, and Web/Desktop production builds all passed. Vite large-chunk warnings are nonfatal.
- Genuine no-prompt ACP smoke: OpenCode v2.0.30 in a disposable HOME/workspace successfully opened `session/new`, exited, and re-opened that same session from another process via `session/resume`. No inference, prompt, existing workspace access or production credential copy. Antigravity v1.4.2 no-prompt headless stream produced no init in five seconds; Resume remains disabled before Task creation.
- Confirmed fault containment: pre-dispatch restore failure leaves a safely retriable failed Task only with proven no-prompt evidence; partial ACP/Antigravity stdin writes are *uncertain* rather than safely retryable. Concurrent Resume cannot mint two in-flight children; historic `unknown`, unreaped, wrong SessionID/Agent/Principal, revoked grants, unsupported isolation and missing capability are blocked. Profile authorization and original ToolIDs are unchanged; users must explicitly select and grant the fifth Tool.
- **Not deployment:** Production SQLite, configured Agents/Profiles/Policy, active Wails/Tunnel processes and historic `unknown` tasks were not modified or restarted. Real Mender-to-OpenCode **model-bearing continuation** and safe native Antigravity recovery remain unproven, and owner-facing UI continuation/unknown task reconciliation are not in this checkpoint. Repository retains preexisting untracked `.agents/`, `.pi/`, `.intent-loop/`, `.commandcode/` directories untouched.

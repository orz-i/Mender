# Mender Agent 委派阻断与 `unknown` 任务治理实机审计 — 2026-10-19

> 后续更正（2026-10-20）：这里对 Agy 原生 Resume 的不可用判断只反映当时 Mender 尚未实现安全预校验。Agy CLI v1.4.2 实际支持 Headless `--conversation`，且可在发送 Prompt 前从 `init` 确认原生会话身份；新实现见 `docs/planning/agy-headless-native-session-continuation-2026-10-20.md`。但 `unknown` 原任务的最终结果仍不可自动确认或重放。

## 结论与边界

本轮在 `/Users/mini/Coding/Mender` 真实 Wails Runtime + 已供应的 `Antigravity` Agent 下审计，不以 UI 模拟测试替代执行证据。Mender 的定位仍是工具与资源聚合、治理、供应平台，不接管 Agent Loop。**前序交付未兑现的不是“真实代码改动”，而是通过 Agy 委派完成并验证、恢复的端到端闭环。**

`unknown` 是正确的保守结果状态：超时/取消/网络中断后的真实副作用未必可知，必须保持不确定性，不能自动当作失败、成功或安全重放。需要治理的是用户无法查明原因、无终态凭据、没有正确后续操作，而不是删除 `unknown` 语义。

## 只读生产证据（2026-10-19）

1. 上轮真实委派 `atask_8e91f2b4778149dfc7a11d96705bfa5e`，Agent Antigravity，13:23:40Z 入库，13:33:40Z 结束，**刚好 600 秒**，最终 `unknown`，诊断 `Agent outcome unknown after timeout`。生命周期仅 `attempt_reserved → process_started → prompt_sent → leader_reaped`，没有 `session_ready`、`terminal_seen`、原生进度或 Session ID。最后 Worker Heartbeat 为 13:33:25Z（距终态约 15 秒），这是 Mender 自身工作线程心跳，不是 Agy 实际业务进展。
2. 同日任务 `atask_68e1f47e860eb77e6f2e9040ed3186f6`：已经持久化 Native Session，但 `load_supported=0` 和 `resume_supported=0`；最后 Agent progress 08:18:28Z，三秒后在十分钟硬超时进入 `unknown`。**即使有 Session 和新鲜进度，也不能跨该 Agent 后端取回终态。**
3. 同日另一条 `atask_87dd6eede03f0484c7866d5acaa364f4`：10 分钟 `unknown`，没有 Session。历史 8 条已结束 Agy Task 中 4 条 `completed`、4 条 `unknown`；样本仅证明该环境反复发生，不能推断所有用户或全量请求故障率。
4. Agy CLI 已检测为 `~/.local/bin/agy`；当前 Agent 是 `host`、限时 `trusted` 模式，默认模型 `gemini-3.8-flash-high`。没有这次历史任务的可靠用户级 stdout/stderr 诊断，**无法断言其发生的是身份验证、网络、模型、资源锁、文件权限还是 CLI 故障**。
5. 重新委派的独立真实 Web 研发任务见验收附录；不可将其完成情况倒灌或归因为历史任务。

## 已确认的架构限制与阻断优先级

| 等级 | 阻断/限制 | 代码证据与用户影响 |
| --- | --- | --- |
| P0 | **固定 10 分钟硬截止，跑满就 `unknown`** | `backend/internal/agent/application/service.go` `TaskTimeout = 10*time.Minute`，运行 `context.WithTimeout`，`DeadlineExceeded` 转 `TaskUnknown`，丢弃未证明的结果。普通 Web 开发可能超过 10 分钟，用户只拿到“不知道成功没”。不能以提高全局超时作为唯一解法。 |
| P0 | **恢复结果缺少后端权威凭证** | `agent_execution_attempts` 只含有界的 phase / heartbeat / native session；`antigravity/runner.go` 在 `init` 和 `step_update` 中只记录有限事件，只有 `result.SUCCESS` 才有终态。没有从已终止或掉线的 Agy 取回原 Task 最终结果的已验证能力。 |
| P0 | **Agy 不支持现有 `resume` 语义** | `backend/internal/agent/application/resume.go` 明确以 BackendAntigravity 拦截，`antigravity/runner.go` 也拒绝非空 Resume。Native Session ID 不代表续接。绝不对 `unknown` 自动 replay 原提示词；旧任务只有人工代码比对和重新规划其他任务。 |
| P1 | **工具目录仍为 Agy 暴露不可调用的 `resume`** | `delegationprovider/plugin.go` 为所有 Agent facade 固定发现 `delegate,status,result,cancel,resume` 五个工具；`resume` 的说明对 Agy 与实际能力不符，构成能力可发现性错误。应按后端已证实能力约束展示，但旧 SourceKey/ToolID 的兼容和 Profile 授权要审核。 |
| P1 | **Mender Heartbeat 可能被误解成 Native Agent 活动** | `service.go` 每 15 秒发 `worker_heartbeat`，不证明 Agent 正在输出或执行；`last_progress_at` 已入库但普通 Observation 不含独立字段，用户需要反复进详情解读 `lastObservedAt` / `lastWorkerHeartbeatAt`。应展示独立的“最后 Agent 活动”“Mender 监督仍存活”。 |
| P1 | **Agy Headless 不支持 Mender ACP 授权回调及 Task ToolSupply** | ADR-0050 和 `antigravity/runner.go` 均明确：有 ToolSupply 则启动前拒绝，`trusted` 等于向原生 Agy 自有工具授予限时无人值守权力，无 Mender ACP Approval 记录；不能声称复用了 Mender 工具授权。 |
| P1 | **前期/中期故障缺乏可归因诊断** | `antigravity/runner.go` 将 stderr 丢弃以避免凭据泄露；如果卡在 `init` 前，状态只能保留 `prompt_sent`、心跳、最终超时；无受控的原生错误类别。绝不能靠持久化全部 stderr/Prompt/推理来“修复”。 |
| P2 | **协调方无法确认交付与重派风险** | `status` 不携带受控的实际交付摘要或状态决策；老 `unknown` Task 无 result，仓库 Git 差异只能是独立线索，不是 Agent 原 Task 成功证明。未知后重复分派同一区域可能冲突或重复写入。 |

## 设计收口建议（不引入大型 Agent 调度控制面）

1. **保持 `unknown`，改进解释与操作**。任务详情用一个动作入口“核对执行状态”展开展示：用户可辨识的最后 Native 进度、监督心跳、执行期限、已回收的进程 leader、原生会话是否已记录、是否支持特定续接。不能只显示笼统 `unknown`；无确定证据就告诉用户“尚不可验证结果／请核对仓库提交”，不给“继续旧任务”的危险按钮。
2. **实时运行中的软提醒，不靠定期全局延时掩盖问题**。区分 `running + 最近 Agent 活动`、`running + 只有 Worker 心跳`、`timeout + outcome_unknown`，必要时允许显式、受界限的任务执行预算扩展，仅未来 Task 生效，单 Agent/用户治理。任务结果仍须可靠 Native `result`。
3. **根据已验证能力做 Source-first 工具供应**。Antigravity 原生会话续接没有证据时不要宣传 `resume` 可用。ACP v1/v2 的 session 恢复语义不同，不能把旧 ACP `session/resume` 或 Agy `--conversation` 猜测性调用当作恢复在途任务。保持五个旧 Canonical ToolIDs、Profile 工具显式选择和跨 Provider 访问控制，调整方式先审查。
4. **设计跨后端可验证的恢复实验**。用 disposable workspace：任务在运行时及 10 分钟之前创建可观察文件，再模拟父进程被杀、网络断开与超时，核对真实进程树、Native terminal 原始可信事件、Session 能力、隔离下的可见结果；绝不对原生产 `unknown` 任务自动重跑或把文件存在当作完成证明。
5. **短期工程流程**：把 Agy Web 任务限制到互不重叠的小切片、先回传任务 ID、同步查询 native-progress 与 Worker heartbeat、合并前检查 Git commit + diff + 类型测试。Agy 超时时停止重派同一区域，把未完成工作保留为人工可审核状态。不要自动扩大 Trusted 授权或保留原生凭据。

## 参考

- `docs/planning/agent-lifecycle-recovery-session-resume.md`（原生命周期治理：阶段 B/C/D 实施、未知保留与 Antigravity 安全不支持续接）
- `docs/architecture/adr/0050-antigravity-headless-agent-backend.md`（Agy Headless 协议和授权边界）
- ACP v1 官方 Session Setup：`https://github.com/agentclientprotocol/agent-client-protocol/blob/main/docs/protocol/v1/draft/session-setup.mdx`
- ACP v2 官方迁移：`https://github.com/agentclientprotocol/agent-client-protocol/blob/main/docs/protocol/v2/migration.mdx`

## 本轮实际 Agy 复测（终态已确认）

- **新 TaskID**：`atask_799ccfec09fbf183fc33ac290b69eb0f`，2026-10-19 15:51:32 UTC 经 Mender `agent.delegate` 真实提交给 Antigravity。任务明确限定只能修改 `packages/app/src/domains/agents/agents-page.tsx` 和 `agents.test.tsx`，与旧 `unknown` 无关。
- **Agy 已独立产生正式 Git 提交**：`e2b681f`，提交时间 **16:00:41 UTC**，即任务启动约 **9 分 09 秒**后。改动仅涉及上述两份文件：Agent 列表适配窄屏，以及两项行为测试。工作树无遗留本任务源码改动。
- **任务却被截断成 `unknown`**：16:01:24 UTC 仍观察到 Agent `progress`；16:01:32 UTC 恰好达到 10 分钟上下文截止，Mender 将任务转为 `unknown`，`diagnostic=Agent outcome unknown after timeout`，`result_text` 长度 0，事件没有 `terminal_seen`，仅 `attempt_reserved → process_started → prompt_sent → session_ready → leader_reaped`。Native Session 已持久化，但 `load_supported=0`、`resume_supported=0`；进程 leader 已回收。
- **两个结果必须独立判定**：源码文件与 Git commit **确实存在**、通过后续独立测试；但 Mender **并未获得原生终态回执**，不能把原 Task 数据库行强制改成 `completed`。这是真实的“交付已落地，但协同控制面无法确认完成”案例，不能用单纯模型失败或没执行任务解释。
- **独立验收**：协调方在子进程已退出后执行 `pnpm --filter @mender/app typecheck`、`pnpm exec vitest run packages/app/src/domains/agents/agents.test.tsx --maxWorkers=6 --reporter=dot`、`pnpm lint` 均通过；完整 `pnpm exec vitest run --maxWorkers=6 --reporter=dot` 为 **53 文件 / 323 tests passed**，`pnpm --filter @mender/web build` 通过。真实 Web Vite 下使用**隔离接口 Fixture** 的浏览器回归：390px 时 Agent 表格转为 block/list 结构、2 行数据可读、2 个详情入口保留、页面 scrollWidth=390；1440px 时仍为 Table，scrollWidth=1440；JavaScript 错误为 0。Fixture 视图不是 Wails 原生数据验收。**没有**把 Agy 原生未回传的验证结果当作已回传。
- **复现比例（仅本机历史）**：原 8 条已结束 Agy 任务 4 完成 / 4 `unknown`；加上这次新任务，已结束的 9 条为 **4 完成 / 5 `unknown`**。不能推断整体线上故障率。

### 本次复现要求的最低修复验收

1. 需能在时间预算耗尽前明确区分 **任务已原生确认完成**、**原生仍有进展但硬截止即将到来**、**超过预算且结果不明**。把“允许长任务”作为有界、用户明确的预算策略讨论；不得因有 Native 进度就无限自动续时。
2. 将 `last_progress_at`（Native）与 `last_worker_heartbeat_at`（Mender）以及 `leader_reaped_at` 分开展示。现在 `lastObservedAt` 被终结的 `leader_reaped` 更新为 16:01:32，却遮蔽了真正的最后 Native 进度 16:01:24；只看 `status` 无法还原这个重要差别。
3. Agy 已完成 Git commit，但尚未返回 `result.SUCCESS` 不能声称原生 Task 成功。可以允许操作者**只读**关联可信的工作区交付证据/检查结果并选择后续新任务；不能自动重写 Task 状态或重放旧 Prompt。
4. 确认不支持 Native `resume` 的 Agent 不再在 Source-first 工具发现中被宣称有可操作的续接能力。解决这一点须保留现有跨 Profile 与 canonical ToolID 治理边界，不能自动授权或粗暴移除历史绑定。
5. 故障注入覆盖：Native 无 `init`、Native 有 `init` 无 `result`、接近期限才出现 Native 进度、Git 交付已落地但没有 `result`、长时间仅 Worker 心跳、进程 leader 回收但子进程不明；每条都不得被误判为自动完成或可安全重试。

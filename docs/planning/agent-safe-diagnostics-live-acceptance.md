# ACP 失败诊断：部署后实机验收与双 Agent 开发

Status: **bounded live ACP stage verification and OpenCode real coding acceptance complete; Grok prompt-transport engineering reliability remains an explicit unresolved follow-up** (2026-10-18).

## Scope and safety

The user has rebuilt and reconnected Mender. Wails and Tunnel are running with the source at `b41baaf` and policy schema v3 / Agent schema v12. Existing owner Host trust grants belong to the user; this round does not enable, extend or revoke them. No historical Task data or production SQLite rows will be deleted, no compatibility adapter added, and Mender remains a tool/resource/permission supplier rather than Agent Loop.

Two explicitly expected configuration rejections exercise the **deployed** safe failure classification without any real Agent prompt or tool invocation:

- OpenCode `atask_3255ba655e547929eba8a4f37f41bd73`: failed, `Agent model or reasoning configuration failed`.
- Grok `atask_4b4933aa6a99c1b9ed6bb335dae6d153`: failed, `Agent model or reasoning configuration failed`.

Both were created through Mender external Agent delegation using an unadvertised model ID; no process prompts, native file changes or Shell calls were requested. This proves the installed Agent v12/Policy v3 Runtime returns bounded stage diagnostics, not underlying remote text, but does not prove every model/provider failure is correctly categorized.

## Genuine parallel coding acceptance

Do not substitute code reading, a failed Task with partial edits, or a coordinator-created patch for an Agent's successful delivery.

**OpenCode assignment:** narrow ACP error-message minimization in `backend/internal/agent/infrastructure/acp/failure_stage.go` and `failure_stage_test.go`. The two error types `rpcRejectedError` and `malformedOutcomeError` currently interpolate `method` into Error(), even though method is trusted on existing call paths. Harden their displayed messages to immutable constants with no method interpolation, preserve their type-based classification and errors.Is/Unwrap, add regression tests containing a deliberately injected sentinel method. Run gofmt, ACP Go package tests and targeted Go Race Detector. No Agent Git commit or other paths.

**Grok assignment:** separate application test-only implementation in `backend/internal/agent/application/task_failure_diagnostic_test.go`. Add a deterministic integration test using the existing application fixture that returns a stage-wrapped `context.DeadlineExceeded`, confirming terminal `unknown` and existing `Agent outcome unknown after timeout` precedence rather than a new failure stage. Verify no diagnostic leaks fake error payload and Task result remains empty. Run gofmt and targeted Go tests. No other files, no Git commit, no external network. Explicit `grok-4.7/high` is the previously verified working per-Task configuration; no user default changes.

Both work in disjoint files with real Mender TaskIDs. Owner independently reviews Go diffs and outputs, runs race/full gates, commits code with accurate attribution only for completed Agent deliverables. Before any production-code merge, no unexplained modifications outside the allocated scopes. If Grok cannot edit/test, preserve `failed` plus the new safe stage diagnostic rather than guessing its root cause.

## 实机委派结果和下一步边界（2026-10-18）

**部署确认。** 用户再次构建并重连后，Mac mini 上 Wails、Tunnel 及 Mender Profile 的 OpenCode/Grok 委派工具均在线。源代码基线为 `b41baaf`，schema Agent v12 / Policy v3，User-owned Host trusted 模式在原有作用域内有效（OpenCode、Grok、Codex 均由用户已有配置），本轮没有设置、延长或撤销授权。

### 实际 Task 与证据

| Backend | Task ID | 终态 | 可核对的结果 |
| --- | --- | --- | --- |
| OpenCode，故意指定不存在的模型 | `atask_3255ba655e547929eba8a4f37f41bd73` | failed | **`Agent model or reasoning configuration failed`**，没有发送实际提示词 |
| Grok，故意指定不存在的模型 | `atask_4b4933aa6a99c1b9ed6bb335dae6d153` | failed | **`Agent model or reasoning configuration failed`**，同样在配置阶段拒绝 |
| OpenCode，真实 Go 源码安全加固 | `atask_d085cffd965d06a807da5141bacb3d01` | **completed** | **read 8 / edit 7 / execute 10**，均 `allowed_once/policy`，真实修改两个限定文件，Agent 自行运行 Go package tests、Race Detector、diff check；协调者独立复验 |
| Grok，真实 Go 单测开发 | `atask_c29481d4c161bcacf0ccbee352233693` | failed | 固定诊断 **`ACP prompt transport failed`**，0 原生文件审批，没有代码修改 |
| Grok，明确不使用工具的对照任务 | `atask_d6e32b3327df186d6223ec67eff3132c` | **completed** | 返回 `GROK_TRANSPORT_CONTROL_OK`，证明简短无工具会话可完成，但不是编程交付 |

OpenCode 的真实安全修复位于 `backend/internal/agent/infrastructure/acp/failure_stage.go` 和 `failure_stage_test.go`：`rpcRejectedError.Error()` 与 `malformedOutcomeError.Error()` 不再拼接 method，改为固定无上下文短语，新增 **57 行**模拟私有方法名的泄漏回归测试。其 Agent Task `completed`，且实际 Git 修改只涉及这两个获准文件；我独立运行 `go test ./backend/internal/agent/infrastructure/acp -count=1`、定向 `go test -race`、`pnpm go:lint` 均通过。由协调者审查后提交 `253a903`，没有由 Agent 自行 Git commit。

Grok 的失败在新的 ACP 分类中被准确定位到 **prompt transport**（提示执行/传输阶段）而非 session/create/configuration；其无工具任务完成表明基础模型通信不是一直不可用。**不能据此推断**具体是模型服务、运行时断开、CLI 内部工具执行，或凭据问题。Grok 没有原生 ACP 审批记录，因此不能认定其读写操作已被治理；其真实开发仍**未通过**。不要重复盲派同样的开发任务或通过扩大 Host 权限试错。后续独立研究应使用仅暴露固定类别（例如 RPC 远端拒绝/流关闭/子进程退出）的安全观测点，而非读取或持久化原始 stderr、提示词或 bearer。

**安全与历史保证：** 五条 Task 的审计与终态由生产 SQLite 只读核对，均无待处理审批。原项目其余已跟踪文件保持不变，`.agents/`、`.pi/`、`.intent-loop/` 和 `.commandcode/` 原有目录被保留。本轮没有改变生产数据库 schema、User-owned Agent 信任状态、Profile/Tool/Provider/Workspace 配置、既有 Task 历史和远端 MCP 隧道。


## 全量质量门禁及最终交接

- **本轮实机验收按限定范围完成**：更新后的 Wails 实际返回两条正确的 `configuration` 失败诊断；OpenCode 真正完成代码编辑/测试和成功终态；Grok 的无工具调用正常，但**真实开发仍未通过**。不能将有意失败的模型配置探针算作开发失败率或正常模型故障。
- 新的真实开发提交 `253a903` 由 OpenCode 实际修改、协调者独立验证并 Git 提交，实际只涉及 `failure_stage.go` 与 `failure_stage_test.go`。测试后源码仍为固定错误消息、保留类型分类/原始一次性授权语义，没有增加一个权限入口。
- 完整规范门禁 `/tmp/mender-post-deploy-diagnostics-20261018/check.exit` **0**，日志 `/tmp/mender-post-deploy-diagnostics-20261018/check.log`：Go 全量测试/Race Detector、Go lint、Provider/Sandbox/Tunnel SDK、Wails bindings 和架构、TypeScript、ESLint、Vitest **46 文件 / 269 项测试**、简体中文 i18n **1325/1325**、Web/Desktop 构建通过。独立原生 Wails Go 可执行文件编译通过，测试二进制随即移除。Vite chunk-size 警告非阻断。
- 生产 SQLite **只读**终检：Agent schema v12 / Policy v3、3 Agent、42 条历史 Task、163 条 ACP 决策、3 个有效用户拥有的 `trusted` Host 授权（Codex、Grok、OpenCode）、0 个受治理 Tool placement，`quick_check=ok`、外键异常 0；五条验收 Task 仍待审批为 0。本轮有意正常添加五条 Task 历史，不修改已有任务内容、用户 grant、Agent 默认模型或真实项目其他源码。
- 原生 Wails 与 Mender Tunnel 由用户启动，进程和权限在本轮未被协调者重启或改写。**`253a903` 晚于已运行的 Wails 二进制**，因此新的 Error 文本固定化只经 Go 测试验证，尚未通过用户正常构建重启进入生产运行时。先前 `b41baaf` 的九阶段诊断已经通过真实部署后验收，二者要分开记录。
- 保留未解决的 **Grok ACP prompt_transport 子原因**：不可据此断言是模型故障、账户故障或工具权限不足。下一轮如需推进，应在不收集原始 stderr、提示词、凭据或敏感路径的前提下，进一步区分子进程意外退出、ACP 流关闭、JSON-RPC 解码或模型 RPC 拒绝等固定非敏感类别；并在独立一次性测试中检验具体类别。不能以无限增加 Host 授权或反复盲派任务代替故障定位。

## Closure gates

- Two deployed configuration probes return exact safe diagnostic as above.
- Real Agent code edits + actual tests + terminal `completed` are required individually.
- No credential/file content in Task `diagnostic`.
- Go race, Go lint, `pnpm check`, native Wails Go build; production DB readonly verification.
- Do not restart production Wails on the user's behalf or change its Host approval modes.

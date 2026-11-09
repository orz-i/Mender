# F4 唯一收口总控（冻结版）

> Scope freeze: **F4 不再派生 F4a/F4b/F5/新模块/新特性/候选系统。** 本文件是 F4 的唯一执行清单；Stage F 旧文档仅留历史证据。实施代码只允许消除下面三个既定门槛之一。其他发现登记为已知风险，除非会阻止验收或造成越权，不转化为开发任务。
>
> 基线：`governance/tool-ownership-hardcut`，`ef605a38`；工作树干净；2026-11-09（Mac 主机本地日期）。
>
> 现有 20 次 F4 提交完成了 typed Catalog/Profile/Policy/Agent/Sandbox/MCP 的多个候选验证；**正式 `bootstrap.NewRuntime`、menderd/Wails 启动仍使用 Provider-v2**。已验证真实 DB 迁移报告 `blocked`：54→41 tools，13 冲突（来源所有权 5、SSH 恢复 5、Placement 2、Agent Task 1，涉及 unknown 8），24 MCP 导出变更待审查。测试证据 ≠ 生产切换授权。

## F4 完成定义（只有以下 3 个门槛）

| 门槛 | 准确交付 | 可验收证据 / 否则明确 Blocked |
| --- | --- | --- |
| 1. **唯一 v3 Runtime** | 用现有 typed Store、单一 `Invocation`、真实 Provider Activation / Agent Task / Sandbox Lease / native Jobs 与 Managed MCP **组成一个正式运行时**；menderd/Wails 共用；无 test-only executor、no-op 关键后端。 | 从独立 **可丢弃** Stage E v3 数据库经正式程序入口启动；用真实外部 Provider、Agent delegate/status/result/cancel/resume、Workspace read/edit/shell、SSH Jobs（可用时）验证最小正确链路。运行时不可执行/不能证明授权的能力必须拒绝，不能谎称完成。 |
| 2. **已知数据门禁** | 仅对现有 13 冲突与 24 MCP Diff 做可审计的 owner 判定；保留旧→新 ToolID、Profile TargetScope、Policy、Workspace、Agent/SSH 的既有权限与证据。 | `assess` 真实库只读报告 + 确切冲突对照。未取得 owner 证据的项**保持 blocked**，绝不通过代码/自动删除/自动重放 unknown Task 清零；生产切换需事先获明确授权。 |
| 3. **退休旧路径 + 完整质量门禁** | 最终仅一套 Catalog/Invocation/MCP/CLI/Wails/SDK 合同；移除 Agent/Sandbox Provider Facade、Provider-keyed targets、遗留 CRUD/兼容 API、死代码；一次性 schema 迁移和备份/回滚演练。 | `pnpm check`（含全量 race）、`pnpm security:check`、Go/SQL/FTS/CLI/Wails/SDK/MCP parity、真实 Wails 验收；`git diff --check`；无旧 Facade invoke/写入/双读回退，旧程序拒绝新 schema。 |

**完成状态**分两种严格区分：

- **Code-complete / Release-blocked**：唯一 Runtime 与旧路径清理通过所有可自动验证门禁，但现有真实 DB 冲突/owner 审批或人工 Wails 未通过；不可宣称 F4 完成或生产升级。
- **F4 closed**：三个门槛全部具有证据。恢复／迁移前未获授权时不得改变生产真实数据。

## 执行路径（不构成子阶段）

1. **只做直接集成**：以 `backend/internal/bootstrap/runtime.go` 为正式入口，将已验收 typed 组件接上**一个**新 Schema Runtime；尽可能删除取代旧的 Provider Facade，而非建立旁路 `NewTypedRuntime` 长期 API。任何测试适配只能用于 fixture；不能替代最终启动。
2. **只解决确定的代码阻断**：复用现有 `NewTypedCatalog/NewTypedProfiles/NewTypedPolicies/newTypedMCPDelivery`、`sourcecatalog`、`typedlease`、`mcpserver.NewTyped`、`agentapplication.Service`、`SandboxRouter`。若这些合同无法接入，修改最低必要调用点，不引入新的抽象/Source/Task/Workspace CRUD。
3. **只跑最终链路与负面测试**：先以 dispose v3 + 真实 Bootstrap 启动验证；然后全量质量门禁。若真实 SSH/ACP 不可达，记明确外部阻断，不追加模拟服务证明或声称完整验收。
4. **最后才发布切换**：保留 Stage E `assess → stage-copy → VerifyStagedCopy` 与 live writer quiescence、backup / restore；实际 DB 在人工审批与 0 blockers 前**只读**。CLI 不添加危险 apply 自动化。
5. **原有 G/H 仍归 G/H**：不顺便做 UI 美化、新导航、插件市场重组或重复治理。仅做避免旧路径继续运行的必要契约调整。

## 不可突破的执行纪律

- 不新开阶段、不刷新“再来一轮”的任务清单；每笔提交说明：解决三个门槛的哪一个、证据、剩余阻断数是否变化。
- 已有测试适配不再每次单独附一篇“候选完成”文档，统一维护**本文件**一行结果和最后一个 `git log` 依据。
- `unknown` Task、SSH Jobs 必须先验证执行证据；禁止重试、删除、篡改、假定完成。不会为了 `assess` 变绿而扩大 Profile、Sandbox、Agent 或 Workspace 权限。
- 未证实外部 Provider/Agent Backend/SSH 真实能力时**fail closed**；不能使用 test-only 替代物冒充正式执行。
- 所有测试优先使用独立 disposable DB；严禁将候选 v3 替换真实 DB；不向远端推送、不重启真实 Wails、不更改用户运行资源，除非另有明确授权。
- 出现影响正确性的真实阻断时，可直接修复；其他建议直接拒绝纳入 F4 范围。

## 既有迁移阻断的事实分组（只读，非新工作项）

来源：`/tmp/mender-f4-live-assess-20261107.json` 与 `/tmp/mender-f4-native-managed-readonly-assess.json`；两份均为 `blocked`，54→41 Tools、**13/13 冲突未减少、24/24 MCP Diff 未减少**。

| 既有阻断 | 已核实的事实 | 唯一允许的处理 |
| --- | --- | --- |
| Source owner 5 | 集中于**一个**缺乏 ownership pin 的 resource-style Provider 与它的 4 个 orphan workspace Tools | 从已有持久引用、实际资源与 Profile/Policy 关联核实 owner；没有等价证据就保留 blocked，不能推断/强删 |
| SSH 5 | 涉及 3 个 SSH Sandbox 恢复证明均 `unverified`，其中两个旧 Job Facade 仍有 Provider→Sandbox 关联 | 先验证远端 Job/Task 原生恢复证据和归属；不能把网络不可达当作任务完成 |
| Placement 2 | 两个 Sandbox 无法从可信后端元数据证明远端根/Owner | 无可验证的 Root 与 Owner 就保持不迁移，不按字符串同名匹配 |
| Agent Task 1 | Task evidence 为 `unknown:8`、`completed:34`、`failed:33`、`cancelled:1` | 逐条保留身份和恢复证据；不自动提交、完成、取消或重跑 |
| MCP Export 24 | 全部集中于**同一个** Profile：15 个 Source Namespace 变更，9 个 breaking ToolID 变更；另有 9 个 Profile Binding 映射 | 基于原来的 Profile 限制完成 owner-review 的逐项清单和客户端更新；不做名称 alias 兼容桥 |

**阻断数量是条件，不是绩效指标。** 代码门禁可先完成；上述可核实事实未齐备时，最终状态仍为 `release-blocked`。

## 当前审计基准 / 验收记录

- [ ] 门槛 1：**未完成**。`bootstrap.NewRuntime` 仍 Provider-v2；typed Stores 与 MCP 候选分散；`menderd` 与 `apps/desktop` 仍从此启动。
- [ ] 门槛 2：**Blocked**。只读报告 `/tmp/mender-f4-native-managed-readonly-assess.json`：13 conflicts、24 MCP export diffs；真实 DB 不可自动转。
- [ ] 门槛 3：**未完成**。旧 Facade 写入路径仍存在；`pnpm security:check` 与更新 Go 依赖后的完整 `pnpm check` 均已通过。旧 Facade 与正式 v3 Runtime/Wails 实机验收仍未完成。
- Git 状态与后续实测：以每轮经验证的提交/命令为准；本清单不以新增代码量、测试量或文档长度表示完成度。

### 2026-11-09 唯一完整质量检查记录

- **`pnpm check` exit 0**：脚本/插件 SDK、架构及 Go modules、OpenAPI SDK 与 Wails Bindings drift、Go lint、常规全量 Go、**全量 `go test -race ./backend/...`**、ESLint、TypeScript、i18n **1639/1639**、Vitest **76 files / 512 tests**、Web+Desktop 构建均通过；Bootstrap Race 完整耗时 **465.918 秒**。原始日志：`/tmp/mender-f4-freeze-pnpm-check.log`，退出记录：`/tmp/mender-f4-freeze-pnpm-check.exit`。
- **`pnpm security:check` exit nonzero**（在 Go 扫描处终止，JS 审计没有运行，不可宣称通过）：`govulncheck` 发现 **3 项可达漏洞**：`GO-2026-6809`（当前 `go1.27.2`，修复于 `go1.27.3`）；`GO-2026-6795`、`GO-2026-6785`（`github.com/getkin/kin-openapi v0.145.0`，分别修复于 `v0.148.0`、`v0.149.0`）。日志：`/tmp/mender-f4-freeze-security.log`。这是**既定门槛 3 的失败证据**，不衍生新阶段；未修改依赖来隐藏失败。
- 以上是**现存 Provider-v2 运行分支的质量基线**，不能证明新的 v3 Runtime 已正式集成，不能冒充真实 Wails 原生操作验收，也不是生产迁移授权。
- **安全修复**：`27b9835f` 将共享 Go 工具链版本从 `1.27.2` 更新到 `1.27.3`，`kin-openapi` 从 `v0.145.0` 更新到 `v0.149.0`（随其修订必要的 transitive jsonschema）。`pnpm security:check` **exit 0**（8 个 Go 模块均无可达漏洞、JS 生产依赖审计无已知漏洞），`pnpm go:modules:check` **exit 0**。完整日志 `/tmp/mender-f4-security-fixed.log`、退出码 `/tmp/mender-f4-security-fixed.exit`。此结果仅消除原门槛 3 的安全失败，不改变 F4 其他阻断状态。
- **修复后全量回归**：`pnpm check` **exit 0**，包含独立插件 SDK/示例检查、Go 全量测试、`go test -race ./backend/...`（Bootstrap **446.246 秒**）、架构/lint/模块一致性、SDK/Wails bindings、i18n **1639/1639**、Vitest **76 个文件/512 个测试**和 Web/Desktop 构建；日志 `/tmp/mender-f4-fixed-pnpm-check.log`，退出码 `/tmp/mender-f4-fixed-pnpm-check.exit`。Vite 大 chunk 为构建告警，未新增 G/UI 任务。以上检查对 v2 现有启动路径有效，**不是** v3 Runtime/Wails 实机验收。

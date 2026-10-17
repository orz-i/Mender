# Phase 48 — Agent 审批中心与无人值守放行

Status: implementation and automated acceptance complete; native Wails owner GUI click acceptance pending macOS unlock (2026-10-17). Authority: [ADR-0053](../architecture/adr/0053-agent-approval-inbox-and-scoped-trust.md).

1. Docs/ADR and paused cleanup worktree fingerprint; commit only new docs.
2. Agent v12 policy row, atomic automatic approval audit, owner-only APIs, bounded pending list and negative tests. One full cut; no compatibility bridge.
3. Native Wails SDK/client, focused Agent permission settings and global approvals inbox with pending indicator. Default-off, no form wall on inventory, Chinese and English.
4. Synthetic ACP read/read/unreviewable/trusted/revoke and full quality gates, isolated DB and native UI acceptance. Do not silently change user Agent settings. Return to cleanup after owner acceptance.

Existing unfinished provider-v1 cleanup files are intentionally excluded from Phase 48 commits.

## 实施与自动化验收记录（2026-10-17）

- 完整质量门禁：最终 canonical pnpm check 返回 exit 0（收据 /tmp/mender-phase48-approval-final/check3.exit；详细日志 /tmp/mender-phase48-approval-final/check3.log），包含全部 Go 测试及 Race Detector、Go lint、OpenAPI/SDK/插件校验、Wails bindings/architecture、TypeScript、ESLint、46 个 Vitest 文件 / 267 项测试、i18n 1309/1309，以及 Web/Desktop 生产构建。较大的 Vite chunks 提示为非阻断建议；最新原生 Wails Go 可执行文件也已单独编译通过。
- 入口回归提交 ecb9ea3：真实应用路由下验证本地所有者能从顶栏待审批数量直达审批中心，并直接看到跨 Agent 请求，无需进入 Agent 详情或 Task；相关定向前端测试 12/12 通过。
- 后端提交 1a537a2：Agent v12 默认拒绝自动审批；逐次审批 / 工作区读取自动放行 / 24 小时显式信任；原始 AgentGrant、Profile 和 Policy 重新核对，SQLite 原子检查 Task generation、取消、工作区以及当前授权；每次实际允许仍返回 ACP allow_once 并生成不可变审批记录，绝不选择 allow_always。
- 前端提交 6659eba：独立审批中心 /approvals、顶部待处理数字提醒、跨智能体待审队列、按 Agent 与工作区设置授权。Host 高风险放行必须明确勾选风险确认；Web/远端 MCP 不能使用这些 owner-only 接口。组件测试五项通过，TypeScript/i18n 通过。
- 修复提交 cf7d271：客户端不通过读取浏览器当前时间判定信任有效性，由服务端返回生效模式，避免 React render purity 报错及本地时钟偏差。
- 安全审计提交 7422bee、299bb7d：新增 SQLite 不可修改的授权模式变更事件。授权设置、更新、撤销留下模式、AgentID、精确工作区及有效期，不持久化文件内容、凭据或原始 Agent 输入。Profile 原始授权在任务执行中撤销后，后续自动读取被拒绝。对应定向测试、迁移测试和 Go lint 通过。
- 使用 Mac 上一致性生产数据库的独立副本执行了 v11→v12 迁移。原有 58 张保护表的行数与 SHA-256 指纹比对中仅 schema-migration 表增加版本 12。保留 3 个 Agent、17 条 Task、16 条历史审批；新授权记录 0、新授权变更事件 0；quick_check=ok、外键异常为 0。真实生产库没有被迁移或修改。
- 独立签名 Wails QA 包已经构建并成功启动，只访问临时 v12 SQLite 与临时 Agent。但 Mac 被锁屏（登录界面），无法在未经解锁的情况下对原生 WebView 执行 GUI 点击验收；没有尝试绕过密码。QA 进程已单独停止，生产 Wails 与隧道仍运行。前端测试与 Wails owner API 的自动化通过**不等于**真实 GUI 操作完成。

### 待所有者实机验收

用户解锁 macOS 后，应使用**最新代码重新构建的独立 Wails 包和新的一次性测试数据库**验证：审批中心导航及顶部铃铛、跨 Agent 待处理数量、逐次批准和拒绝、工作区读取放行、Host 信任风险确认、保存后释放已等待请求、刷新持久化与撤销。旧的临时 v12 QA 库生成于补充授权事件表之前，不能重新作为最新版验收库使用。真实用户生产部署需正常构建重启，不做未授权的生产配置写入。

### 边界与 Phase 47 暂停状态

- Host ACP 原生 read/edit/execute 放行是**协作式权限**，不能宣称限制了宿主用户文件系统或网络。受治理的项目读写应使用 restricted-host 及 Mender 的工具执行链。
- 原提交 Principal 的 Policy 在写入之前重新评估；Task、grant 与工作区等条件在 SQLite 中检查。跨领域 Policy 在这两个阶段之间同步改变的严格原子性尚未证明，不能对外宣传为全局不可竞争的策略撤销。
- Phase 47 Provider-v1 清理暂不实施。待恢复的补丁安全保存于 Mac 私有临时路径 /tmp/mender-phase47-paused-before-approval.patch 以及 /tmp/mender-phase48-paused-phase47-worktree.patch，均未混入 Phase 48 的提交或生产数据。

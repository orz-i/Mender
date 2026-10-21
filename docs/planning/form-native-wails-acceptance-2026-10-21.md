# Mender 表单治理：重建后 Wails 原生验收（2026-10-21）

## 环境与安全边界

- 仓库：`/Users/mini/Coding/Mender`；代码基线 `d11da70`，Mac Wails Desktop 重新构建于 2026-10-21 19:12:10。
- Desktop 原生进程 PID 94574，原生 WebKit 窗口 Mender（约 1239×851）；Wails Vite `127.0.0.1:9245`，MCP `127.0.0.1:8765`。
- 采用本机 System Events / AXUIElement **仅浏览、点击 UI、输入临时草稿、展开并取消**，并以 `screencapture` 捕获真实屏幕。不是 Chrome fixture 或 DOM 模拟。
- 生产数据库只用 SQLite `mode=ro` 读取基本计数和 `PRAGMA quick_check`；不能为验收创建任何真实 Agent Workspace、授权、Policy Rule、Runtime 修改或 Agent Task。
- 生产数据库基线：profiles=1、agents=4、agent_workspaces=0、agent_workspace_grants=0、policy_rules=0、agent_tasks=75、quick_check=ok。因为没有额外 Agent Workspace，**真实 Profile grant 选择下拉框不能非破坏性打开**，仅能回归它的测试和隔离 UI，不伪称实机 PASS。

## 重建后逐项证据（代码变更前）

| 路径 | 观察结果 | 证据 | 判定 |
| --- | --- | --- | --- |
| Agent → OpenCode → 工作区 → 添加工作区 | 核心字段只有别名、目录，可选本机路径；未填不可确认，底部取消/确认完整；取消后无记录 | `/tmp/mender-native-accept-20261021-04-add-workspace.png` | PASS（草稿/取消） |
| Runtime → 运行时预算 → 编辑 | 8 个数值字段、紧凑范围、固定 Footer、按需字段说明；点击信息按钮可看具体用途，文字未挤占输入区；取消不修改 | `/tmp/mender-native-accept-20261021-06-runtime-budget.png`、`07-runtime-help.png` | PASS（读取与取消）；真实失败提交未执行 |
| Runtime → 进程隔离 → 编辑 | 三个授权隔离字段、Host 风险解释及固定 Footer 可见，后端原值保持 | `/tmp/mender-native-accept-20261021-08-isolation.png` | **P2 显示问题**：闭合态选择框仍显示内部值 `restricted_host`/`host`，而非已翻译的“受限主机”“主机” |
| Policy → 添加规则 → 输入临时名称 → 下一页（范围/目标）→ 展开技术主体引用 → 取消 | 原生页面显示一个步骤提示条而非两行同级标题；精确 Principal 字段默认折叠但持续显示“留空表示不限制主体引用”；展开后技术输入/帮助可找到；底部操作固定；未创建 Policy Rule | `/tmp/mender-native-accept-20261021-11-policy-scope.png`、`12-policy-expanded.png` | PASS（只读草稿） |
| 审批 → 无人值守授权 → Antigravity → YOLO（仅未提交草稿）→ 取消 | Host/YOLO 24h 风险立即显露，必须额外勾选风险确认才能保存；本轮不执行保存、不改变旧授权 | `/tmp/mender-native-accept-20261021-15-approval-antigravity.png`、`16-approval-yolo-risk.png` | PASS（风险呈现）；未执行授权 |

## 定向修复范围

Runtime 的 `IsolationField` 使用共享 Base UI `SelectValue` 时不传显示文本，Wails 的实际闭合态回退为 raw API enum。保留 API 数值 `host` / `restricted_host`、选项可用性、Host 提权显式授权和工具权限边界，仅为当前 SelectValue 提供本地化可见文本。新增中文显示的 React 回归验证，并在正在运行的 Wails Dev/HMR 原生窗口只读复核。如果热更新未实际生效则如实标记，避免把浏览器 fixture 当成原生验收。

不修改 `packages/ui` 通用 Select 行为、不做新模式设计、不更改配置存储或权限。实施后重新检查无生产写入和版本对应关系。

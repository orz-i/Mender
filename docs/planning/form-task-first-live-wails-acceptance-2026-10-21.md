# Mender 表单任务优先治理 — Wails Desktop 重建后实机验收（2026-10-21）

## 基线与判定

- 仓库 `/Users/mini/Coding/Mender` 的 `main` 处于提交 `3224852`，包含本阶段的 `2a8cf0f`（设计合同）、`bbc939a`（11 个前端和测试文件）、`3224852`（隔离 QA 记录）。
- Mac 实际重新构建：`apps/desktop/bin/mender-desktop.dev.app/Contents/MacOS/mender-desktop` 修改时间 **2026-10-21 15:53:55**；运行进程 PID **98675**，Wails `wails3 dev` PID **98475**，前端 Vite `:9245` PID **98654**；当前运行窗口的中文管理页面来自新的 Desktop，而非过去的浏览器 Fixture 或旧 `:5174` 实例。
- 实际 `default` Profile 32 个 ToolBinding，MCP `http://127.0.0.1:8765/mcp` 正在由 PID 98675 监听，Tunnel 客户端已存在。当前 UI 读取真实管理数据，不通过浏览器 Mock。
- **结论：条件验收通过（functional acceptance with UX follow-ups）**。凭据/隧道/Server/Sandbox 本轮新入口及既有安全确认未见功能回归；发现一项 P1 普通中文用户任务阻碍与一项 P2 可预见端口冲突体验，Docker/Daytona 实机表单受当前 Runtime 不可用阻断，不宣称全流程完成。

## 实际操作路径和结果

| 步骤 | 真实原生操作 | 结果 | 证据 / 局限 |
| --- | --- | --- | --- |
| 1 | 左侧「连接与供应 → 服务器 → 启动服务器」 | **PASS**：唯一 Profile `default` 可被选中，显示 32 个工具；本地监听地址 `127.0.0.1:8765` 有摘要，高级设置展开出现完整可编辑地址和 Streamable HTTP/stdin 指南，弹窗底部取消/启动按钮可见；点**取消**退出 | `/tmp/mender-form-live-20261021-server-dialog.png`, `server-advanced.png` |
| 2 | 「隧道 → 添加隧道 → OpenAI Secure MCP Tunnel」 | **PASS**：未自动选中生产已有 Credential（共 3 个），明确留在「选择凭据」，创建按钮灰置；名称、外部 Tunnel ID、凭据选择与来源说明均可见；点**取消**退出 | `/tmp/mender-form-live-20261021-tunnel-dialog.png`；未对已配置实际 Tunnel 重新创建 |
| 3 | 「高级管理 → 凭据 → 添加凭据」 | **PASS**：原生密钥环默认仅展示来源、名称、机密；Namespace/内部 Name/Reference 位于高级设置，首屏创建按钮在必填缺失时禁用；点取消时草稿关闭；已有 3 条凭据没有被修改 | `/tmp/mender-form-live-20261021-credential-dialog.png` |
| 4 | 凭据来源选择 OAuth → 展开高级 OAuth 选项 | **PASS**：MCP endpoint 必填在主流程；client ID 和 scopes 可选且可展开；不在 OAuth 主流程索取密钥文本；切换来源至 SSH 走原生控件 | `/tmp/mender-form-live-20261021-credential-oauth-advanced.png`；**没有真实授权** |
| 5 | 凭据来源选择 SSH 私钥 | **PASS**：名称、选择文件、手动粘贴私钥、可选口令与钥匙串安全说明完整保留；用户未填写任何真实密钥；点取消 | `/tmp/mender-form-live-20261021-credential-ssh.png`；解析器/密钥环写入仅由回归测试覆盖 |
| 6 | 「沙盒 → 创建沙盒」后端选择 | **PASS**：本地和 SSH 可用；Docker 提示引擎不可用，Daytona 提示实验性功能未开启，未误将不可用后端呈现为可创建 | `/tmp/mender-form-live-20261021-sandbox-backends.png` |
| 7 | 「SSH 远程沙盒」配置 | **PASS**：host、端口（22）、账户、远程目录、SSH 私钥凭据与自动 known_hosts/SHA256 首次确认以及 root/非隔离风险说明可见；页面 Page Down 后「返回/核对」动作可完整显示 | `/tmp/mender-form-live-20261021-sandbox-ssh-config.png`、`ssh-bottom.png`；没有连接远程主机 |
| 8 | 「本地工作区」填写**仅作草稿**的 Name、路径、选择「主机（未隔离）」→「核对」 | **PASS**：Review 实际显示草稿 Name、`/Users/mini/Coding/Mender` 目录、Backend 与「未隔离」级别，警告指出本机用户的文件和进程权限；**绝不点击创建**，由返回列表结束 | `/tmp/mender-form-live-20261021-sandbox-local-review.png` |
| 9 | 前后核对本地 SQLite（`mode=ro`） | **PASS**：`credentials=3`、`tunnels=1`、`sandboxes=7`、`profiles=1`、`profile_tools=32`、`agent_tasks=75` 均未改变；`default` revision = **10**，`PRAGMA quick_check=ok` | 未运行对生产有写入的 CREATE/START/STOP/DELETE/GRANT/APPROVE/RESUME |

## 实机确认的待治理 UX 问题

### P1 — 中文显示名称依赖手工英文内部 Name

- **路径**：凭据 → 添加 → 默认 Native 类型 → 用户在「名称」输入「项目密钥」。
- **可观察现象**：表单立刻提示「无法从该名称生成有效标识。请展开高级设置，输入简短的英文字母名称」，高级设置中的内部 Name 非法，无法仅依赖中文显示名称完成创建。Wails 原生 UI 的错误、字段状态和标识预览均已观测。随后手工将内部 Name 改为 `project-key`，预览正常恢复为 `credential://personal/project-key`；没有填写或保存密钥。
- **证据**：`/tmp/mender-form-live-20261021-credential-chinese-recheck.png`、`credential-cn-advanced.png`、`credential-ascii-resolved.png`；源码 `packages/app/src/domains/credentials/credentials-page.tsx` 的 `canonicalCredentialPart()` / `validCredentialPart()`。
- **整改建议**：保留后端 ASCII 身份契约和用户高级自定义，但对 Unicode 可见名称提供**稳定、合法且碰撞安全**的自动技术标识生成；高级栏清楚展示并可改；防止创建后编辑显示名时无意改变已确认标识。需要定向测试中文、日文、emoji、拉丁混排、重复名称、超长输入与 OAuth 重授权身份保持。不要扩大凭据授权范围。

### P2 — 服务器添加时固定默认端口已被当前服务器占用

- **路径**：已有 `default` MCP Server 运行于 `127.0.0.1:8765` → 点击「启动服务器」。
- **可观察现象**：地址摘要仍默认 `127.0.0.1:8765`，按钮可用，未提示它正被当前 Mender 监听器占用；`lsof -nP -iTCP:8765 -sTCP:LISTEN` 证实该监听存在且 PID 是当前 Wails。**没有实际提交启动**，因此不能冒称已复现后端报错；但前端确实缺少可预见冲突提示。
- **整改建议**：根据已有 Server 清单给出明确的占用提示和可选空闲/OS 分配端口；仍让用户可以修改固定端口，不要偷偷改现有 Server 的 Profile 绑定、Tunnel、Endpoint 或权限。若后端已拒绝同一 Profile 重复启动，优先呈现现存 Server 的管理入口。验证一服务器/多个服务器及端口被外部进程占用场景。

### 低优先级布局观察

当前 Wails 窗口初始打开 SSH 配置时底部「返回/核对」处于折叠线以下，但经 Page Down 可以正常滚动到操作区，**不构成不可访问故障**。可单独考虑长向导的底部操作区持续可见；不得因此移除安全说明或审查步骤。

## 自动化和安全结论

- **重新运行** `pnpm lint && pnpm typecheck && pnpm test && pnpm build` 退出码 **0**：53 个 Vitest 文件、329 项测试全通过，i18n 1450/1450，Web/Desktop frontend 均正常构建，仅保留既有 Vite 大分块警告。
- 原生 Wails 与隔离 Web QA 是两种不同证据，**本次未使用 Fixture 模拟上述原生成功状态**；本次原生没有真实网络授权和创建。
- 当前 Runtime 的 Docker Engine 不可用、Daytona 未开启，**Docker/Daytona「资源限制」折叠内容和真实创建的原生体验本轮尚未覆盖**；只可引用此前的前端定向测试，不将其标为实机 PASS。
- 390px 响应式检查此前在隔离 Web 完成，本次 Wails 原生窗口没有 390px 尺寸；屏幕阅读器、完整键盘 Tab 顺序/焦点返回/缩放与高对比度没有全覆盖，不能宣称 WCAG AA 完整合规。
- 当前代码 HEAD/运行环境不变；没有修改源代码，亦未触发 Agent Delegate。现有四个未跟踪目录 `.agents/`、`.commandcode/`、`.intent-loop/`、`.pi/` 不入库。

**验收处置**：本轮基础表单治理可保留；在下一小切片中分别收口 P1 中文身份生成和 P2 默认监听地址提示，重新构建后做定向原生复核，禁止将其顺便扩张为后端资源或权限重构。

# Mender 表单治理：Skill 部署目标显式选择与错误恢复（2026-10-21）

## 范围与可复核证据

沿用 `form-contextual-help-density-2026-10-21.md`、`form-native-wails-acceptance-2026-10-21.md` 的任务优先和安全约束。本轮优先修复 **Skill 部署**真实可观察的表单缺陷，不调整 Provider、Skill/Deployment 实体、API、授权或 Agent Loop。

当前基线 `40c963e` 在运行中的 Wails Desktop 原生页面「技能 → 部署 mender-tools」：
- 打开「部署技能」时，目标已默认选中「Agents 项目」，`部署` 立即可按，不需要用户明确选择文件系统写入目标。
- 原生对话框底部出现两个 Close，缺少语义清楚的取消按钮；描述首屏展示较长安装标识。
- `packages/app/src/domains/skills/skills-page.tsx` 的 `deploy()` 捕捉错误后仅设置页面级 `error`，对话框保持覆盖，错误不在当前表单中可见；再次打开另一 Skill 时，`deploymentTarget` 沿用上次选择。
- 两种目标范围不是等价操作：Desktop 的 `chooseAndDeploySkill` 在 `*-project` 类别选择目录，`*-user` 类别直接写用户级目录（服务端 `backend/internal/skill/domain/deployment.go`）。因此不可静默选中/复用目标。

原生证据：`/tmp/mender-skill-deploy-native-pre-20261021.png`；AX 树确认 `技能部署目标=Agents 项目`、部署按钮可用和两个 Close。只打开/截图，不执行部署，不写磁盘。

## 不变量

1. 打开每个 Skill 的部署表单都要求**明确选择一个目标**。保留全部五种目标：Agents/GitHub/Claude 项目、Agents/Copilot 用户。不得自动写入某个默认项目、用户目录或沿用上一次选择。
2. 目标选择前部署按钮禁用；选择后显示简短的可理解作用范围（项目类后续需选目录，用户类会影响本机用户范围），不展示所有内部部署实现。完整 Skill 安装标识仍可按需查到；不是另造“简单模式”。
3. 部署后端失败保持弹窗打开、目标选择和用户输入不丢失，错误在表单内可读；不包含任意 Secret 或敏感凭据。Wails 原生选目录取消返回 `null` 不是部署成功；不产生部署记录。
4. 仅在部署成功后关闭弹窗/刷新列表；忙碌时禁用重复提交/取消/ESC 关闭；空或非法目标不可传给 `deploySkill`。
5. 恢复原有源/目标身份、权限、插件安全边界；不改变 Wails/Go Deployment API、Project Root 选择器、现有 Skill 安装或部署存储。
6. 复用现有 `DialogBody` / `DialogFooter` / `Button` / `t`；保持一个 Footer Cancel 和一个主操作，关闭 X 属于 Dialog 内容的已有功能。完整中文翻译及键盘/读屏标签对应。
7. 实际 Wails 验收仅允许打开、选择、取消，不调用真实 Deploy、Undeploy、Install、Trust 或删除。原生和隔离浏览器/组件测试分别标注。

## 交付与验收

- D0：记录原生问题和约束，独立提交。
- D1：Skill Deploy UI 定向修改、定向回归测试（未选择、不同目标、取消、后端失败/重试、原生文件选择取消、再次打开不同 Skill、Busy）。
- D2：定向 Vitest + TS/i18n/Lint、全量测试和 Web/Wails frontend build。
- D3：Wails Dev/HMR 原生确认未选中、禁用态、选择后语义提示和取消，桌面 & 390px 隔离 Web 视觉测试，读数据库 `mode=ro` 核对无生产写入，记录不可覆盖项目。全程分段提交、保护现有未跟踪 .agents/.pi/.commandcode/.intent-loop 目录。

参照：
- [GOV.UK 表单错误指导](https://design-system.service.gov.uk/components/error-message/)：错误应对应相关字段且不丢失已输入内容。
- [NN/g 确认对话框设计](https://www.nngroup.com/articles/confirmation-dialog/)：有实质作用的操作要让用户清晰理解并明确决定目标，避免默认肯定答案。

## 实施、回归与验收记录

### 分段提交

- `b04a3c7`：上述设计/安全约束先行。
- `3512870`：Skill 部署必须明确选择五种目标之一；每次打开重置选择与错误；区分项目级（随后弹出项目目录选择）与用户级（当前系统账户的 Skill 目录）；打开表单的身份卡显示名称/版本/来源，技术安装 ID 可展开；当前表单内渲染错误，失败保留选择并允许重试；原生文件选择器取消（`null`）不伪装成功。采用有 Header、可滚动正文和固定 Footer 的 Dialog，一处取消按钮及一处部署按钮，忙碌时禁用重复提交和关闭。
- `5c613b5`：将“安装 ID”技术折叠项移到核心“部署目标”选择之后，使打开表单时优先聚焦真正要输入的字段。显示内容和部署语义不变。

### 质量门禁

最终执行：
`pnpm lint && pnpm typecheck && pnpm i18n:check && pnpm exec vitest run --maxWorkers=6 --reporter=dot && pnpm build`

**退出码 0**：Vitest 57 文件、359 项全部通过；i18n **1470/1470**；Web / Wails Desktop frontend 构建成功。记录：`/tmp/mender-skill-form-final-20261021.log`。保留 Vite 既有大 chunk 非阻断提示。定向回归覆盖默认不选、用户/项目范围、失败重试、项目目录对话框取消、busy 禁止取消/提交、中文提示和跨 Skill 重置。

### 实机与渲染证据

1. **Wails 原生 Dev/HMR**：`mender-desktop` PID 13398，打开本机已有内置 Skill `mender-tools` 的“部署技能”弹窗。修复前 AX `技能部署目标=Agents 项目` 且“部署”可用、两个 Close。修复后 AX `技能部署目标=选择部署目标` 且 `部署 enabled=0`；五种目标完整保留。人为选择“Agents 用户”后 AX `部署 enabled=1` 且页面显示“部署到当前用户：此技能可供当前账户使用，不局限于单个项目”；点击**取消**，重新打开时恢复未选择状态（`部署 enabled=0`）。没有调用真实部署、未进入文件系统写入。证据：
   - `/tmp/mender-skill-deploy-native-pre-20261021.png`
   - `/tmp/mender-skill-deploy-native-post-20261021.png`
   - `/tmp/mender-skill-deploy-native-selected-20261021.png`
2. **隔离 Chromium/Playwright**：本机现成 Google Chrome 和 Playwright，不新装依赖。对 Vite `127.0.0.1:5174` 的 `/src/main.tsx` 仅在浏览器网络层注入失败模拟部署函数，并将 `/api/v1` 替换成只读 fixture；未修改真实 Web/Skill 运行代码或持久化存储。`/tmp/mender-skill-deploy-visual-qa.mjs`。桌面 **1280×800** 默认弹窗约 512×331.5px；390×844 默认弹窗约 358×375.5px，模拟失败后约 358×467.5px，底部按钮始终在视口内，`documentWidth=390`、没有应用脚本错误或任何 HTTP 写入请求。失败后当前表单内准确显示“QA simulated destination unavailable”，`agents-user` 值仍保留：
   - `/tmp/mender-skill-qa-1280-default.png`
   - `/tmp/mender-skill-qa-390-default.png`
   - `/tmp/mender-skill-qa-390-failure.png`
3. **焦点及稳定性**：移位前 Wails 新弹窗会首先聚焦技术“安装 ID” disclosure；移位后 Chromium 390px 截图显示目标选择器为初始聚焦，技术标识仍可通过键盘展开。后续远端 macOS GUI 会话的辅助功能查询/系统截图出现无窗口/黑屏，因此未把第二次 Wails 截图记作已完成的原生完整焦点链路验收。

### 安全与范围

只读打开生产 SQLite 数据库（`mode=ro`）校验：`skill_installations=0`（内置 Skill 非普通安装记录）、`skill_deployments=0`、`agent_workspaces=0`、`policy_rules=0`、`agent_tasks=75`，`PRAGMA quick_check=ok`，同前基线。未部署或卸载 Skill、未触碰文件系统目标目录、未更改权限。现有四个未跟踪 `.agents/`、`.commandcode/`、`.intent-loop/`、`.pi/` 目录保持原样，不纳入提交。

**未验证**：真实文件系统目标写入成功/失败、Wails 原生目录选择器实际取消、真实磁盘占用/权限失败、完整 VoiceOver 与全部键盘 Tab 路径。相关语义已有组件测试和隔离模拟失败覆盖，但不可冒充原生端到端测试。新源码已在运行中的 Wails Vite/HMR 查看；最终发行二进制打包仍需下次正常构建。

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

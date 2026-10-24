# Mender 工具来源与 Agent 列表信息密度收敛（2026-10-24）

## 实际来源与范围

承接 `ux-native-audit-2026-10-24.md` 的下一轮 P2。Mender Desktop 原生 Wails 正在运行；上一轮已在真实环境捕获工具目录与 Agent 列表。当前代码路径：

- `packages/app/src/domains/providers/providers-page.tsx`：无描述服务回退为相同的“查看这个服务提供的工具”；`toolCount = 0` 时仍让“查看工具”优先。
- `packages/app/src/domains/agents/agents-page.tsx`：整列显示绝对 `workspaceRoot`，多行同样的长路径显著重复。

已检查 `AgentDetailPage` 继续提供**完整工作区路径**；现有来源标题可进入工具详情，非内置来源的“操作”包含同步/编辑/删除，高影响行为需要继续显式触发。

## 本轮用户意图与不变量

普通用户能从来源名称、工具计数和一条明确操作判断下一步，而不是反复阅读通用文案；Agent 列表应优先看名称、后端、状态、工作区区分信息。

1. 已有来源描述：保留用户或插件提供的原文（不伪造翻译，不对后端类型编造功能）。
2. 无来源描述且已有工具：不再插入重复的通用提示，也不强制占据空白高度。
3. 零工具来源：展示原项目现成的“此服务尚未发现任何工具”；重点入口由“查看工具”改为“管理”（已知 Agent/Sandbox 归属）或“详情”（其他来源）；工具详情仍可通过来源标题访问，菜单里的同步/编辑/删除操作保持完整，不能自动触发同步。
4. 有工具来源：维持“查看工具”主要入口，并保持原有管理/详情/操作能力。
5. Agent 工作区：列表仅显示短路径尾部（跨 POSIX / Windows 分隔符），详情页继续显示完整 `workspaceRoot`，列表的 `title` 与辅助名称也保留完整路径；按完整路径搜索仍然生效。
6. 不改 Provider 工具可用性判定、CLI 探测、Profile 权限、Agent 任务、路由、API 或实体状态；不要把 `toolCount = 0` 误当作同步失败。
7. 空态 / 出错 / 加载互斥行为沿用原有机制。移动端操作不能仅靠鼠标悬停定位。

UX 依据：简短、直接的界面文案优于靠大段说明解释操作（https://www.gov.uk/service-manual/design/writing-for-user-interfaces）；交互目标尽量符合 WCAG 2.2 2.5.8（https://www.w3.org/TR/wcag/#target-size-minimum），但完整读屏符合性须另行验收。

## 验收

- 单个和混合多来源：0/非0工具数，描述存在/缺失，自主管理与 Agent/Sandbox 管理；卡片名称、主/次操作与工具目录路由功能保存。
- Agent 多种路径：长 POSIX / Windows、根路径、相同尾目录、空值；全路径搜索、行内标题与详情完整路径不变。
- 中文文案使用现有字典，不新增“模式”或冗余翻译；lint / TypeScript / i18n / Vitest 全量 / Web build。
- 优先原生实机浏览；如果 macOS 辅助功能无法操控已运行窗口，必须明确说明截图 / 点击的证据限制，不能把 jsdom 视作实机。

不涉及对真实运行来源做同步/重置/删除、启动 Agent 任务或变更 Profile 绑定。

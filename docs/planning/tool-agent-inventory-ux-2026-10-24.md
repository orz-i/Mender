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

## 执行与实机验收结果（2026-10-24）

### 版本

- `de74d7f`：本轮意图与行为契约（文档先行）。
- `f1dccd4`：工具服务卡片、Agent 路径预览及其回归测试。
- 本段验收记录作为独立文档提交。

### 原生 Wails 验证

使用已运行的 Desktop 实例（Wails Dev Frontend HMR，`9245`），通过 macOS 系统辅助功能的真实 `AXLink` / `AXButton` 导航，未以静态 HTML 或空数据 Web Fixture 冒充真实管理页。截图仅在本机临时路径 `/tmp/mender-ux-20261024-r2/`，不加入仓库。

| 步骤 | 画面/行为及结论 | 截图 |
| --- | --- | --- |
| 1. 工具目录 | 真实 Agent、AnySearch、Keling、Rocky 等来源渲染正常。有工具项主要入口仍为“查看工具”。Keling 明确显示 0 工具、未发现工具提示，主入口变为“详情”，原有“操作”仍在。 | `05-tools-native-current.png` |
| 2. 零工具来源详情 | 在 Keling 点“详情”，查看名称、Provider ID、连接、0 个同步工具、日期等；与零工具状态一致，原始配置未被更改。中文底部关闭按钮显示“关闭”，退出正常。 | `07-zero-tools-details.png` / `08-details-localized.png` |
| 3. Agent 列表 | 四项 Agent 的 CLI 探测状态仍正常；列表只展示 `…/Coding/Mender`，保留清楚的名称、后端和查看入口。 | `06-agents-native-current.png` |
| 4. Agent 详情 | 点“查看”后“工作目录”仍展示完整 `/Users/mini/Coding/Mender`；没有改变 Agent 存储数据。 | `09-agent-detail-full-path.png` |
| 5. 960px 窄桌面窗口 | 改为有文字的导航抽屉，进入 Agent 列表与工具来源页面。列表与卡片均正常排布；测试后窗口恢复原尺寸和位置。 | `13-mobile-nav.png` / `14-agents-narrow-list.png` / `15-tools-narrow-list.png` |

### 自动验证

- `pnpm exec vitest run --maxWorkers=6 --reporter=dot`：**68 个文件、439 项通过**。
- `pnpm i18n:check`：1577 项必需文案与中文映射覆盖通过；删除了已经不使用的重复通用描述翻译。
- `pnpm lint`、`pnpm --filter @mender/app typecheck`、`pnpm --filter @mender/web build`、`git diff --check` 通过。
- 测试保留并新增了 0 / 非 0 工具数量、来源描述、Owner 管理链接、同步/编辑菜单、Agent POSIX 和 Windows 路径、原路径搜索、详情、中文按钮等行为验证。
- 构建仍提示大于 500KB 的分包优化建议；Vitest 的历史模拟路由与旧 Audit 重复 key 测试中仍有已知警告，但无失败。

### 已知边界

1. **没有** 对真实 MCP Provider 执行同步/删除，没有开始 Agent Task，也没有更改工作区、权限、数据或业务生命周期。
2. Wails 原生桌面设定的最小宽度为约 960px，故 **390px 真实存量数据设备级验证仍未进行**；这不能据 960px 的截图视作已经验证手机屏幕。
3. 真实 Provider 返回的英文 `description` 如“Managed Agent Delegation tools for ...”仍直接显示，不擅自伪造中文。后续可调查多语言元数据契约或仅在内置来源引入可信本地化。
4. 本轮只收敛两个列表与零工具详情退出，未覆盖所有管理页面的读屏/键盘焦点/缩放，也不宣称整体 WCAG 2.2 AA 合规。
5. 本次改动只在桌面 Wails DEV 环境即时验证，发布版需要重新构建和重新启动后才包含新前端。

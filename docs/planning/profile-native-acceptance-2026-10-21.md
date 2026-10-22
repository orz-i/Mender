# Profile Wails 实机验收（2026-10-21）

## 验收环境

- 设备：Mac mini，macOS 27.0.1
- 桌面：Wails v3 开发客户端（`mender-desktop.dev.app`），进程 PID 90773，Vite/HMR 端口 9245
- 工作区：`/Users/mini/Coding/Mender`
- 页面：共享 `packages/app`，桌面 HashRouter
- 视口：1239 × 851 与客户端最小窗口 960 × 680
- 证据：本机 `/tmp/mender-native-profile-*.png`、Accessibility 树、Vitest 与构建日志

## 真实设备操作路径

1. 服务器页面观察现有 `default` Profile 所对应的运行中 MCP 服务，再打开侧栏「配置档」。
2. Profile 列表：显示现有记录、暴露模式、工具数与 Tunnel；「查看详情」进入独立详情页。
3. 详情页：概览/工具切换，先查看来源再钻取单个来源工具；返回来源与返回列表均有效。
4. 新增页：独立字段（Slug、显示名称、模式）与本地校验；检查初始按钮禁用后取消，**未提交新的真实 Profile**。
5. 编辑页：标识/显示名称/模式只读，Tunnel 可选；按来源管理现有 ToolBinding。
6. 来源选择：实际来源加载与每个 ToolID 的选择状态正确；对一个已选工具临时取消后显示 4→3，点击取消、重新打开显示仍为 4；现有 Profile 总数仍为 32。
7. 工具操作：弹出文字菜单正常；选择「工具展示」打开单对象编辑对话框，取消后返回；选择「移除」会弹独立确认框，取消后工具保持不变。
8. 在最小 960×680 桌面窗口复验编辑页：信息区、筛选控件、来源工具与操作入口未裁切；菜单靠近视口底部时自动换向。
9. 期间原生桌面进程与 `127.0.0.1:8765` MCP 监听保持运行，没有重启、重配或改动现有 Profile 的授权。

## 发现与修复

### P1：长工具描述吞噬整个视口

真实 `AnySearch` 来源的部分工具具有数千字符的描述；详情页原先默认渲染全文，遮挡其他工具。现在默认两行预览，只有明确操作才展开。长描述披露控件添加工具名语义标签，避免辅助技术将整段元数据误读为控件名称。

### P1：工具行操作密度高且直接移除

原来每条工具旁并排四个图标操作（展示、治理、凭据、移除），移除立即提交并没有确认。现合并成单个带文字的「操作」弹出菜单，功能完整保留；移除改为明确确认，取消无副作用，失败在确认框本地显示。

## 质量检查

- `pnpm typecheck`：通过
- `pnpm lint`：通过
- `pnpm i18n:check`：通过
- `pnpm exec vitest run`：64 个测试文件、401 项全部通过
- `pnpm --filter @mender/desktop build`：通过
- `pnpm --filter @mender/web build`：通过
- `pnpm desktop:architecture:check`：通过
- `git diff --check`：通过

## 范围限制

- 本轮在真实 Wails WebView 中对用户现有配置进行了**只读与非持久性操作验证**；未新增永久 Profile，也未保存/删除生产 ToolBinding 或 Tunnel 绑定，以免影响正在运行的 MCP 服务。
- 提交与后端持久化的正确性由隔离数据的组件/集成测试覆盖；真实连接写入未执行。
- 生产前端构建已通过；本轮原生 UI 变更由现有 Wails 开发版的 Vite 热更新加载，未强制重启运行中的 Desktop Runtime。
- 通过 macOS Accessibility 树核对交互语义，但这不等同于完整 VoiceOver/WCAG 认证。

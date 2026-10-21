# Mender 表单帮助密度治理：Web 验收与重建边界

## 交付

- `513fb15`：文档先行，区分核心字段、用户必须可见的风险/错误、按需读取的技术说明。
- `533ea03`：`FormInfo` / `FormHeading` 共用组件及两项交互测试；基于 Base UI Popover，支持 hover、点击/触屏、键盘与 Escape，独立的辅助按钮不嵌进 HTML label。
- `b8b4a02`：Server、Tunnel、Credential、Profile、Provider、Agent 创建、Sandbox 创建 7 个主要领域的创建/配置表单帮助收敛。保持 API、授权、安全字段与已有高级设置不变。

## 交互与布局证据

采用原有 Web Vite :5174 的隔离浏览器，GET Profile/Server/Credential 使用只读 Fixture；未真实创建凭据、Profile、Server、Tunnel 或 Sandbox。

- Server 1280×800：创建弹窗高度 361.5px，核心配置档、地址及短的关联 Tunnel 重要状态始终可见；原多段介绍、重复信息卡片已退出视觉首屏。
- Server 390×844：无横向溢出，底部按钮可用；帮助图标点击和 hover 打开 Base UI Popover（观察到 aria-expanded=true），定位在对应字段下，不遮挡弹窗标题。
- Credential 1280×800：创建弹窗 453.5px，默认仅显示来源、可读名称、Secret 及折叠高级设置，来源与 Secret 等字段附有 Info 入口；导入/轮换及提交安全约束未改变。
- 视觉记录保存在 Mac `/tmp/mender-formdensity-final-server-1280.png`、`/tmp/mender-formdensity-final-server-390.png`、`/tmp/mender-formdensity-credentials-1280.png`；Fixture 只代表浏览器交互，不是生产 Wails 实际 Server 启动成功。

## 自动化与部署边界

`pnpm lint && pnpm typecheck && pnpm test && pnpm build` **退出码 0**，55 个测试文件、346 项通过，i18n **1460/1460**，Web/Wails frontend 产物构建成功，既有大 bundle 提示非阻断。日志 Mac `/tmp/mender-formdensity-final-20261021.log`。

本轮开始时复核生产运行 Desktop 构建日期为 2026-10-21 17:30:18，MCP :8765 在监听，生产数据库只读计数 Credential=3、Tunnel=1、Sandbox=7、Profile ToolBindings=32，`quick_check=ok`。此批 UI 源码晚于当前 Wails 构建，**重新构建刷新后的原生 Info 实机验收尚待进行**，不因 Web 测试通过而误记。

未触碰四个既存未跟踪工具目录 `.agents/`、`.commandcode/`、`.intent-loop/`、`.pi/`。

## 后续范围

已优先覆盖高频创建表单；Skills、Policy 规则编辑、Agent 详情和 Runtime 部分低频配置仍存在说明段落，后续按同一规则逐项检查。不得仅用 `<p>` 标签数量评价：风险、失败、费用、权限和状态必须可读可见。原生完整屏幕阅读器和全键盘路径尚未完成。

# Mender 表单 P1/P2 收口：Unicode Credential 名称与 MCP 端口自动轮换（2026-10-21）

## 源头与目标

接续 `docs/planning/form-task-first-live-wails-acceptance-2026-10-21.md`。普通中文用户只输入可见凭据名称就应该能够创建；已占用 `127.0.0.1:8765` 时，新建 MCP Server 应在 **实际 bind 阶段**自动切换后续可用端口，而不是要求用户理解、扫描或手动试错端口。

### 不变量

- 维持现有 Credential `credential://<namespace>/<name>` 和 Go 端严格 ASCII 标识规则，不变更数据库/schema、权限、密钥保管、OAuth 授权、现有 Credential/Server/Tunnel 配置。
- **Unicode 派生仅对未手动指定内部 Name 的新凭据草稿有效**；已有 OAuth 重授权必须保留原 `namespace/name`，切换类型必须清空密钥/私钥/旧 OAuth 参数。高级设置仍允许用户明确编辑标识。
- 界面名称按原值保留并校验 UTF-8 最大长度。技术标识需要确定性、稳定、合法；纯合法 ASCII 名称保持原语义，包含 Unicode 或其他无法直接映射的名称生成可辨识的 ASCII 前缀 + 非敏感、固定大小指纹。重复时优先根据已加载库存选择合法顺延后缀，并以 Go 端唯一性校验作为最后权威，绝不覆盖现有 Credential 或悄悄替换已授权的 OAuth 标识。
- 服务地址只允许 loopback。原有 `127.0.0.1:0` 仍由 OS 原子分配可用端口；非零端口从指定端口开始尝试，只有 `EADDRINUSE` 时有限步进（至多 32 个连续端口、不得越过 65535），其他网络失败原样终止。不做 UI TCP 预探测或重复启动整套 Profile/Tunnel，以免授权/进程副作用重放。
- 返回的 `Address/Endpoint` 必须是成功绑定的真实地址，保持 `desiredAddress` 的原值以支持已有 Restart 机制（重启重新选择当前可用端口）。已运行的 Server/Tunnel 不被迁移或停止。
- 既有表单高级固定地址输入仍可编辑；在主表单明确说明端口占用会自动尝试后续端口，实际启动后的服务地址只能取后端返回值。
- 严格源代码 TypeScript、i18n、Go 单元/集成与浏览器验收，安全测试不得调用真实生产 Credential 创建或中断已运行 Tunnel。

## 验收矩阵

1. Latin/中文/日文/emoji/混合名称、长名称、规范化等价 Unicode → ASCII 63 字节以内、以 `[a-z]` 开头，原始 DisplayName 不变；名字相同/库存已存在可选择顺延后缀；手动高级标识不被可见名称改写；SSH 保持 `ssh` 命名空间；OAuth 重授权身份不变。
2. 以真实 loopback listener 事先占用 requested port，`StartMCPHTTPServer` 只能创建一个管理 Server，向后绑定真实可用端口，返回实际 Endpoint，老 listener 保持运行；`RestartMCPServer` 使用原请求地址重新轮换；`:0` 维持 OS 选择；非 loopback 校验、非端口占用错误不得无条件重试。
3. Web/Wails 同源表单中的帮助文案明确，但无新的隐藏授权；用户不用自行尝试 `8766` 等端口。
4. 文档先提交、实现分阶段提交、自动检查通过，给出应用未重建时的明确验收边界，不触碰已存在的 untracked Agent 配置目录。

## 实施与证据记录

### 分阶段提交

- `6d50c31`：先提交本实施合同，冻结数据、权限、UI/Runtime 端口不变量。
- `46eb33a`：`packages/app/src/domains/credentials/credential-form.ts` 的 Unicode → 合法 ASCII Name 派生；原有纯 ASCII 行为不变。新凭据优先与当前库存对比，同一 Namespace 同名可获得 `-2`/`-3` 顺延，Go 唯一性约束仍是权威。原生、SSH 创建共用派生逻辑，OAuth 重授权使用旧 namespace/name，显式修改过的高级 Name 不被覆盖。新增独立单元和完整创建表单测试；删除已不再适用的“必须手填英文名称”提示。
- `e889343`：`backend/internal/bootstrap/mcp_server_listener.go` 新增一次性绑定过程中的顺序尝试。请求地址如 `127.0.0.1:8765` 被占用时仅监听端口移动到 `8766`、`8767` 等，至多 32 次且永不越过 65535。仅 `syscall.EADDRINUSE` 继续；不重复 `ActivateProfileProviders` / `NewMCPServer` / `Tunnel.Start`，不改变 `desiredAddress`。使用现有实际 `listener.Addr().String()` 形成 `Endpoint`；原先 `:0` 的 OS 自动端口语义保留。更新 Server 表单提示、i18n 与结果展示回归。

### 定向行为证据

- Go `TestManagedMCPPortAutoRotationPreservesExistingListener` 使用真实占用的 loopback socket 和隔离 `Runtime` / 空 Profile 验证：新 Server 自动绑定后续端口、原监听器不受影响、Restart 重新从原始端口请求、Stop 后原端口仍可连接。 `TestManagedMCPPortAllocationRetainsPortZeroContractAndRejectsInvalidPort` 验证 OS 分配和越界非法端口；原已有非 loopback 拒绝测试保持通过。`go test -race ./backend/internal/bootstrap -run 'TestManagedMCPPort|TestManagedMCPHTTPServerLifecycle' -count=1` 已通过；`golangci-lint run ./backend/...` 已通过。
- React 专项 5 文件 **84/84** 通过；凭据专项 3 文件 **34/34** 通过；TypeScript 和 i18n **1450/1450** 在后续端口文案更新后通过。
- **390×844 隔离 Web 浏览器（Mender :5174，read-only API Fixture）**：真实操作 Credential → 输入“项目密钥” → 展开高级设置，生成 `credential://personal/credential-u-029aa923621e88e6`，无需英文手填；dialog 底部 828px 在 844px 视口之内，无横向溢出，内容可滚动。Server 页面在有两个 Fixture Profile、已有 `8765` 服务时，默认不自动选择 Profile，端口顺延提示可见，表单底部操作区在视口内。截图只保存在 /tmp 下而不加入生产库，**Fixture 不能证明生产 Server 已切换端口**。
- 用户的真实 Wails 进程仍为本阶段编译前的版本，最新 Go 端口轮换实现尚未重建/部署；没有对正在使用的 MCP Server、Tunnel 或生产 Credential 发起写操作。本次 UI 浏览器只进行打开/填写非机密草稿/取消。历史的四个 untracked Agent 工作目录始终不纳入提交。

### 验收限制

- 本次按“自动轮换端口”要求默认对 **所有合法的非零 loopback 监听端口**应用有界顺延；若操作者在高级设置中输入显式端口，当端口被占用时也会自动顺延，并在列表里显示成功的**实际**端口。不能保证每次 Restart 都保留同一数值端口；消费实际 Endpoint 的客户端需要使用当前返回值。
- Tunnel 的 provider-side 凭据、控制面连接、同一 Tunnel 是否允许多个运行实例属于独立治理，**端口顺延不能消除 Tunnel 后续启动拒绝**。该行为未被改变。
- **完整 `pnpm check` 已通过，退出码 `0`**，日志 `/tmp/mender-port-rotation-fullcheck-20261021.log`。包括 provider/sandbox/tunnel plugin SDK、proto/schema、desktop bindings/architecture、Go lint + `go test ./backend/...` + `go test -race ./backend/...`、TypeScript/ESLint、i18n 1450/1450、Vitest **54 文件 / 344 项全部通过**以及 Web/Wails 前端构建。Go bootstrap 普通测试 40.505s、全量 race 的 bootstrap 测试 283.436s；构建仅保留既有的大 bundle 非阻断告警。
- **部署边界明确**：截至记录时 Wails 二进制时间仍为 2026-10-21 15:53:55（早于这次 Go 改动）。不能将新 Go 行为描述为已在生产运行；须由用户重新构建、重启，再对现有 8765 监听保持不动的前提下使用隔离 Profile/不带生产 Tunnel 的测试资源验证新端口分配。此次没有写入已有 Credential/Server/Tunnel/Profile，read-only SQLite 确认为凭据 3、隧道 1、Sandbox 7、Profile ToolBinding 32、revision 10、quick_check=ok。

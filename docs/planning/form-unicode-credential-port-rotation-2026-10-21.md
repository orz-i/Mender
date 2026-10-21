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

（完成代码、测试和部署/浏览器验证后追加。）

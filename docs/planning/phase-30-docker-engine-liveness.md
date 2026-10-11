# Phase 30 — Docker Engine 长期自动连接与健康治理

状态：实施中（2026-10-11），衔接已收口 Phase 29。遵循 ADR-0035/0036/0037；文档先行、分段提交。

## 审计结论

Phase 29 已打通固定本地 UNIX Socket 自动发现，成功注册后 `runDockerDiscovery` 即退出。当前 Docker Desktop 退出、重启、切换 Docker context / 守护进程时，`DockerAvailable()` 只检查 Provisioner 非空，`DockerStatus()` 恒为 `ready`，`BackendAvailable` / 插件市场同样会错误报告可用。后台丢失真实连接状态。

Docker Engine `/_ping` 和 `/info` 是官方读接口；守护进程的 `Info.ID` 是可观察身份。既有 `Inspector.Probe` 逐次校验 API、Linux、seccomp、cgroup-v2、CPU / memory / PID，允许通过固定 Socket 重试。不能将“Unix Socket 存在”视作 Engine 就绪，也不能只依赖单次检查作为永续凭证。

## 行为契约

1. **稳定 Engine 身份钉住**：首次通过安全探针时捕获并内部钉住 `Info.ID`（要求非空、有界）；之后任何探针、镜像验证、容器生命周期及工具 Session Open 必须拒绝另一个 `Info.ID` 的 Engine。引擎身份更换需重启 Mender，由操作者重新确认管理的沙盒；不自动切换 Docker Desktop/Colima/OrbStack。
2. **持续有界探测**：单一可取消 Worker 每 10 秒运行：尚未连接时按照现有固定可信候选路径顺序尝试；已连接时**只重探相同端点/同一 Engine 身份**，失联进入 `unavailable`，同身份恢复进入 `ready`。配置 `MENDER_DOCKER_SOCKET` 的桌面运行时也纳入监测，但不改变明确配置的优先级。成功后**不停止**监测。每次探针限时、串行执行，不在只读列表或页面刷新时主动探测，不产生日志/网络端点泄漏。
3. **安全失效**：插件市场、`GetSandboxBackends`、沙盒资源 `backendAvailable` 必须基于实际观察的状态，而非注册表是否仍包含 Backend；不可用时禁止新建沙盒、打开 Workspace 工具 Session 以及继续 start/stop/destroy/reconcile 的 Docker 生命周期（不做宿主降级）。工作中的 Tool Session 也应在发起下一次操作前检查当前可用标志。所有持久化 Run、Generation、Lease 与 Container 保持原样；不可用不等于自动销毁。
4. **连续恢复**：原 Provisioner/Inspector/Registry、绑定 ID 不变；同 Engine 重新上线才恢复状态。绝不删除或重建已持有的 Sandbox，不回收不确定的 Lease，不试探其它 Engine。身份变化单独标记并持续拒绝（必须重启），即使新的 Engine 满足隔离配置。
5. **前端一致性**：当 Docker 从 ready 变 unavailable 时，插件市场及沙盒创建页面也应按低频只读刷新降级状态；恢复后继续可用，不额外要求点击按钮。只有 native Desktop 启动监测，管理 HTTP/MCP/Tool 不得开启自动连接与选 Endpoint。
6. **验证**：Engine 伪端点/身份变化/探针错误单测，后台 Worker 的断线-恢复/重复调用/取消/关机 Race 测试，Registry+Execution Router 的 fail-closed 与恢复用例；HTTP 和 Wails 统一投影、UI 降级/恢复；真实 Docker 只读监测和原沙盒生命周期回归。完整 `pnpm check`、安全扫描、原生构建与 Dev 启动。测试不停止用户正在运行的 Docker、不触碰真实用户 Sandbox、容器或 DB。

## 提交节奏

- A：本文档与验收契约。
- B：Engine ID pin、持续有界 Worker、故障恢复及后端测试。
- C：路由/管理/UI 状态闭环、完整回归、安全/真实 Engine 测试、收口。

边界：不构建 Agent Loop；不自行启动/停止 Docker Desktop，不扫描任意 UNIX Socket 或 Docker contexts，不引入新的兼容桥接，不自动拉取镜像，不开放 daemon API 给 Tool，不修改 `.agents/` / `.pi/` / `.intent-loop/`。

参考：https://docs.docker.com/reference/api/engine/ ，https://docs.docker.com/engine/security/protect-access/ 。

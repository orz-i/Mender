# Agent 失败阶段诊断与真实开发委派

Status: implementation in progress (2026-10-18). Boundary: Mender only admits, governs, supplies and audits external Agents; it does not run their reasoning loop.

## Confirmed post-rebuild baseline

- User-rebuilt Wails dev process and Mender Tunnel are connected; repo HEAD `e0cb565`, tracked worktree clean. Production schema: Agent v12, Policy v3, 3 Agents and 2 existing owner approval modes; `PRAGMA quick_check=ok`.
- Previous real Mender OpenCode coding attempts `atask_1d5261ea4bec2d2ada8f54d988815d27`, `atask_be54e86395897faee1da2693c1ff1d32`, `atask_d4152cc9717a680269068e084a5302e4` ended `failed`, all persisted only `Agent execution failed`. The separate true code-edit and test delivery `atask_798778de8ef6af7e5ee68e461ed36c48` completed. These failures must not be reinterpreted retroactively.

## Goal and design

Replace the generic `Agent execution failed` with a **finite, immutable and non-sensitive** stage code/message, grounded in a failure from the native ACP runner. No raw CLI stderr, model prompts, tool parameters, remote RPC error messages, credential values, shell commands, full file paths, session identifiers or model output may enter `agent_tasks.diagnostic`, Wails bindings, external profile Tools or logs. A stage code identifies *where* failure surfaced, not its root cause or a proven model error.

The native runner returns an error implementing the backend-neutral interface:

```go
interface { SafeAgentFailureStage() string }
```

Only these fixed values are accepted by the Agent Application Service:

| Code | Owner-facing stable diagnostic |
| --- | --- |
| `process_launch` | Agent process could not start |
| `initialize` | ACP initialization failed |
| `session_create` | ACP session creation failed |
| `configuration` | Agent model or reasoning configuration failed |
| `permission` | Agent permission request was rejected |
| `prompt_rejected` | Agent rejected the ACP prompt |
| `prompt_transport` | ACP prompt transport failed |
| `protocol` | ACP protocol validation failed |
| `result` | Agent returned an invalid completion |

Anything else, including maliciously supplied free-form stage labels, remains `Agent execution failed`. Existing cancellation, timeout, agent outcome unknown, result overflow and governed ToolSupply failures retain their established semantics/priority; no modification of prior Task records. No new database table or schema bump is required.

### Implementation ownership

1. **Docs first**; commit this scope before implementation.
2. **OpenCode real coding owner**, precise non-overlapping ACP-only files: `backend/internal/agent/infrastructure/acp/runner.go`, NEW `failure_stage.go` and NEW `failure_stage_test.go`. Implement stage tagging and fixture tests; run actual gofmt + Go package tests and Race Detector. Do not commit or edit outside these files, use network, secrets, Profile settings, production DB, Wails process or Skill directories. Real Task must finish `completed` with a verifiable source diff; otherwise the coordinator must report failure, not claim successful Agent development.
3. **Coordinating assistant**, disjoint Application-only files: `backend/internal/agent/application/task_failure_diagnostic.go`, `task_failure_diagnostic_test.go`, `service.go`, targeted test fixtures if needed. Map the *fixed*, trusted vocabulary to fixed strings, test censorship of malicious/raw error chains, and preserve cancellation/outcome logic. Don't modify OpenCode-owned files while its Task is running.
4. Independent line-by-line review of Agent diff, targeted Go test/Race Detector, production-consistent **private SQLite copy** acceptance, full `pnpm check`, native Wails build, sectional Git commits. No forced restart or migration of the active user Wails.

### Acceptance

- Synthetic native ACP failures at process launch, init, new session, invalid protocol, model configuration, RPC prompt rejection, premature prompt transport close, unresolved permission and malformed final result produce correct **fixed codes**; tests must not persist raw fake secrets even when underlying `error` contains them.
- The application persists exactly one safe message for each known code. Unknown or injected stage names and raw error chains cannot influence the diagnostic; no arbitrary `fmt.Sprintf` of an untrusted error.
- Running real OpenCode via Mender can still edit/test and use the existing explicit owner `trusted` mode; this round must not claim a failed real Agent Task as successful development. Any live smoke test may use a disposable workspace and no user data mutations.
- Original user Task history, Agent/Provider identities, ToolSupply, grants and Policy v3 behavior remain unchanged. Schema upgrade unnecessary.
- Limited nonsecret stage reporting is **not** full observability of model APIs or native OS shell outputs and must not be described as such.

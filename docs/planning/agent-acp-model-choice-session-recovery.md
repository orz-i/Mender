# ACP 会话模型选项波动：配置栅栏与单次受限恢复

Status: bounded implementation in progress (2026-10-18). Authority: existing Agent lifecycle and ADR-0054. Mender remains a tool/resource/permission supplier, not an Agent Loop.

## Confirmed discrepancy

- Owner-rebuilt Wails on main 47ff1d8, Agent schema v12 / Policy v3. Two real Grok Mender-delegated Tasks atask_d5bfe0efe8ea3d5bd6a2bf9e7bdaa271 and atask_1bf98e920992aaa0592abda088498339 failed in configuration before prompt, with zero native ACP approvals. Production Grok defaults grok-4.7/xhigh must NOT be changed or silently substituted.
- A no-prompt ACP v1 probe with the same allowlisted Grok executable, environment and exact Mender repo workspace showed intermittent advertisement: first of three fresh sessions did not advertise grok-4.7; the other two did and confirmed grok-4.7/high and grok-4.7/xhigh through session/set_config_option. Four further fresh sessions all advertised it. Upstream source of the fluctuation remains unknown.
- The existing model.go setACPChoice rejects an unadvertised requested model, but currently produces only a generic configuration Task diagnostic. runner.go configures choices after new session and before the unique prompt; Mender need not store stderr/raw frames/credentials.

## Strict solution contract

1. Fail closed using a finite fixed error classification for configured model not advertised and configured reasoning effort not advertised, without interpolating model IDs or user-provided strings. Existing cancellation/deadline precedence and generic configuration fallback remain.
2. Permit at most ONE extra ACP session/new on the SAME owned child process, same canonical WorkspaceRoot, same exact MCP ToolSupply token and unchanged owner admission, only after a configured model or effort is *not advertised*. Retry takes place strictly before any session/prompt. Use a distinct RPC ID and revalidate the new SessionID. The requested model/effort must be explicitly advertised and confirmed; never auto-switch a model, weaken the reasoning request, or turn on allow-always.
3. No retry after a prompt, RPC rejection, confirmation mismatch, transport failure, subprocess crash or cancelled context. A retry-session creation error fails closed; no silent fallback to the original session. On normal successful options only one session is created.
4. These are voluntary Host ACP permissions, not filesystem confinement. A Grok native Host file read can bypass permission callbacks; mandatory file access rules require actual process isolation and governed Workspace Tools, not a stronger Host trust setting.

## Real parallel code ownership

- OpenCode real Mender delegation: writable model.go plus NEW model_choice_recovery_test.go only. Implement bounded fixed errACPModelNotAdvertised and errACPReasoningNotAdvertised sentinels, preserve agentdomain.ErrUnavailable, and prove selection errors and valid confirmations in Go tests. Actually edit/test, no git commits, no other files.
- Coordinator: runner.go, runner_test.go and any NEW runner-specific test; application fixed diagnostic mapping/tests; frontend translation/tests. No overlapping source writes until Agent terminal state. Independently review and commit all changes.
- Docs first, grouped commits, no DB migration, no new external Tool API, no Agent Loop or extra sandbox authority. Preserve Skill directories and existing user DB/Agent policies and historical tasks.

## Required acceptance

- Fixture missing model first new-session then advertising on second: exact configured model and effort confirmed, ONE prompt delivered, Task completed.
- Model still unadvertised twice, effort missing twice, confirmation mismatch and malformed/rejected retry: fail closed with fixed reason and ZERO prompts.
- Normal options: ONE session only. No change to restricted-host ToolSupply and original ACP permission tests; contexts retain errors.Is cancellation/deadline.
- Go full Agent/bootstrap tests, Go Race Detector, lint, pnpm check, native Wails compile, production SQLite read-only check, and real Agent Task receipts. No forced production Wails restart.
- Do not claim Grok live success until owner rebuilds and the newly delegated Grok Task actually completes. Previously stored failures remain immutable.

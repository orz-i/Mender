# Grok ACP prompt transport failure: bounded diagnosis

Status: document-first, implementation/acceptance in progress (2026-10-18). Boundary: Mender aggregates, governs and supplies Agent tools/resources; it does not own Agent Loop or the model's retry strategy.

## Baseline and problem

- Wails and Tunnel reconnected after the owner rebuilt; repository at `9e37c06`, tracked worktree clean, Agent schema 12 and Policy schema 3. OpenCode, Grok and Codex have **existing user-owned** non-expired `trusted` Host authorizations; do not change/renew them.
- Previous real Grok development `atask_c29481d4c161bcacf0ccbee352233693` reached **`ACP prompt transport failed`** with 0 native ACP approvals and no repository modifications. A separate no-tool Grok task `atask_d6e32b3327df186d6223ec67eff3132c` completed. The earlier failed reason is a **broad** category, not proof of auth, model or tool failure.
- Current `runner.go::stream.call` reports generic EOF, scanner invalid JSON or scanner read errors; `runner.go::annotateRead` merges scanner failure with a generic stream close; `failure_stage.go::stagePromptError` maps almost all such failures to `prompt_transport`. No raw stderr is retained.

## Contract: one bounded improvement, no sensitive detail

Add only fixed, non-sensitive error categories to the existing `SafeAgentFailureStage()` whitelist, without changing database schema or public Tool API:

| Code | Meaning bounded to observed Mender ACP boundary | Exact safe user message |
| --- | --- | --- |
| `prompt_stream_closed` | ACP stdout closed before a matching `session/prompt` response | ACP prompt stream closed before completion |
| `prompt_malformed_frame` | ACP stdout emitted a frame that is not valid JSON | ACP prompt frame was malformed |
| `prompt_read_failed` | ACP stdout Scanner returned an I/O/frame-size error | ACP prompt stream read failed |
| `prompt_write_failed` | Mender failed to write the outbound ACP request frame | ACP prompt request write failed |

Keep existing `prompt_transport` for unclassified errors, `prompt_rejected` for **explicit** JSON-RPC reject responses, `permission` for client-permission rejection, `result` for invalid final outcome, and existing cancellation/deadline precedence. Never infer which model/API, CLI plugin, OS process status or exact native tool was responsible. No raw `stderr`, prompt, RPC `message`, path, token, command, tool arguments or stdout contents are saved. The mutable prompt frames are never logged. The safe error text is a literal constant only, with no user-controlled interpolation.

## Implementation and ownership

1. Commit this plan first. Validate existing source and tests.
2. OpenCode **real development** on ONLY `backend/internal/agent/infrastructure/acp/failure_stage.go` and `failure_stage_test.go`: add typed local sentinel errors, the four fixed codes and a `stagePromptError` precedence table with tests (including malicious error text and errors.Is cancellation). It must edit code, run gofmt and actual ACP package tests/Race Detector, return a terminal Task. Do **not** count failed/partial tasks as successful delivery.
3. Coordinator concurrently owns disjoint `backend/internal/agent/infrastructure/acp/runner.go`, `runner_test.go` or a new `prompt_transport_runner_test.go`, `backend/internal/agent/application/task_failure_diagnostic.go` and its tests, the React Agent Task detail translations/tests. The scanner/transport tags are emitted only from actual stream terminal paths. No Agent may modify these files while the coordinator is working.
4. Retain an owner-only narrow failure classification. A task completion status and the observed immutable Task diagnostic are more authoritative than prompt text; exact test coverage includes the 4 failure types plus existing remote RPC error, permission failure and cancelled contexts.
5. Independently review OpenCode diffs; do not stage or commit until terminal status and tests verified. Canonical `pnpm check`, Wails Go build, SQLite read-only state/fingerprint if migrations were changed (expected no schema changes), sectional Git commits.
6. Once compiled, optional *disposable* real Grok test may exercise the new binary's prompt path with an isolated workspace/database and original scoped owner-approved settings; no production Host grant mutations or new network taps. Final deployment still requires the owner's usual Wails rebuild. Historical failed Tasks are never reclassified.

## Non-goals

No bypass of Grok's permission request, no Host allow-all expansion, no logs of raw ACP frames, no reading credential files, no model API interception, no introduction of a Mender-owned harness, no Agent Task history deletion. A stream EOF is not proof of child crash or model service outage.

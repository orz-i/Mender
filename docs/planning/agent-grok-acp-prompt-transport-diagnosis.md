# Grok ACP prompt transport failure: bounded diagnosis

Status: **bounded four-stage ACP prompt transport diagnosis accepted with full Go Race Detector, canonical quality gate and isolated real Grok QA** (2026-10-18); original Grok coding transport failure remains unexplained and latest code awaits owner-controlled Wails deployment. Boundary: Mender aggregates, governs and supplies Agent tools/resources; it does not own Agent Loop or the model's retry strategy.

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

## Implementation, real Agent delivery and owner-independent proof

**Source commits** (bounded, no compatibility bridge or schema migration):

- `70d4056` — this design contract, before coding.
- `7c46fa9` — real OpenCode contributed four fixed stages, local sentinel errors and tests in `failure_stage.go` and `failure_stage_test.go`. Actual Mender Task **`atask_c67109477cb9ff2d77322a19c6c29600`** ended `completed` after **read 4 / edit 6 / execute 11** one-shot `policy` approvals. Its first full-package test failed because simultaneous coordinator Runner edits exposed a classification priority mismatch; the coordinator independently corrected it and a test false-positive from Go lint. **Agent's initial full-package exit was 1 and must not be reported as an independent green deliverable.**
- `1edc7fa` — coordinator connected the actual ACP IO origins in `runner.go`, added a synthetic real-child fixture and deterministic tests. A combined scanner failure + EOF must prioritize the **original malformed JSON/scanner failure**, not the secondary stream close. Actual subcategory test results passed for explicit stdout EOF, malformed JSON, oversized Scanner frame and broken outbound writer.
- `b108a3a` — coordinator extended the strict Application Stage→literal mapping and sensitive-error redaction tests. Cancellation/deadline, existing permission failures, malformed result and remote RPC rejection all keep their older precedence.
- `d6ab44f` — coordinator extended the owner Wails Task result localization; targeted Vitest and TypeScript passed, i18n check now **1329/1329**.
- `f618e76` — opt-in **real installed Grok ACP** acceptance on an independent temporary SQLite database and workspace. No real model inference is triggered by normal `pnpm check`; all one-shot test grants are scoped to a temporary workspace and disappear with the private database.

**Independent gates before canonical check:** ACP full package and Application/Bootstrap Go tests; scoped Go Race Detector; `pnpm go:lint` **0 issues**; React/TypeScript; 14 focused Agent UI tests; i18n 1329/1329. Full canonical `pnpm check` and native Wails Go build are separately recorded below when completed. No production Wails restart or owner authorization change was requested or made.

## Actual Grok behavior in isolated real ACP sessions

Three intentionally bounded, disposable runs produced **different** outcomes:

| Evidence (private temp log) | Configuration / observed outcome | Meaning |
| --- | --- | --- |
| `/tmp/mender-grok-transport-real-probe-20261018/real-grok.log` | Explicit agent `grok-4.7/high`; no-tool and benign-file prompt both failed **configuration** in ~21 s; zero ACP grants. Test runner returned 0 because it was collecting safe evidence, **not because the model tasks succeeded**. | No proof of prompt transport in that run. |
| `.../real-grok2.log` | Temporary inspected session reported `grok-4.7` not advertised but `high` advertised; backend-default no-tool Task **completed**; file-read Task exceeded 80 s and was cancelled. Test exited 1. | Dynamic session capabilities or model-choice inconsistency; the read Task's pending-permission counters were **not captured**, so do not infer why it stalled. |
| `.../real-grok3.log` | Session advertised both `grok-4.7` and `high`; backend-default minimal file-read Task **completed** in ~8 s and returned the exact marker stored only in the disposable `marker.txt`. **ACP approval ledger: 0 read/other allowed, 0 pending, 0 rejected.** Test exit 0. | Strong evidence Grok can read a native Host file without emitting a `session/request_permission` request observed by Mender. |

In the opt-in run the **test-only** owner approval mode was `workspace_read`, intentionally narrower than `trusted` and backed by a new SQLite rather than the real owner's live settings. This does **not** create mandatory filesystem confinement. The native reader succeeded without any recorded ACP approval, so **Mender's ACP native approval gate cannot be presented as a complete or mandatory file-access guard for Grok Host Tasks**. A mandatory boundary requires actual isolation and governed tools (e.g. restricted-host + scoped Mender Workspace Tools), not another permissive Host trust mode.

The original live Grok coding task `atask_c29481d4c161bcacf0ccbee352233693` remains an immutable **`ACP prompt transport failed`** record from the previous binary. The new fixed subtype is not retroactively knowable, and the isolated Task successes/failures above do **not** prove its exact root cause. Do not infer model outage, credentials failure, ACP tool support or subprocess crash without matching first-party observation. The next live production occurrence, **after owner-controlled rebuild**, can distinguish stream EOF vs malformed frame vs Scanner error vs failed outbound write without exposing stderr or messages.

**Security and data invariants:** all three probes used disposable workspace and database, without edits to real project files or original owner Agent policies. The code includes no prompt/raw stderr logging or per-file target telemetry. Local test model metadata was reduced to boolean advertised options, not logged verbatim. The production Agent/Policy schemas remain v12/v3, no existing Task histories were rewritten, four preexisting Skill-like directories untouched.


## Final canonical validation and rollout gate (2026-10-18)

- **Full `pnpm check`: exit 0**, private receipt `/tmp/mender-grok-acp-transport-closure-20261018/check.exit` and log `/tmp/mender-grok-acp-transport-closure-20261018/check.log`. Includes full Go tests and Go Race Detector (ACP, Bootstrap and all backend packages), `golangci-lint` 0 issues, Plugin SDK/protocol validations, Wails bindings and architecture, TypeScript, ESLint, **46 Vitest files / 270 passed**, i18n **1329/1329**, Web/Desktop production builds. Existing Vite chunk-size warnings are nonblocking.
- The latest native macOS Wails Go application was separately compiled from these changes to an isolated temporary executable, verified successful and then deleted. No QA executable was installed into the running owner's app.
- Immutable source commits: **`7c46fa9` (real OpenCode code contribution, independently reviewed/integrated), `1edc7fa` (Runner), `b108a3a` (fixed Task diagnostics), `d6ab44f` (Chinese Wails UI), `f618e76` (real isolated Grok acceptance)**. OpenCode Task `atask_c67109477cb9ff2d77322a19c6c29600` was completed, but its originally failed full package run due to concurrent integration remains honestly recorded; the coordinator fixed the cross-file priority and independently reran Go/race/lint/full checks.
- Final **production SQLite read-only** snapshot: Agent schema **v12**, Policy schema **v3**, **3 Agents**, **43 historical Tasks**, **184 historical ACP approvals**, **3 existing unexpired `trusted` owner modes** for Codex/Grok/OpenCode, **0 governed Tool placements**, `PRAGMA quick_check=ok`, foreign key violations zero. Historical Task/approval rows were not rewritten, scopes/grants/default models unmodified and four existing skill directories preserved.
- The production Wails process and Mender tunnel predate these new transport-substage commits; neither was restarted or reconfigured by the coordinator. **These new subcategories are accepted in disposable/test Runtime, not yet verified in the live production Wails binary.** The owner's next normal rebuild/reconnect is required. Existing Grok failures stay under their original broad `prompt_transport` diagnostic forever; retroactive reclassification is prohibited.
- **Disposition:** bounded diagnostic enhancement and genuine OpenCode development accepted. **Not resolved:** precise upstream cause of intermittent Grok native coding failure, dynamic ACP model option visibility, and absence of native ACP permission receipts for successfully observed Grok file reads in Host mode. Never treat a new stage code as proof of model API outage or client-side confinement. For mandatory file access controls use actual process isolation and governed Workspace Tools, not voluntary ACP Host approvals.

## Non-goals

No bypass of Grok's permission request, no Host allow-all expansion, no logs of raw ACP frames, no reading credential files, no model API interception, no introduction of a Mender-owned harness, no Agent Task history deletion. A stream EOF is not proof of child crash or model service outage.

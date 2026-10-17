# Phase 44 research — ACP approvals and governed OpenCode tools

- Status: **B and C implemented; D cooperative read/denial acceptance passed, authoritative write/shell gate remains blocked** (2026-10-17, Mac local).
- Authority: [ADR-0050 — ACP approval mediation](../architecture/adr/0050-acp-agent-tool-approval-governance.md), Accepted.
- Product identity: Mender serves and governs tools/resources. OpenCode owns the model, plans, native tools and Agent Loop.

## Findings

Current Mender ACP v1 refuses all session/request_permission and client-side fs/terminal calls. It does not advertise Client file/terminal capabilities. An Agent Task currently has only queued/running/terminal statuses, generation fencing, a ten-minute deadline, immutable workspace/model snapshot and a short result; owner approval persistence does not exist. OpenCode v2.0.30 is connected and advertises real task tools, but its native read/edit/shell tools do not automatically travel through the Mender Workspace Tool router.

OpenCode v2 uses permissions rules (not old V1 permission/basH/task config). Its base policy allows common tool operations; only ask requests reach ACP. Project configuration, plugins, subagents and saved always options affect behavior. Mender host isolation is a host-user process. An approval dialog alone therefore CANNOT guarantee all file mutations passed a Mender policy decision.

ACP v1 permission selection is an authorization decision, not client execution. Optional ACP fs/read_text_file, fs/write_text_file and terminal/* are separate implementations. OpenCode v2's ACP adapter offers once/always/reject and attaches client-provided MCP servers to the ACP process's directory for the process lifetime.

## Recommended one-phase, four-checkpoint implementation

| Checkpoint | Scope | Acceptance gate |
|---|---|---|
| **A — Contract first** | ADR and threat model, explicit cooperative vs authoritative labels, fixed ACL and approval semantics | Docs-only commit, no runtime writes |
| **B — ACP Approval Kernel** | Bounded ACP request pump; pending ApprovalID scoped to live Task generation/session/toolCallId; persistence and CAS; allow-once/reject only; cancel/timeout/restart rejection; sanitized Audit | Synthetic RPC tests for duplicate/missing IDs, response order, stale approvals, revocation, deadlines and fail-closed behavior |
| **C — OpenCode Adapter + Owner UI** | Verify v2 resolved permissions; Wails Task-detail approval view (one operation preview, Allow once / Reject), owner-only HTTP/SDK/Wails parity; no Profile-facing approve tool | Real ask request is observed on a disposable worktree; model without permission remains unable to approve; never return always; no silent Host fallback |
| **D — Actual governed access gate** | Positive/negative read, one explicitly approved disposable edit and narrow shell tests; establish process/Sandbox and/or Mender Tool execution enforcement before describing strong governance | No unapproved write, escape, self-delegation or replay; full pnpm check, Race Detector, user identity/ToolID unchanged, Wails and tunnel restart tests |

Do not expand into D100-style phases. The same Agent, ProviderID and delegate/status/result/cancel ToolIDs remain stable. Additional registered Agent workspaces are still ordinary owner-approved local directories; strong isolation is an OPTIONAL placement property and cannot be inferred from a directory alias.

## Critical implementation decisions

1. **No allow-always in the first iteration**. OpenCode's suggested remembered rule may persist and be broader than the exact request. Do not create a second persisted trust inventory.
2. **First trusted approver is local owner**. Upstream ChatGPT/MCP Profile cannot authorize its own side effects. The approval decision must be bound to a Task generation, ACP session, toolCallId and short TTL.
3. **Task phase vs Task status**. Keep Task running while a live ApprovalRequest exists; display Awaiting approval derived from pending rows. The current ten-minute timeout is not indefinitely paused. No owner response yields expiry/rejection.
4. **Native host tools vs Mender-managed tools**. Label the first as *cooperative* approval. Only promise enforceable access after verifying a process isolation/controlled tool route that native OpenCode cannot bypass.
5. **ACP capability negotiation is explicit**. Keep fs and terminal capabilities false until real constrained Mender-backed execution methods exist. Do not use OpenCode's project config alone as a security boundary.
6. **Owner absence & restart**. Pending requests expire; cancellation/termination invalidates them. Unknown Task results are never auto-replayed.
7. **One focused viewport**. Agent → Tasks displays waiting count; Task detail opens one bounded approval review. No raw untrusted SQL, shell, credential or entire diff in global Audit.

## Verification outline

- Simulated permission request with exact offered once/reject optionIDs; affirmative response only after owner-authenticated CAS.
- Revoke a Profile AgentGrant or workspace grant while a request waits: old approval fails.
- Crash/restart and duplicate permission IDs: pending is expired, no implicit response/replay.
- Incoming fs/read_text_file or terminal/create when not advertised: reject without filesystem or command side effects.
- Real OpenCode v2 dry run in a NEW disposable workspace: read-only path first; edits and narrow Shell actions only after explicit approval and environment-level checks. No user project write without separate opt-in.
- Strong mode: bypass attempts via built-in OpenCode edit/shell, child Agent and project override must all fail.
- Preserve all user IDs/grants/tasks and the four canonical Agent tool identities.

## Sources

Repo: backend/internal/agent/infrastructure/acp/runner.go, launcher.go; application/service.go; domain/task.go; bootstrap/agents.go; sqlite/admissions.go; packages/app/src/domains/agents/agent-detail-page.tsx.

ACP: https://agentclientprotocol.com/protocol/v1/tool-calls ; https://agentclientprotocol.com/protocol/v1/file-system ; https://agentclientprotocol.com/protocol/v1/terminals

OpenCode v2: https://dev.opencode.ai/v2/docs/cli/acp/ ; https://dev.opencode.ai/v2/docs/permissions/ ; https://dev.opencode.ai/v2/docs/config/ ; https://dev.opencode.ai/v2/docs/policies/

**Implementation gates:** Stage B must keep all client-file/terminal requests rejected; only an actual ACP permission event can create a pending review. Never enable the OpenCode native host read/write claim by merely adding UI. No production data modifications until a tested forward migration and a controlled operator restart.

## Implementation checkpoint (2026-10-17)

**Implemented, tested, but not deployed to the running user's Wails backend:** cooperative, local-owner one-shot approval of a *single reviewable in-workspace native OpenCode read*. This is **not** a guarantee of all read/edit/shell operations being intercepted.

- `fcb51db` approved the documents, `1bc60f6` implemented forward-only Agent schema v10 approval ledger and bounded ACP callback handling, `1631944` added Wails-owner-only approval controls, `629ad81` injected the fixed OpenCode v2 permission overlay, `7f7a922` limited allow-once to reviewable in-workspace reads, `f67d42b` added strict ACP `rawInput.path` / `locations` agreement, task request budgets, current grant rechecks and owner rejection after Profile revocation.
- The SQLite v10 ledger is keyed to TaskID, AgentID, generation, ACP SessionID and toolCallId; no prompt, diff, command, rawInput, file contents or full reasoning is written to it. Approvals only transition pending→allowed_once/rejected/expired; resolved records are immutable; all old pending rows expire on restart. Limit: two concurrently pending requests, 64 sequential requests per Task, two-minute approval wait, bounded by the existing ten-minute Task timeout.
- Wails Task detail presents the sanitized action class, only a relative local target, expiration and **Allow once / Reject**. It never enables `allow_always`. The Web/HTTP SDK does not offer operational approval authority, and no Profile-visible MCP approval Tool was added.
- The backend allows an affirmative decision only for a regular file within the already-admitted, canonical workspace, with matching ACP rawInput.path and locations path. It rechecks the lexical path before deciding. Unknown, external, changed-symlink, missing, compound, edit and shell actions remain reject-only. These consistency checks are conservative previews of **untrusted Agent metadata**, not forced interceptors of native filesystem operations.
- Explicit AgentGrant, Profile workspace grant, Task generation, active state and current Policy are rechecked before allowing. The owner can still *reject* after revocation (which never restores permissions). Cancellation, timeout, Task termination, wrong SessionID/ToolCallID, malformed options, duplicates and session restart fail closed.
- The real installed **OpenCode v2.0.30** was exercised on a disposable file through ACP v1 and the fixed overlay; it emitted a `read` permission request offering `allow_once / allow_always / reject_once`. The reported `rawInput.path` and `locations` matched the same disposable file, the test client returned **cancelled**, the Task stopped and the file remained unchanged. No real file operation was approved, no user workspace was accessed.
- Offline production snapshot: the actual live v9 Agent SQLite was backed up **read-only** and upgraded using the newly built CLI to v10. Fingerprints across **10 resource/relationship tables** matched the original database; zero FK/integrity errors; new approval table initially empty. The user's live DB is still v9. No user resources or secrets were modified.

**Unfinished D gate — explicitly NOT enabled:** reviewable edit diffs, safe shell and Mender-authoritative native tool confinement. OpenCode Host mode is still ordinary user access, OpenCode project/agent/plugin configuration and saved permissions may affect its effective policy, and the ACP metadata is not trusted execution input. An approved single-file read does not demonstrate a mandatory filesystem boundary. In particular, runtime approval still does not independently recheck every upstream Profile ToolBinding mutation once a Task was admitted; this must be decided/fenced before advertising a stronger cross-resource governance guarantee. True source write/shell requires a vetted Sandbox placement and/or execution through Mender's canonical governed Workspace Tools with no bypass. Do not switch to host fallback, autoapprove or add allow-always.

**Rollout:** Wails native app and the user's MCP Tunnel are intentionally left running. The new native Agent v10 migration, command environment overlay and owner approval endpoints will only run after a controlled Wails restart and separate owner acceptance. The independent local binary/fixture tests must not be represented as live production enablement.

### Final quality gate and rollout receipt

- The final, full **`pnpm check` returned exit code 0** after all Phase 44 B/C/read-only hardening commits. The local receipt is `/tmp/mender-phase44-final-check.exit`, with detailed output in `/tmp/mender-phase44-final-check.log`.
- Go unit/integration tests and the entire backend Race Detector passed; Go lint, TypeScript strict, ESLint, OpenAPI-generated SDK parity and Wails bindings/desktop architecture checks passed. i18n: **1263/1263**. Vitest: **44 files, 253 tests passed**. Web and Desktop production builds succeeded; Vite chunk-size advisories are nonfatal and unrelated.
- Actual OpenCode v2 read-ask was revalidated in a fresh disposable directory with the shipped V2 `OPENCODE_CONFIG_CONTENT` overlay: the Agent reported exactly one read approval, its `rawInput.path` and `locations` matched the same temporary file, it offered allow_once/allow_always/reject_once, and the test client replied **cancelled**. ACP stopped and the file remained unchanged. No affirmative owner approval was given to the real OpenCode binary.
- A newly compiled CLI reopened a **private, read-only backup** of the user's live schema-v9 Mender database, migrated only that copy to v10 and compared 10 protected resource table fingerprints against the live original. All matched; foreign keys and `quick_check` passed. Live schema remained v9 with original Agents/ProviderIDs/ToolIDs/Tasks/Grants and no newly stored approvals.
- The current Wails native runtime and connected MCP Tunnel were intentionally not restarted; the source commit exists but new native approval methods will run in the user's live app **only after a controlled owner restart**. Do not claim live UI mouse-click approval acceptance yet.
- **Remaining blocked gate:** a true executable, mandatory Mender-governed edit/shell route (including Profile ToolBinding revocation and prevention of native OpenCode bypass), and explicit local owner approval of a disposable read/edit under that route. This is a separate enforcement implementation check, not satisfied by the cooperative ACP outcome.

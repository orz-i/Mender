# Phase 44 research — ACP approvals and governed OpenCode tools

- Status: research complete, **implementation pending explicit go-ahead**, 2026-10-17 (Mac local).
- Authority: [ADR-0050 — ACP approval mediation](../architecture/adr/0050-acp-agent-tool-approval-governance.md), Proposed.
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

**No code, database schema, local Agent permissions or user files were modified in this research phase.** Proposal approval is needed before implementation.

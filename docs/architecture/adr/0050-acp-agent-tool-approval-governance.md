# ADR-0050 — ACP approval mediation and governed Agent tool execution

- Status: Accepted for staged Phase 44 implementation (owner approved 2026-10-17). Individual capabilities remain unavailable until implemented and accepted.
- Date: 2026-10-17 (Mac local).
- Refines: ADR-0038, ADR-0034, ADR-0035, ADR-0047, ADR-0049.
- Mender remains a platform for aggregating, governing and supplying tools and resources to LLM/Agents. OpenCode retains the Agent Loop, model inference and choice of actions.

## 1. Verified evidence

Repository:

1. The production Agent adapter (backend/internal/agent/infrastructure/acp/runner.go) implements ACP v1 over stdio and launches one new process/session per Task. The initialize clientCapabilities are empty, session/new.mcpServers is an empty list. The current message pump discards tool-call updates and retains only bounded agent text.
2. stream.respondToAgent cancels session/request_permission and refuses all client-side filesystem/terminal methods with -32601. No ACP approval request store, operator decision endpoint, approval queue, action preview or waiting-for-approval state currently exists.
3. Agent service has durable TaskID and immutable model/effort/workspace snapshot, owner and Profile/AgentGrant/Policy admission, queued/running/terminal lifecycle, 10-minute timeout, generation fencing and unknown-outcome recovery. Policy has a require_approval decision variant, but Agent admission currently rejects anything other than allow; this is NOT an ACP owner-approval implementation.
4. Agent launcher starts trusted opencode acp (local OpenCode v2.0.30), with explicit cwd and chosen isolation. In host mode this is an ordinary host-user child process, not a file/network sandbox. Restricted-host can prevent the CLI accessing its own authentication/configuration, and no fallback is permitted.
5. Running Wails/MCP Tunnel and the user's original OpenCode Agent are connected. The local OpenCode agent is currently host-isolated, with a configured model and an explicitly granted extra workspace. A prior requested local repository read through Mender ended with generic Agent execution failed; the exact root cause was not established. Prompt-only and workspace-selection calls succeeded. Do not attribute the failed read solely to permission mediation without diagnostics.

Official specifications (research sources):

- ACP v1 Tool Calls and Requesting Permission: https://agentclientprotocol.com/protocol/v1/tool-calls
- ACP v1 Initialization: https://agentclientprotocol.com/protocol/v1/initialization
- ACP v1 File System: https://agentclientprotocol.com/protocol/v1/file-system
- ACP v1 Terminals: https://agentclientprotocol.com/protocol/v1/terminals
- ACP v1 Cancellation: https://agentclientprotocol.com/protocol/v1/cancellation
- OpenCode v2 ACP adapter: https://dev.opencode.ai/v2/docs/cli/acp/
- OpenCode v2 Permissions: https://dev.opencode.ai/v2/docs/permissions/
- OpenCode v2 Config and policies: https://dev.opencode.ai/v2/docs/config/ and https://dev.opencode.ai/v2/docs/policies/

ACP v1 permission requests carry sessionId, toolCall metadata, and offered options; an affirmative reply echoes ONE ACTUALLY ADVERTISED optionId. Cancellation must return the cancelled outcome. A permission decision is NOT itself the execution of the tool; the Agent performs its own action. Optional fs/read_text_file, fs/write_text_file and terminal/* are independent client-execution methods, which must not be advertised before Mender actually implements them.

Official OpenCode v2 ACP adapter offers option IDs once (allow_once), always (allow_always), reject (reject_once). Permission request diffs may be included as preview content, and requests by subagents carry child-session metadata. The adapter loads the project's OpenCode configuration (including agents, plugins, native tools and MCP servers) from the session cwd. Its client-provided MCP servers persist in the directory for the lifetime of that ACP process, not only the current prompt.

OpenCode v2 permissions are rule-based; its BASE rules default to allow for most actions, and external_directory is allowed by default. read/edit/shell and related actions only ask when effective rules say ask. The latest OpenCode v2 config documents system/global/project precedence and project-scoped saved always-approvals. The V1 permission/bash/task nomenclature is NOT valid as an implementation guide for V2 permissions/shell/subagent. User/project rule precedence and remembered approvals must be verified with the actual installed version.

## 2. Explicitly separate two trust boundaries

### A. ACP approval mediation — cooperative native Agent tools

Mender can accept bounded session/request_permission requests from its child Agent, wait for an authenticated owner, present sanitized action/target/diff information, re-check authorization, and return the selected offered option. In this mode OpenCode still executes its OWN native read/edit/shell actions. Mender governs only actions that the Agent actually asks about. Tool-call notifications, rawInput, title, path and child-session metadata are UNTRUSTED hints, never evidence that the tool did exactly what it reported.

Only allow-once and reject choices are supported initially. Never select allow-always, even when offered; OpenCode v2 may save broad project-scoped rules that outlast current Task/AgentGrant/Policy. Never auto-allow when UI is absent. If the effective OpenCode permissions cannot be forced to ask under this user's project configuration, mark host execution cooperative only and do not claim authoritative protection.

### B. Authoritative Mender execution — fully governed resource operations

To promise that all code reads, edits and shell commands are governed by Mender, either:
- The actual operation executes through Mender's canonical Workspace Tools/Invocation Policy/Sandbox router and grants, with an approved Task-scoped identity and no native bypass; OR
- A verified process/container isolation boundary prevents OpenCode from bypassing the claimed filesystem/network restrictions.

Simply advertising ACP client fs/terminal capabilities does not redirect OpenCode native tools to those methods. If Mender implements ACP fs/* and terminal/* methods, they must use task-bound canonical roots, host-owned executors, Profile grants, budgets and validation before advertising the capability. OpenCode may continue choosing its native tools, so process isolation and tested enforcement remain mandatory.

Mender may supply task-scoped MCP tools through ACP session/new.mcpServers in a later stage, using a short-lived scoped token; avoid exposing the upstream Profile's delegate/status/result/cancel back to its own Agent (recursive self-delegation). OpenCode's directory-wide MCP server lifetime and child subagents must be covered. Agent-local workspace registration never automatically implies a Sandbox or tool grant. A stronger execution placement is an explicit selection for users who require that protection, not a mandatory Sandbox for every Agent.

## 3. Minimal ACP approval contract

Proposed sequence:
1. Upstream Profile Tool invokes delegate. Mender admits AgentTask using exact AgentGrant/Policy/Profile, model, effort, workspace and generation.
2. ACP child session starts in that admitted cwd, under its chosen process isolation. OpenCode chooses a tool and its permission policy resolves ask.
3. Mender validates a bounded session/request_permission: same live transport/session, TaskID, generation, toolCallId, offered option IDs, canonical workspace, and permitted request count/size. A tool-call notification ALONE never creates an approval.
4. Persist pending ApprovalID scoped to TaskID/generation/AgentID, ACP session/toolCallId and optional child-session identity. Persist short, sanitized action metadata, expiry, policy digest and request hash; NOT full prompts, secrets, raw arguments, unrestricted diffs or reasoning.
5. In Mender Agent → Tasks → Task detail, show Awaiting approval in a focused owner-only action preview. Buttons: Allow once and Reject. The approval UI is not placed as a full form on a listing view.
6. Only a trusted local owner (Wails or owner-authenticated management, never tunneled Profile MCP) may atomically CAS pending → approved_once/rejected. Recheck Task liveness, generation, origin, Policy, AgentGrant/Profile grant, workspace binding and expiry immediately before finalizing. Approval cannot upgrade deny or forged metadata into a trusted Principal.
7. Return the exact ACP optionId from the live Agent's offered options, never a guessed value. OpenCode continues its OWN Agent Loop and tool execution. Mender observes completion, cancellation or failure and stores data-minimized evidence.

The ACP read pump must NOT synchronously block on the owner UI, or it would stall cancellation, other ACP notifications and concurrent pending requests. Refactor a bounded message pump, pending-request registry and nonblocking decision handler, with per-Task concurrency limits, duplicate ID rejection, request expiry, limited diff/command previews and data-minimized Audit.

Existing TaskStatus remains running while a live pending approval is visible as a DERIVED substatus. The waiting child still consumes the 10-minute Task budget; approval expires at or before the Task deadline. Cancellation/shutdown invalidates pending approvals and replies cancelled where possible. If side effects may have occurred before a process crash, retain unknown; never replay automatically after restart. A disconnected ChatGPT tunnel is not an approval or a reason to auto-replay.

## 4. Permissions, policy and UX invariants

- No Profile/ChatGPT model call may approve its own tool request. Do not publish an approval Tool in the delegated Profile.
- An ACP allow-once response does not replace Profile AgentGrant, workspace grants, ToolBinding, Policy or resource lease. Policy require_approval and ACP tool-use approval are different checkpoints; do not treat an ACP approval as an unconditional override.
- Never infer permission from toolCall title/kind/rawInput alone. Use immutable Task/transport identity, sanitized bounded display values and a backend-specific effective policy for native Agent actions. Authorization for a Mender-owned tool must use its canonical ToolID and actual execution parameters.
- A request to approve shell activity from an unisolated OpenCode host process cannot guarantee protection of other host files. Its command/path scanner is best-effort. State the effective trust level visibly.
- No allow-always, wildcards, process auto-approve flag, unsafe shell template, auth token copy or inherited credential sharing in this first phase.
- No introduction of Mender Agent Loop, prompt planning, chain-of-thought capture, nested Agent orchestrator, generic arbitrary Agent executable, or automatic retry after unknown state.

## 5. Rejected shortcuts

1. Returning allow-once for every session/request_permission request: policy bypass.
2. Auto-selecting allow-always to make coding work: durable overly broad resource grant.
3. Advertising fs/writeTextFile or terminal:true while continuing to reject methods: false capability claim.
4. Assuming the presence of a work directory or a friendly tool_call event means all native actions are constrained.
5. Reclassifying OpenCode's internal read/edit/shell as Mender Workspace Tools without routing the actual execution through Mender.
6. Reusing the upstream MCP Profile identity or credentials to grant the subordinate Agent owner approval rights.
7. Retrying interrupted tool operations after a restart without the executor proving an idempotent result.
8. Overcomplicating the UI with Provider internals or forms in the main Agent listing.

## 6. Acceptance gates before real project writes

- Synthetic ACP fixture must prove approved once, declined, cancelled, timeout, multiple concurrent requests, mismatched IDs/generations, stale/revoked grant, process restart, malformed metadata, unknown client requests, bounded previews and no stale replay.
- In a disposable test directory, verify actual installed OpenCode v2 EFFECTIVE permission rules: shell/edit/read and external_directory ask or deny as expected, project config and subagents cannot silently broaden beyond reviewed bounds, always is not selected. If these checks fail, do not label native host operations governed.
- Test real no-secret file read and single disposable file edit only after the local owner explicitly approves. Declined edit/command must have ZERO side effects. Symlink traversal, directories outside admitted root, forbidden secrets, double invocation and self-delegation must all fail.
- Strong-governance acceptance additionally demonstrates that native tools cannot bypass the Mender-governed execution path or verified Sandbox boundary.
- Full pnpm check, Go Race Detector, HTTP/OpenAPI/SDK/Wails parity, invariant Agent/Provider/ToolID/grant checks, tunnel disconnect, cancellation and restart/unknown tests.

No real coding authorization was enabled during this ADR research.

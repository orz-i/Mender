# ADR-0038: Govern delegated Agent CLI tasks as a typed execution resource

- Status: Accepted for Phase 33
- Date: 2026-10-11
- Scope: host-owned Agent integration, not an Agent loop

## Context and identity

Mender is a governed tool/resource service for external LLM/Agent clients. Its existing `provider.tools`, `sandbox.backend` and `tunnel.transport` extension kinds are **not** general-purpose Agent sessions. Provider tools are one canonical invocation, Sandboxes are execution placement, and Tunnels are downstream reachability. Codex and Grok implement their own Agent loops, tool requests, progress and mutable sessions. Representing those runtimes as Sandbox drivers or generic MCP Providers would conceal different lifecycle, isolation, approval, and accounting boundaries.

The repository already has host-owned plugin inventory (ADR-0037), Profile-scoped Invocation, Principal/Policy, Sandbox admission and explicit delegation, Runtime process isolation and budgets. The existing MCP adapter explicitly declines MCP Tasks and elicitation/input-required requests; long-running tasks cannot be made reliable by increasing the synchronous Tool timeout.

## Decision

Add **`agent.backend`** as a fourth typed extension kind and a separate Agent bounded context.

- A Plugin Package contributes one versioned Agent Backend implementation. A configured **Agent** is a stable resource pinned to the exact built-in extension or installed package; it contains implementation-specific settings, a host-selected execution target, bounded permissions and budgets. A package is not a resource, and an Agent is not a Provider.
- An **AgentSession** identifies one implementation-native conversation if supported, with its owner, Agent binding, protocol and persistence/recovery flags. A session is not automatically a task or proof that a process remains alive.
- An **AgentTask** identifies one durable delegated request: AgentID, immutable trusted Principal/Origin/Scope, owning Profile if any, request correlation, creation/terminal timestamps, bounded status, output/diagnostic references, requested cancellation, observed process/session result and a monotonic generation. A task is not assumed complete when its prompt is acknowledged.
- An **AgentGrant** permits one immutable ProfileID to delegate to one immutable AgentID. Policy `tool.invoke` and the agent-specific admission remain mandatory. A Profile's service Principal must not impersonate the local owner, and a TaskID alone is never a read/cancel authorization token.
- A host-owned **Agent Delegation Provider** publishes canonical Tools for permitted, explicitly configured Agent resources and task status/result/cancel operations. Its provider is a Tool facade, not the Agent Backend's lifecycle owner. The Tool schema never accepts arbitrary executable paths, `sandboxId`, plugin installation IDs, arbitrary env vars, or command-line flags as model-controlled values.

A configured Agent has an immutable host-validated backend reference and an execution placement. Phase 33 starts with a **locally installed, host-reviewed CLI** on a trusted, explicitly selected local Workspace. It must not silently fall back from Docker, SSH or remote execution to the host; those targets remain unsupported until true bidirectional stdin/stdout and process ownership are delivered by a Sandbox backend.

## Protocols and execution boundaries

- **Agent Backend → process:** prefer version-pinned **ACP over stdio/JSON-RPC**, with capability negotiation, `initialize`, `session/new`, `session/prompt`, `session/update`, permission requests, cancellation, and bounded reads. Grok advertises `grok agent stdio`. Codex can use the separately installed `codex-acp` adapter (which drives the Codex App Server); the presence of `codex` alone is not proof that an ACP adapter is installed.
- ACP is a **transport**, not an authorization system. Host capabilities, allowed commands, working root, process launch isolation, resource budgets and permission decisions are all host-owned. Initial unattended mode **rejects or cancels permissions** instead of auto-approving. A later owner-mediated approval interface must bind a request to a live Task/Session and a specifically authenticated operator.
- **Upstream → Mender:** publish delegated operations through the existing Profile-scoped MCP Tool/Invocation path. Internal Task persistence is protocol-independent. When negotiated and implemented, MCP `io.modelcontextprotocol/tasks` projects the same persisted task; while not negotiated, regular `task.status`/`task.result`/`task.cancel` Tool facades provide a controlled polling path. The MCP adapter must not claim Tasks extension support until it actually implements its methods and conformance checks.
- A2A remote Agents are deferred; they require an explicit remote identity/authentication and task/artifact schema. No A2A endpoint is inferred from an ACP CLI.

## Admission order and safety invariants

1. Resolve only inert Agent/Plugin/Profile metadata, verify exact installation binding and current state.
2. Validate Tool arguments/schema, Profile membership and AgentGrant (for delegated MCP), then evaluate policy at `tool.invoke` and `agent.task.submit` / `agent.activate` as applicable. **Before** resolving credentials, constructing a process, or modifying workspace content, check execution target ownership/delegation and acquire a durable lease. A denied request has no agent-side effects.
3. Persist a task with trusted origin, Principal, Scope and generation; atomically reserve concurrency budget/lease; start a host-selected executable through the Runtime process launch boundary with explicit minimal environment. Native Agent CLI may have its own user credentials; absence of a verified executable or auth is `unavailable`, not silent downgrade to a different engine.
4. Observe ACP messages and permission requests, redact/truncate progress, store bounded public diagnostics only. Full prompt, secrets, raw tool args, model chain-of-thought and complete streams do not belong in global Audit.
5. Cancellation is an intent, not a terminal fact. Send ACP cancel (if supported), then bounded process termination; persist confirmed outcome or `unknown` after uncertain execution. On restart, reconcile interrupted tasks before accepting replay; **never** replay filesystem-affecting work merely because a process disappeared.
6. Verify exact Principal/Scope/AgentGrant or local owner before status/result/cancel access; return stable canonical TaskIDs, never OS process IDs or raw implementation SessionIDs to untrusted callers.
7. Agent recursion is constrained: do not automatically mount Mender MCP tools back into the subordinate CLI; when enabled explicitly in a later phase, propagate an independently bounded delegation depth and prevent self-call cycles.

A local process, even a child with an ACP wire protocol, is not a security sandbox. A Restricted Host mode is not a Docker VM, and CLI-specific sandbox options are not proof of host enforcement. Do not launch plugins/Agent processes from read-only catalogue queries or autodiscovery. The runtime must avoid untrusted `PATH` precedence, native package auto-download, unknown binary flags or unrestricted inherited environment.

## Lifecycle and capability boundaries

Initial task lifecycle: `queued → running → completed|failed|cancelled|unknown`, with nonterminal `waiting_approval`/`waiting_input` only when a working trusted response path exists. An approval request with no response path fails closed. Terminal states are immutable. Process exit ≠ Task success; prompt acceptance ≠ completion; cancellation signal ≠ confirmation. Store one bounded result and data-minimized event digest rather than unbounded transcripts.

Task budget is distinct from canonical Invocation's 120-second/default 1 MiB synchronous limits; the submitting Tool returns promptly with TaskID. Rate/concurrency/TTL policies must apply to Agent, Principal and Profile. The owner may explicitly delete/revoke an Agent only when active tasks and sessions have been drained or safely reconciled, and a pinned package may not be disabled while used.

## UI / contract and migration

Keep the established one-information-type-per-viewport design. Add **Agents** under Runtime between Sandboxes and Profiles. Lists are previews with unified action row; separate Agent details display Overview, Access, Tasks, Sessions and Diagnostics. Create via Marketplace `agent.backend` selected Add action, with a focused backend → local execution target → governance → review flow. Discovery is inert and does not run an Agent or resolve credentials. No plugin-provided executable UI or arbitrary raw-form injection.

HTTP/OpenAPI, Go CLI, typed SDK and Wails/Desktop must have consistent input shapes, authentication, state and errors. New persistent tables use a forward-only migration with no legacy readers. Preserve all existing provider/sandbox/tunnel/plugin installation identities, no compatibility bridges, and no changes to the Skill/plugin dot directories.

## Rejected alternatives and deferred work

- **Put an Agent inside `provider.tools` only:** hides Task/Session lifecycle and distinct approval/lease requirements. Keep the Provider solely as an upstream Tool facade.
- **Treat Agents as Sandboxes:** confuses a working process with placement/isolated filesystem.
- **Expose a generic `run(command, args, cwd)` Tool:** allows model-controlled code execution and bypasses host-owned CLI selection.
- **Make Mender a planning/coordinating multi-Agent harness:** explicitly outside Mender's product scope.
- **Automatically install codex-acp, bypass approvals, import CLI auth tokens or access live user projects during tests:** disallowed.
- **Treat model/tool events as trusted Principal, approvals or policy facts:** disallowed.
- **Full MCP Tasks v1/A2A/Sandbox-bidirectional protocol before core delegated-task governance is proven:** separate acceptance gates, no fictitious support badges.

## References

- [ADR-0037: Typed plugin platform](0037-multi-kind-plugin-platform.md)
- [ADR-0032: Principal / Provider admission](0032-provider-activation-principal-governance.md)
- [ADR-0034: Runtime process isolation](0034-runtime-owned-process-governance.md)
- [ADR-0035: Sandbox execution router](0035-governed-sandbox-execution-router.md)
- [ADR-0036: Profile Sandbox delegation](0036-sandbox-profile-delegation.md)
- https://agentclientprotocol.com/protocol/overview
- https://github.com/agentclientprotocol/codex-acp
- https://modelcontextprotocol.io/specification/2026-07-28/basic/utilities/tasks

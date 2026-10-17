# ADR-0051 — Task-bound Agent access to canonical governed tools

- Status: Accepted for staged Phase 45 work (2026-10-17).
- Refines ADR-0035 (Sandbox Execution Router), ADR-0038 (governed Agent Delegation), ADR-0047 (Agent workspace reuse), ADR-0049 (owner-approved local roots), ADR-0050 (ACP approvals).
- Mender supplies **governed tools and resources**, not an Agent Loop. Every downstream Agent (OpenCode, Grok, Codex or future backend) retains its own inference, plan and Agent Loop. ACP is **one protocol adapter**, not the governance domain or a mandatory property of all Agents.
- Existing AgentID, ProviderID, ToolID, TaskID, Profile ToolBinding, AgentGrant, Policy and Sandbox execution identities are authoritative; do not create a second tool inventory.

## Why another boundary is necessary

Phase 44's real Wails approval and single-file read both completed. They proved cooperative approval for an **ACP-compatible Agent**, not mandatory native-tool mediation. Any native host Agent (including OpenCode, Grok or Codex) might execute built-in read/edit/shell independently of its protocol callbacks; neither an ACP permission answer nor task-scoped MCP tool injection can by itself force native actions through Mender. Never label a host Agent session with only an approval overlay "fully governed". Backend-specific configuration, plugins and subagent behavior must be assessed independently.

Current Mender Workspace Tools (read/search/edit/shell) already run through Profile ToolBinding → canonical InvokeTool → Policy/Approval → Sandbox Execution Router → Lease → backend → bounded result/Audit. This existing path, rather than a new filesystem or shell implementation, is the execution authority to reuse. The ability to consume supplied tools is negotiated **per Agent adapter/session**: ACP v1 requires stdio MCP and optionally advertises HTTP/SSE, while different non-ACP adapters may use other surfaces or not support injected tools at all.

## Decisions

### 1. Canonical task-bound capability gateway (first deliverable)

- Create a **per-active-Task**, runtime-owned, randomly generated opaque capability. Do not use a user-supplied TaskID, ToolID, ProfileID or ACP-supplied metadata as a standalone authorization token. The capability exists only in the owning runtime memory and expires with the active Task; no replay across process restart.
- Admission must verify original Task status=running, `cancel_requested=false`, exact generation/AgentID/approved root, Profile origin, the current AgentGrant, Profile workspace grant (when an alias is chosen), and an explicit **allowlist of canonical existing Workspace ToolIDs**. This allowlist is selected by trusted owner/Runtime configuration, not tool arguments.
- Every call re-fetches the immutable Task snapshot and active grants and executes via the current Mender canonical `InvokeTool.Execute` with the original Profile Principal/Scope. This rechecks current ToolBindings, Policy, Runtime Budget, binding execution guard, Sandbox resource authority, lease and Audit **for each real operation**. Do not invoke the Sandbox backend directly. The gateway must fail closed if any grant/binding/policy/Task state changed, when the tool is outside the allowlist, or when the task expired/cancelled/closed.
- For this first gate, only `workspace.read`, `workspace.search`, `workspace.edit` are eligible; the `workspace.shell` operation remains denied until process/network isolation is separately verified. No Agent Delegation, approval, credential-management, arbitrary shell or profile mutation ToolID can be called recursively.
- Any bound Sandbox Tool must refer to **the exact admitted Agent Task workspace root** (for a local workspace placement), with ready state and `restricted_host` isolation for the eventual strong mode. No automatic creation or implicit binding of a Sandbox when the user adds an Agent local workspace. An explicit optional governed-tool placement may be configured separately by the owner.
- Gateway arguments and responses stay bounded by the existing Runtime Budget and Tool schema. No new Executor, adapter-owned permissions, duplicate ToolIDs, parallel Tool Router or hidden Profile grant.
- The capability must never be published through normal Profile MCP, and it must not become a durable wildcard permission. Prefer ephemeral loopback or stdio transport with task-bound control and cross-session replay resistance.

### 2. Enforced Agent process placement (mandatory before strong mode)

- The subordinate ACP process must not be able to write the protected project root by its own native tools. Require an owner-selected **verified** restricted/container placement that refuses native write, escape via symlink, unapproved host home credential access and external-root writes. No fallback to `host`.
- A macOS `restricted_host` Seatbelt profile can restrict project writes, but currently permits network and temp-file writes and may block OpenCode's native authentication; it is **not** a full VM/container. Validation of exact protected-root writes is meaningful, but host-wide or network containment is not established by that test alone.
- If a Docker/remote Sandbox is used, its workspace mount, OpenCode binary/runtime, scoped authentication and process lifecycle must be verified explicitly; native host OpenCode cannot automatically run inside a Linux Docker engine. Do not download random CLI packages or inject user's local credentials to make integration tests pass.
- Do not expose a "governed mode" toggle until native bypass tests prove its advertised guarantees. Continue reporting Host execution as cooperative ACP approval only.

### 3. Agent-neutral ToolSupply port and protocol-specific transport adapters (after the gateway)

- The **Agent application** receives only a task-scoped, protocol-neutral `ToolSupply` description (canonical ToolIDs and invocation capability); it does not know or assume ACP, HTTP, stdio, Grok, Codex or OpenCode. An adapter exposes an explicit support result for `tools`, `permission_approval`, and `placement`; unsupported capabilities fail closed, never silently fallback.
- For **ACP v1** specifically: negotiate `agentCapabilities.mcpCapabilities.http` during initialize before injecting a short-lived, host-owned loopback HTTP MCP server through `session/new.mcpServers`. Do not assume all ACP Agents support HTTP; v1 stdio is mandatory but a separately reviewed stdio runtime adapter is required to support it. No accidental upgrade to ACP v2; protocol version and capability schema are explicit.
- OpenCode is the first tested adapter, **not the owner of this contract**; Grok/Codex integration must be tested against their actual protocol capabilities and may remain unsupported until verified.
- MCP server must have no recursive Agent Delegation or owner approval Tool; the service is process/Task scoped and expires on cancel, timeout, restart, grant revocation or Sandbox retirement.
- Tool descriptors must derive from canonical Catalog metadata. Do not wrap Workspace Tools again in arbitrary backend-specific Tools or let the Agent pick any source Provider/ToolID in the gateway.
- Redact credential values, raw execution arguments from global audit, and reject oversized JSON, unknown ToolID, cross-Profile call, stale token and confused-deputy origin.

## Four fixed implementation checkpoints

A. Docs and tests contract (this commit).
B. Task-capability gateway, exact ToolID/root/grant revalidation and canonical InvokeTool execution. Tests must include a real local Workspace Sandbox and Profile ToolBinding using an **isolated disposable SQLite database**, negative stale/revoked/unbound/wrong-Task/recursive calls, and no new ToolIDs.
C. Generic ToolSupply and capability-negotiated Agent adapters (ACP v1 HTTP first; stdio/non-ACP as supported), plus fail-closed process placement preflight. Test a disposable Agent/fixture and protocol/tool discovery; unsupported Agents must not receive an implicit host fallback. Do not enable live Host integration as "governed".
D. Verified process confinement, isolated OpenCode read/edit/shell acceptance, explicit operator approval for real file modification, `pnpm check`/Go race, preservation of production identities and controlled Wails restart. If confinement or scoped authentication cannot be demonstrated, **stop at the proven checkpoint** rather than silently falling back.

## Source evidence

- Mender: `backend/internal/invocation/application/invoke_tool.go`, `backend/internal/workspace/infrastructure/plugin/plugin.go`, `backend/internal/sandbox/application/lease.go`, `backend/internal/agent/application/service.go`, `backend/internal/agent/infrastructure/acp/runner.go`, `backend/internal/runtimegovernance/infrastructure/processisolation/launcher.go`.
- Official ACP v1 `session/new` MCP capabilities and HTTP/stdin descriptors: https://agentclientprotocol.com/protocol/v1/session-setup
- Official OpenCode tools/permissions/config: https://docs.opencode.ai/docs/tools/ ; https://dev.opencode.ai/v2/docs/permissions/ ; https://docs.opencode.ai/docs/config/

No production user resources are created/modified by checkpoint B; all test writes must be inside disposable directories. Historical Agent migration versions and ACP approval ledger are retained.

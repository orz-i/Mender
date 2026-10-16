# ADR-0047 — Agent runtime configuration and reusable workspace targets

- Status: Accepted for staged implementation
- Date: 2026-10-16
- Refines: ADR-0038, ADR-0035/0036, ADR-0039 and ADR-0046

## Problem and evidence

The current Agent resource stores `workspace_root` and `isolation` in `agents`; `AgentService.Submit` and the managed `agent.delegate` Tool accept only a prompt. The ACP v1 runner opens a fresh session for each task with the Agent's fixed root and no session configuration, so switching model or project requires a duplicate Agent. Workspace Tools are similarly bound to exactly one Sandbox through Provider config v2 `sandboxId`, although a caller needs the same four Tool definitions for multiple allowed workspaces. This duplicates managed Providers, Tool IDs and Profile bindings instead of reusing the capability.

ACP v1 session creation uses `cwd` and may advertise `configOptions`; `session/set_config_option` can select a model using the Agent-advertised model selector. These are *Agent-owned session settings*, not a license for Mender to implement model inference or Agent reasoning. ACP v2 is a separate protocol/version negotiation effort and **is not** silently accepted by this change.

## Decision

1. **Stable Agent identity:** Keep AgentID, Backend/Plugin installation pin, managed Agent Delegation facade, ProviderID, ToolIDs and Profile AgentGrant independent from models and projects. Existing IDs and live Profile bindings survive the change. Backend and host process isolation remain owner-selected and never come from a model-controlled call.
2. **Defaults versus execution selections:** Agent stores optional default model and its existing canonical local workspace root; authorized task input may explicitly select `model` and a *registered workspace key*. No model/cwd is set by an executable flag or raw Tool command. Effective selection is snapshotted into an admitted Task before execution; changing defaults never retargets an accepted Task. Nonempty selected model must be *actually offered* in the new ACP session's `configOptions` model category/identifier and confirmed by `session/set_config_option` before `session/prompt`; unsupported/unavailable selection fails closed. Empty model means the Backend's default; no fabricated model catalogue or probing during list/read.
3. **Workspace registry:** Reuse existing *local_workspace* Sandbox identities for additional Agent workspaces, not another independent directory inventory. An owner explicitly binds an Agent to a local workspace Sandbox under a short, unique key. Default `workspaceRoot` remains supported for existing Agents and is the implicit default selection; additional keys resolve only through the binding. Reject non-local Sandbox backends, owner mismatch, unavailable/destroyed targets, unsafe canonical paths and remote CLI execution. No silent host fallback or root substitution.
4. **Separate delegation authority:** An AgentGrant alone authorizes the Agent default workspace as before; added workspace choices need **additional explicit Profile workspace grants**. The profile key, AgentID and workspace selection must be rechecked atomically with durable Task admission. Model input never sets trusted Principal/Scope. Owner-only edits to Agent defaults/workspace bindings cannot modify a running Task's immutable effective selection. A host CLI launched in an explicitly selected local directory is *not* a Sandbox workspace lease; do not imply Docker/SSH or stronger security guarantees from an attached Sandbox record.
5. **Reusable Workspace Tools:** Keep one selected Workspace Provider/managed facade and its four canonical ToolIDs, allow its owner to bind additional named Sandbox targets. An optional `workspace` argument selects only among registered targets; omitted uses the existing pinned default `sandboxId`. At execution resolve the exact provider, target and operation, check existing Sandbox Profile capability grants/Policy, acquire the actual Sandbox lease and operate on that same filesystem. Governance facts must include the selected target identity as well as relative paths; a model-supplied raw Sandbox ID, absolute root, executable, credential or arbitrary endpoint is not accepted. No implicit cross-Provider target sharing.
6. **Presentation:** One Agent overview/defaults area with a focused edit dialog; a focused Workspaces tab previews granted targets and provides owner-only add/remove and Profile access control. Delegation schema displays optional model and workspace selection without additional Tool instances. Workspace Tools remain four stable Tools in the existing unified Tools source/detail view; choosing a target does not expose Provider/Facade internals. Keep single-topic viewports and i18n.
7. **Process and lifecycle:** No Agent loop, prompt rewriting, session persistence/replay, self-mounted MCP tools or automatic approvals. Preserve ACP v1 fail-closed permission handling. Do not run paid model calls during tests, and do not silently launch Agent CLIs during listing. No automatic resume/replay after uncertain failure. No compatibility dual-readers: any stored shape migration is forward-only, validated and tested with old live identities.

## Ordered implementation and acceptance

- **A — Contract / ADR (docs-only commit).** Record invariants and protocol references before code.
- **B1 — Agent selection core.** Persist owner-managed default model and registered local workspaces; introduce typed SubmitOptions and immutable Task runtime snapshot; verify owner/Profile exact authority, path containment and concurrent grant revocation. Test no process side effects on denial.
- **B2 — ACP + public surfaces.** Apply v1 advertised model selection, pass only admitted canonical local cwd to the launcher/session, update Agent Delegation input, HTTP/SDK/Wails/UI and i18n. Exercise synthetic ACP model supported/missing/rejected cases, local workspace allow/deny and end-to-end MCP delegation with stable existing ToolIDs; no real paid CLI.
- **C — Workspace multi-target.** Add owner-only target bindings for existing Sandbox Workspace Providers, schema and routing with per-target fact extraction, operation grants, leases, UI and API; migrate pinned existing links once; negative tests for cross-provider/foreign/remote targets, revocation and symlink escapes. Do not break existing ProviderIDs/ToolIDs.
- **D — Closure.** `pnpm check`, Go race/lint, TypeScript, SDK/OpenAPI/Wails parity, clean `git status` (ignoring pre-existing untracked skill directories), explicit desktop smoke when available. Task result honestly distinguishes unverified real CLI and remote execution.

## References

- [ADR-0038 Agent delegation](0038-governed-agent-delegation.md)
- [ADR-0035 Sandbox routing](0035-governed-sandbox-execution-router.md)
- [ADR-0036 Sandbox Profile grants](0036-sandbox-profile-delegation.md)
- [ADR-0046 Unified Tools workspace](0046-unified-tools-workspace.md)
- https://agentclientprotocol.com/protocol/v1/session-setup
- https://github.com/agentclientprotocol/agent-client-protocol/blob/main/docs/protocol/v2/migration.mdx

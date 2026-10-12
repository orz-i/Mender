# Agent CLI Delegation (Phase 33)

Phase 33 adds `agent.backend` alongside Provider, Sandbox and Tunnel extension kinds. Mender remains a resource/Tool governance service; Codex, Grok or another subordinate CLI owns its own Agent reasoning loop.

## What is implemented

- Built-in typed `io.mender.agent.codex` and `io.mender.agent.grok` backend descriptors; offline binary discovery in host-reviewed locations only.
- Configured Agent records with canonical local workspace root, explicit `host` or `restricted_host` execution placement, owner identity and an immutable backend binding.
- Local ACP v1 stdio client: `initialize`, `session/new`, `session/prompt` and bounded text updates. No automatic approvals or arbitrary client filesystem/terminal requests. Requesting unavailable client authority fails closed and ends the task.
- Durable TaskID and task status/result/cancel with a 10-minute hard deadline, 64 KiB bounded result, two concurrent tasks per Agent/Principal and eight global tasks. A cancellation request is separate from confirmed cancellation; uncertain shutdown/timeout/restart is `unknown`.
- Exact ProfileID–AgentID grants alongside canonical ToolBinding, Principal, Policy and Audit. Agent admission records include **no prompts, credentials, client raw arguments, reasoning or result contents**.
- Profile-scoped canonical Tool facade in built-in `io.mender.provider.agent-delegation`: `delegate`, `status`, `result`, `cancel`.
- Agent management in HTTP/OpenAPI, TypeScript SDK, Go CLI, Wails Desktop, React UI. The UI keeps list previews, a dedicated Create flow, and separate Overview, Tasks, Access and Activity views.

An Agent resource is not itself a Provider. Create a **separate, explicitly bound delegation Provider** if an upstream Profile should see Agent tools. Do not put arbitrary executable paths, environment variables or shell flags into model-controlled Tool arguments.

## Getting started in the desktop UI

1. Open **Agents** (Runtime group), select **Add agent**, choose Codex or Grok, then configure the local directory and execution isolation. Review before saving. This operation only saves configuration; it does not start a CLI or grant any Profile.
2. In the new Agent detail, verify binary detection under Overview. **Detected** means only that the executable/adapter is present, **not** that protocol, login, model, network or tool permissions are verified.
3. Use **Connect Agent tools** from the Agent detail. This opens the built-in Agent Delegation Provider form scoped to this exact AgentID. Create and synchronize the Provider to publish its four canonical Tools.
4. Open **Profiles** and bind only the Agent tools the Profile needs. Separately, open the Agent's **Access** tab and grant that ProfileID. Both ToolBinding and AgentGrant are required. Existing Policy must allow `tool.invoke` and `agent.activate` / `agent.task.submit`; a second `agent.task.cancel` decision governs cancellation.
5. Connect the upstream MCP client to an already-configured Mender Profile through the supported local or tunneled MCP transport. Submit `delegate` with a bounded `prompt`, receive `taskId`, then call `status` / `result` (or `cancel`) using that opaque ID.

This flow is validated with a deterministic fake Agent through an **actual MCP client session**, not only through direct invocation; cross-Profile access and revoked grants are negative-tested.

## CLI management

The Go CLI shares the same domain and database:

```bash
mender --output json agent list
mender agent add --name "Code Reviewer" --backend codex --root /absolute/project/path --isolation restricted_host
mender --output json agent show agent_0123456789abcdef0123456789abcdef
mender agent grant agent_0123456789abcdef0123456789abcdef prof_0123456789abcdef0123456789abcdef
mender agent grants agent_0123456789abcdef0123456789abcdef
mender agent tasks agent_0123456789abcdef0123456789abcdef
mender agent task atask_0123456789abcdef0123456789abcdef
mender agent admissions agent_0123456789abcdef0123456789abcdef
mender agent cancel atask_0123456789abcdef0123456789abcdef
mender agent revoke agent_0123456789abcdef0123456789abcdef prof_0123456789abcdef0123456789abcdef
mender agent remove agent_0123456789abcdef0123456789abcdef --confirm
```

The `--output json` flag is a root persistent flag; scripts can pass it before `agent` (for example `mender --output json agent list`). Identifiers above are illustrative and must be replaced with real IDs. Remove is blocked while Task history or a delegation Provider still references the Agent.

## Execution and security boundaries

- Codex needs the **separately installed `codex-acp` binary** as well as `codex`; Mender does not download it or run npm/npx during discovery. Grok uses native `grok agent stdio`.
- The default `restricted_host` process isolation is deliberately **not** a Docker sandbox or VM. It constrains filesystem access and may prevent existing CLI authentication credentials from being read. Do not silently retry in unisolated `host` mode. `host` must be explicitly selected by the owner, and uses the Runtime account's local authority.
- Mender uses a minimal child environment and does not automatically pass arbitrary Environment Credential values, the Runtime's MCP tools or Agent plugins to the child. No arbitrary parent directory, remote SSH or Docker execution is supported by this Agent backend.
- Directory and binary detection is passive. Credentials/login, actual task completion, external model cost, network dependency, and real-world model permissions are **not proven** by a healthy plugin card.
- ACP permission and client-side filesystem/terminal requests fail closed. Subordinate Agents may still possess their own internal Tools; the Agent's host isolation boundary remains essential. Audit does not imply a complete record of all subordinate Tool calls.
- The Task store fences generation, Profile grants and concurrency; it does not replay unknown work. On restart, an overdue task becomes `unknown` rather than being re-run.
- MCP `io.modelcontextprotocol/tasks` is **not advertised** until a separately verified transport implementation exists. The current stable fallback is Mender's four ordinary Tools. ACP session resume, remote A2A, automated multi-Agent orchestration, external Agent plugin process protocol, native UI approval continuation and Docker/SSH Agent Process streams are not part of Phase 33.

## Validation and safe testing

The supported automated acceptance exercises synthetic ACP JSON-RPC child processes and a fake Agent Executor in an isolated temporary workspace. Tests cover invalid frames/versions, unexpected sessions, oversize output, permissions refusal, cancellation, restart fencing, policy denial, same-Agent and Principal budgeting, MCP ToolBinding, AgentGrant revocation and data-minimized Audit.

Run the canonical repository gates:

```bash
pnpm check
```

For focused edits, run `go test -race ./backend/internal/agent/... ./backend/internal/bootstrap`, `pnpm i18n:check`, `pnpm typecheck`, and `pnpm test`. No real Codex/Grok prompt is executed by the automated suite; external CLI authentication and production desktop GUI smoke remain separate manual operator checks.

See [ADR-0038](../architecture/adr/0038-governed-agent-delegation.md) and [Phase 33](../planning/phase-33-governed-agent-delegation.md).

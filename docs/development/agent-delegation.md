# Agent CLI Delegation (Phase 33)

Phase 33 adds `agent.backend` alongside Provider, Sandbox and Tunnel extension kinds. Mender remains a resource/Tool governance service; Codex, Grok, OpenCode or another subordinate CLI owns its own Agent reasoning loop.

## What is implemented

- Built-in typed `io.mender.agent.codex`, `io.mender.agent.grok` and `io.mender.agent.opencode` backend descriptors; offline binary discovery in host-reviewed locations only. OpenCode uses its own native `opencode acp` executable.
- Configured Agent records with canonical default local workspace root, optional default ACP model, explicit `host` or `restricted_host` process isolation, owner identity and an immutable backend binding. Phase 40 adds explicitly registered extra local workspace selections.
- Local ACP v1 stdio client: `initialize`, `session/new`, optional advertised model selection through `session/set_config_option`, `session/prompt` and bounded text updates. No automatic approvals or arbitrary client filesystem/terminal requests. Requesting unavailable client authority fails closed and ends the task.
- Durable TaskID and task status/result/cancel with a 10-minute hard deadline, 64 KiB bounded result, two concurrent tasks per Agent/Principal and eight global tasks. A cancellation request is separate from confirmed cancellation; uncertain shutdown/timeout/restart is `unknown`.
- Exact ProfileID–AgentID grants alongside canonical ToolBinding, Principal, Policy and Audit. Agent admission records include **no prompts, credentials, client raw arguments, reasoning or result contents**.
- Profile-scoped canonical Tool facade in built-in `io.mender.provider.agent-delegation`: `delegate`, `status`, `result`, `cancel`. Phase 34 provisions/adopts one host-managed facade for each Agent without assigning any Profile.
- Agent management in HTTP/OpenAPI, TypeScript SDK, Go CLI, Wails Desktop, React UI. The UI keeps list previews, a dedicated Create flow, and separate Overview, Workspaces, Tools, Tasks, Access and Activity views.

An Agent resource is not itself a Provider. The canonical Provider remains an **internal, managed Tool facade**, created with a new Agent or explicitly adopted/repaired for an existing Agent. Do not create a second Provider manually or put arbitrary executable paths, environment variables or shell flags into model-controlled Tool arguments. An existing unique manually configured Provider is adopted without changing its ProviderID, ToolIDs, namespace or external tool names; multiple such Providers are reported as a conflict rather than silently merged.

## Getting started in the desktop UI

1. Open **Agents** (Runtime group), select **Add agent**, choose Codex, Grok or OpenCode, then configure the local directory and execution isolation. Review before saving. Creation prepares an internal governed four-Tool facade but **does not start an Agent CLI or grant any Profile**.
2. In the new Agent detail, verify binary detection under Overview. **Detected** means only that the executable/adapter is present, **not** that protocol, login, model, network or tool permissions are verified.
3. Open the Agent **Tools** tab to inspect the four canonical Tools and their stable identity. If the Agent predates Phase 34 or a prior tool setup failed, use **Prepare Agent tools** to explicitly adopt/repair the binding. A conflict directs you to Advanced internal Provider management; reads alone never repair data.
4. Open **Profiles** and bind only the Agent tools the Profile needs. Separately, open the Agent's **Access** tab and grant that ProfileID. Both ToolBinding and AgentGrant are required. Existing Policy must allow `tool.invoke` and `agent.activate` / `agent.task.submit`; a second `agent.task.cancel` decision governs cancellation.
5. Connect the upstream MCP client to an already-configured Mender Profile through the supported local or tunneled MCP transport. Submit `delegate` with bounded `prompt` and optional `model` / registered `workspace` alias, receive `taskId`, then call `status` / `result` (or `cancel`) using that opaque ID.

This flow is validated with a deterministic fake Agent through an **actual MCP client session**, not only through direct invocation; cross-Profile access and revoked grants are negative-tested.

## CLI management

The Go CLI shares the same domain and database:

```bash
mender --output json agent list
mender agent add --name "Code Reviewer" --backend codex --root /absolute/project/path --isolation restricted_host
mender --output json agent show agent_0123456789abcdef0123456789abcdef
mender --output json agent tools agent_0123456789abcdef0123456789abcdef
mender agent tools agent_0123456789abcdef0123456789abcdef --prepare
mender agent grant agent_0123456789abcdef0123456789abcdef prof_0123456789abcdef0123456789abcdef
mender agent grants agent_0123456789abcdef0123456789abcdef
mender agent tasks agent_0123456789abcdef0123456789abcdef
mender agent task atask_0123456789abcdef0123456789abcdef
mender agent admissions agent_0123456789abcdef0123456789abcdef
mender agent cancel atask_0123456789abcdef0123456789abcdef
mender agent revoke agent_0123456789abcdef0123456789abcdef prof_0123456789abcdef0123456789abcdef
mender agent remove agent_0123456789abcdef0123456789abcdef --confirm
```

The `--output json` flag is a root persistent flag; scripts can pass it before `agent` (for example `mender --output json agent list`). Identifiers above are illustrative and must be replaced with real IDs. Remove is blocked while Task history, Profile/Policy ToolBindings or conflicting/legacy Provider references still need resolution. For a managed, unused tool facade, explicit Agent removal retires it after reference checks; deleting the same Provider through generic Provider CRUD is blocked.

## Execution and security boundaries

- Codex needs the **separately installed `codex-acp` binary** as well as `codex`; Mender does not download it or run npm/npx during discovery. Grok uses native `grok agent stdio`; OpenCode uses native `opencode acp` over ACP v1 stdio. OpenCode's `model`, `effort` (category `thought_level`) and `mode` are session options. Mender exposes model and effort from actual ACP capabilities, while OpenCode itself owns its default mode.
- The default `restricted_host` process isolation is deliberately **not** a Docker sandbox or VM. It constrains filesystem access and may prevent existing CLI authentication credentials from being read. On the currently tested Mac, OpenCode's owner-initiated no-prompt model inspection failed in `restricted_host`; explicitly selecting `host` on a newly created, temporary OpenCode Agent succeeded. There is **no automatic isolation downgrade**. Only choose Host after accepting its local user filesystem/credential authority. Do not silently retry in unisolated `host` mode. `host` must be explicitly selected by the owner, and uses the Runtime account's local authority.
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


## Phase 40 — Reusable model and local workspace selections

An Agent's **identity is stable** across model and workspace changes. New Agents may specify an optional default ACP model; existing Agents can change it through **Agents → Overview → Edit runtime defaults** without deleting the Agent, modifying its tool identities or changing Profile bindings. The selected model must be advertised by that specific ACP v1 session and confirmed through `session/set_config_option` before any prompt is sent. An empty model uses the Backend's default. Model discovery on catalogue reads and CLI flag interpolation are not supported. In **Agent → Overview → Edit runtime defaults**, an owner action starts a bounded no-prompt ACP session to inspect the currently authenticated Backend's model selector. The dialog lists live model and optional reasoning-effort choices and Backend defaults; the owner may save explicit defaults atomically without changing AgentID or ToolIDs. The equivalent CLI command is `mender agent models <agent-id>` and the owner-only management action is `POST /agents/{agentId}/models/inspect`. Neither inspection nor opening the Agent inventory creates a Task or invokes paid inference.

In **Agents → Workspaces**, the owner may add existing **local directories directly**, using an alias such as `gar` and a canonical absolute directory (choose it through the native directory picker or paste a path). No Sandbox needs to be installed or linked to the Agent; Sandboxes independently govern read/search/edit/shell Workspace Tools. Uppercase alias input is normalized to lowercase (`GAR` → `gar`) and invalid keys/paths show an explanation. Each additional directory still needs an explicit **Agent workspace grant** to the Profile, in addition to the existing AgentGrant, ToolBinding and Policy. Agent task arguments may name only the registered alias, never arbitrary paths or SandboxIDs. Existing `workspaceRoot` remains the default.

```sh
mender agent add --name "Code Reviewer" --backend codex --root /absolute/project/path --isolation restricted_host --default-model <acp-advertised-model>
mender agent model <agent-id> <acp-advertised-model>
mender agent effort <agent-id> <acp-advertised-reasoning-effort>
mender agent workspace bind <agent-id> gar /absolute/local/project
mender agent workspace grant <agent-id> gar <profile-id>
mender agent workspace list <agent-id>
mender agent workspace revoke <agent-id> gar <profile-id>
mender agent workspace unbind <agent-id> gar
```

The existing `delegate` Tool keeps its ToolID, SourceKey and external identity. It now accepts three optional fields:

```json
{
  "prompt": "Review this project",
  "model": "a-supported-acp-model-id",
  "reasoningEffort": "high",
  "workspace": "gar"
}
```

Omitting those fields preserves default selection. A nonempty `reasoningEffort` must appear in the current ACP `thought_level` / `reasoning_effort` options; Mender applies a selected model first, validates its updated effort choices, then confirms the selected effort with `session/set_config_option` before submitting the prompt. Unsupported combinations fail closed. The Backend may advertise different levels for different models, so the UI shows current capabilities and explicitly warns that execution rechecks changed options. After updating an existing Mender installation, open **Agent → Tools → Refresh Agent tools** once: this explicitly republishes the new optional input Schema while retaining the original ProviderID, ToolIDs, Profile ToolBindings and AgentGrant. No automatic permission is added. The admitted Task persists the effective model, reasoning effort and exact local working directory snapshot, which cannot be retargeted by later default edits. Each Task still creates a **new ACP session**; persistent Agent sessions and remote Agent execution remain out of scope.

The `restricted_host` process boundary is not a VM or general Sandbox lease: registering an owner-approved Agent cwd does not grant the Agent stronger isolation. `host` mode must remain an explicit owner choice. Real Codex/Grok login, subscription/model entitlements, API billing, host filesystem policy and live compatibility are not proven by fixture tests or a detected CLI binary.

For multi-target **Workspace Tools** (a separate resource/tool capability), see [Workspace Tool targets](workspace-tool-targets.md). Agent workspace grants and Sandbox Workspace Tool grants remain different authority checks.

## OpenCode local ACP backend (Phase 41)

OpenCode is a third, *built-in* Agent backend (`io.mender.agent.opencode`) rather than an MCP Provider or a copy of a Grok/Codex Agent. It uses the installed OpenCode CLI at a host-owned trusted executable path and launches the fixed `opencode acp` command over ACP v1 stdio. Its native `model` and `effort` (ACP category `thought_level`) selectors are discovered from the current authenticated CLI session, and still use Mender's same model/effort/workspace defaults and per-task overrides. OpenCode also reports a `mode` option, which remains OpenCode-owned in this phase.

From **Plugin Marketplace → OpenCode Agent → Add agent**, choose a name, local workspace and process isolation. The market entry pins the exact backend across configure/review/submit and Back returns to the marketplace; an unknown Agent plugin ID is rejected rather than interpreted as Codex. From the generic **Agents → Add agent** flow, an explicit Backend selection is required before Continue. Backend names in inventory and Agent overview always reflect the canonical stored backend, so OpenCode is never labeled as Grok. The backend is detected passively; adding it does **not** start inference or grant Profiles. The default isolation is `restricted_host`. On the tested Mac, explicit no-prompt OpenCode capability inspection in this mode was unavailable, while inspection with the owner's explicit `host` choice succeeded. The UI warns that Host grants the local CLI access to the Runtime user's files and credentials. **Mender never changes this choice automatically.** OpenCode may require prior login in the terminal, per its own documentation, but Mender does not initiate one.

```sh
mender agent add --name "OpenCode Local" --backend opencode --root /absolute/approved/project --isolation host
mender agent models <agent-id>  # explicit no-prompt ACP option discovery
```

The owner can adjust its default model and thought level under **Agent → Overview → Edit runtime defaults**, then grant Profiles under **Access**. Task calls use the existing `agent.delegate` arguments, including `model`, `reasoningEffort` and registered `workspace`. Agent execution still requires Profile ToolBinding, AgentGrant and Policy; no `--auto`, arbitrary CLI flags, agent-side client filesystem permission grants, background OpenCode API service, SSH Agent process or cross-workspace escalation are introduced. A selected model is applied before the effort, so only its currently advertised effort IDs can be chosen.

This has been accepted through no-prompt local ACP protocol tests and deterministic delegated-task fixtures. Live OpenCode inference/entitlement or paid model success **has not been tested**.

[ADR-0048](../architecture/adr/0048-opencode-native-acp-agent-backend.md) · [Phase 41](../planning/phase-41-opencode-native-agent-backend.md)

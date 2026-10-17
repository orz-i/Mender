# Phase 45 — Governed Agent tool execution and bypass fence

Status: A/B/C completed; D synthetic macOS restricted-process bypass/controlled-edit acceptance passed; live real-Agent governed-mode rollout still blocked (2026-10-17).

## Goal

Give **any supported subordinate Agent** a genuine, Task-scoped way to invoke **Mender's existing governed Workspace Tools**, while keeping native Agent executors outside the trusted Tool boundary and retaining each external Agent's own Agent Loop. This is deliberately narrower than "give OpenCode arbitrary host filesystem/shell permissions".

## Work and commits

| Checkpoint | Deliverable | Minimum acceptance |
|---|---|---|
| A | ADR-0051 + threat matrix / acceptance plan | Docs-first commit |
| B | Runtime-issued ephemeral Task tool capability; exact Agent/Task/Principal/Root/Profile/ToolID validation for every invocation; reuses existing InvokeTool, Policy/Sandbox Router/Audit | Real `workspace.read` and disposable `workspace.edit` through canonical invoke pipeline; negative cross-Task/ToolID/revocation/budget tests |
| C | Backend-neutral ToolSupply contract + negotiated delivery adapters (ACP v1 HTTP first), explicit strong placement preflight | Supports only a negotiated protocol/transport; fail-closed when unavailable, no self-delegation, no host fallback, cancellation closes capabilities |
| D | Restricted/isolated OpenCode or synthetic ACP native bypass tests, read/write/shell verification, full check + operator acceptance | Native write cannot bypass gateway. If blocked by CLI credentials/isolation, do not claim completion |

## Constraints and risk

- `host` remains a cooperative ACP approval mode. A native Host agent may still read/write/execute without Mender and therefore **must not** be mislabeled a governed tool user.
- Task-root registration is not Sandbox tool grant. Strong mode can optionally require an explicitly bound ready local restricted Sandbox; do not make every Agent workspace depend on one.
- Only owner-allowed pinned Profile Workspace ToolIDs become available; no tool source duplication or new Agent facade tool identities.
- Security identity is cryptographically strong per-active Task capability, never a model-supplied string/TaskID. Re-check Profile ToolBinding, AgentGrant, workspace grant, Policy, Sandbox ready root and effective isolation at each actual call; otherwise deny.
- Never expose approval decisions or Agent Delegation Tools to the subordinate Agent, never auto-approve shell, never read/inject credential files.
- No edits to the live user's Mender repo or DB. Real positive write acceptance only with an owner-approved disposable test file.
- Preserve `.agents/`, `.pi/`, `.intent-loop/`, `.commandcode/` and avoid staging them.

## Exit classification

- **A/B proven**: canonical Mender tool routing works, no asserted native bypass protection.
- **A/B/C proven**: ACP Agent can request actual Mender Tools with Task-scoped authority, but strong claims still require D process confinement tests.
- **D proven**: native Agent execution demonstrably cannot bypass protected roots and sensitive Mender Tool calls pass current grants/policy/isolation checks.

The work must not expand into an unbounded series of phases, compatibility adapters or a second full Agent Loop.

## Cross-Agent capability matrix (runtime detection, not user-visible Provider layers)

| Backend | Agent loop | ACP v1 | HTTP MCP injection | Other tool delivery | Strong isolation |
|---|---|---|---|---|---|
| OpenCode | External | Confirmed in local v2.0.30 | Observed in real `initialize` | Must be independently reviewed | Not proven for host process |
| Grok | External | Confirmed in local no-prompt probe | **Observed true**, still negotiate every session | SSE also observed | Native Host not confined |
| Codex | External | Local Codex CLI present but `codex-acp` not installed | **Unverified**, fail closed | Unverified | Not proven |
| Future Agent | External | Optional | Optional | Backend-specific port, no bridge guessing | Explicitly verified |

The **authorization core** knows canonical ToolIDs, Task generation, Principal, Profile grants, Sandbox lease and Policy only; capability transport is an adapter concern. User UI should display the Agent's practical capabilities (e.g. can request governed Tools), not ACP/Provider jargon.

## Implementation and acceptance — 2026-10-17

**Reached:** A, B and C implemented and accepted in isolated Runtime tests; D's read-only-root native bypass checks passed for the local host, but the **real-Agent restricted execution gate is still incomplete**. No live Agent or Profile has been silently switched to governed mode.

### Committed checkpoints

- `e0b4a91` — initial ADR-0051/Phase 45 contract (documents first).
- `e2e9e9c` — hard-cut the assumption of an OpenCode-only API into a **backend-neutral ToolSupply** contract. An Agent adapter, not the governance kernel, negotiates actual MCP/ACP/other tool delivery and refuses unsupported modes.
- `4496529` — task-bound 32-byte random bearer minted only inside Runtime memory, bound to one local **running Task**, exact Task generation/AgentID/original Profile Principal **and original Origin**, canonical cwd, explicit ready `restricted_host` local Workspace Sandbox, exact owner, and a trusted allowlist of existing Workspace ToolIDs. Each actual Tool call rechecks Task liveness, AgentGrant, Profile ToolBinding, workspace binding and grant, Sandbox identity/readiness/pinned Provider source identity. It then calls the **existing canonical Mender InvokeTool**, which enforces Policy, sandbox execution lease, Runtime Budget and Audit. No direct Sandbox backend call or new Tool/Provider inventory.
- `4496529` — ephemeral loopback HTTP MCP gateway accepts only the correct per-server bearer/Host/Origin, bounds each request, publishes only the selected canonical `workspace.read/search/edit` tools and forbids `workspace.shell`, Agent delegation or owner approval. It automatically closes/invalidates on Task cancellation/completion, capability expiry and Runtime shutdown. Tokens never enter durable Task/Agent databases or Profile MCP.
- `4496529` — ACP v1 adapter transmits the optional Task ToolSupply only after the backend's real `initialize.agentCapabilities.mcpCapabilities.http` is true. If unavailable it fails closed. When ToolSupply is present, command construction requires `restricted_host` and a verified read-only project root, with no Host fallback. Normal Agent tasks keep `ToolSupply` empty and retain Phase 44 cooperative approval behavior.
- `28e1148` — negative process-isolation tests for **Grok, OpenCode and Codex** Host modes and missing restricted launcher, plus macOS Seatbelt policy construction of root-read-only project placement for Grok/OpenCode. The existing real macOS Seatbelt integration tests executed `/bin/sh` in ephemeral protected directories and confirmed writes inside that root were denied.

### Deterministic real-chain fixtures (no user source writes)

- In a newly created SQLite database and disposable directory, **an Agent with Grok backend identity** was granted a synthetic Profile and exact, already existing Workspace ToolIDs from a deliberately created ready `restricted_host` local Sandbox. The gateway invoked the **real existing Mender read/edit Tool pipeline**. Read returned the test file; edit created one disposable file. ToolIDs and ProviderIDs did not rotate; no unrelated grants were issued.
- A synthetic external ACP Agent completed ACP v1 `initialize → session/new(mcpServers=[task-scoped HTTP]) → MCP tools/list → tools/call(workspace.read) → session/prompt end_turn` through the Task HTTP bridge and canonical runtime, with no Agent-specific Tool wrappers.
- Negative tests passed: wrong token, token from another server, unsupported ToolID, shell/recursive Agent Tool, Profile ToolBinding removal, AgentGrant revocation, canceled Task, missing Agent subprocess capability, unsupported HTTP MCP, Host isolation, malformed destination, excessive/invalid bearer, Task/server shutdown and replay. A canceled Task loses authority synchronously and the MCP listener is closed shortly thereafter.
- Local installed ACP CLI `initialize` probes **without prompts or billing** reported:
  - **Grok:** ACP v1, HTTP MCP supported, SSE MCP supported.
  - **OpenCode v2.0.30:** ACP v1, HTTP MCP supported, SSE not advertised.
  - **Codex:** local Codex CLI present; required `codex-acp` adapter not present in trusted locations, so its tool-supply capability remains **unverified** and must not be inferred or silently enabled.
- Tests never wrote any existing source file or changed the live Mender user store. A read-only check of live schema-v10 inventory preserved 3 Agent resources, 3 pinned Agent facades, 25 Profile ToolBindings, 2 AgentGrants and one extra workspace grant. No active Tasks or pending ACP approvals, `PRAGMA quick_check=ok`, zero FK violations.

### Hard remaining limits and intentional non-deployment

1. A normal production Agent Task currently uses an **empty** `RunConfig.ToolSupply`. The runtime bridge is internal-only until the owner chooses an explicit governed placement and exact grant-scoped ToolIDs; it does **not** silently bind or create a Sandbox for an existing local Agent workspace.
2. **Host ACP tools remain cooperative**, and even macOS restricted_host allows permitted network access and some temporary writes. A protected-root native write denial alone does not establish universal read/network/credential confinement or stop a subagent/plugin from abusing permitted paths. Thus **do not label native read/edit/shell as universally Mender-governed** without further bypass testing of the actual Agent's chosen placement and resolved config.
3. The first live restricted-host OpenCode attempt in prior acceptance could not inspect model options because OpenCode's CLI environment/auth was blocked. Real Grok/OpenCode/Codex tool-injection acceptance under restricted placement therefore remains gated. Do not copy credentials or auto-fallback to Host to make a test pass.
4. This phase has **no additional native UI permission toggle**; backend-neutral capability delivery is selected internally and must be surfaced as user-comprehensible capabilities only after exact placement/grant acceptance, not as Provider or ACP jargon. The four Profile-facing Agent Delegation ToolIDs remain untouched.
5. In-flight Tool operations use the existing Sandbox/Invocation cancellation and unknown-outcome semantics. A revocation racing with an already admitted irreversible edit cannot be treated as guaranteed rollback; no Agent tool retries after unknown state.

**Classification:** Backend-neutral Mender ToolSupply and ACP HTTP delivery are implemented and verified in disposable tests (**A/B/C**); mandatory broad native confinement and production real-Agent deployment (**full D**) are **not yet accepted**. Do not treat this bridge as a complete Agent Harness.

### D checkpoint — macOS confined synthetic ACP Agent, both paths actually exercised

- `166c4bb`: a **generic** ACP fixture (using the synthetic **Grok** Agent backend identity) was launched as a real macOS Seatbelt `restricted_host` **subprocess** with the admitted temporary root **read-only**. In that process, `os.WriteFile("native-bypass.txt",...)` failed as required and did not create any file.
- The *same* confined process used the task-bound loopback MCP connection to call the preexisting Mender `workspace.edit`, creating **`controlled-edit.txt`** in its disposable workspace through the trusted Profile/Invocation/Sandbox lease execution chain. It then called Mender `workspace.read` and completed the ACP prompt with a bounded result. The parent asserted the direct bypass file did **not** exist and the edited file did contain `through Mender only`.
- Crucially, the native denial was enforced **by the process isolation launcher**, and the allowed edit was enforced **by Mender's existing canonical ToolID and current grants**. The test didn't rely on an OpenCode-specific tool, use a model service, modify an existing project, autoapprove Shell or read any credentials.
- The initial combined fixture assumed a fixed MCP tools/list order; MCP correctly returned tools in another order. The fixture was corrected to verify the **set of canonical names**, not their listing order. Final confined test passed and targeted **Go Race Detector** passed, including cancellation, token revocation and cross-listener isolation.
- The runtime boundary is **narrower than universal filesystem/network enforcement**. Seatbelt's current profile permits some temporary-root writes and network traffic, and it is not a VM. The authenticated Grok/OpenCode/Codex CLI process running in this exact restricted placement, plugin/config effects, mandatory native-tool routing and token delivery from an owner-selected live placement remain unverified. **Production RunConfig.ToolSupply is still empty** until that explicit owner configuration and true-Agent acceptance are complete.

### Final quality/identity receipt

- The **final** `pnpm check` (after the confined synthetic ACP integration test was added) returned **exit code 0**. Private local receipt: `/tmp/mender-phase45-final-check.exit`; complete log: `/tmp/mender-phase45-final-check.log`. It includes complete Go unit/integration tests and **full backend Race Detector**, Go lint, SDK/plugin examples, generated OpenAPI/Wails parity, desktop architecture, strict TypeScript, ESLint, zh-CN **1263/1263**, Vitest **44 files / 253 tests**, and Web/Desktop production builds. Vite large-chunk warnings are nonblocking.
- The native Wails Go desktop binary separately compiled successfully from the Phase 45 product code, with only nonfatal macOS deployment-target linker warnings; its temporary compiled copy was removed.
- The actual production Application Support SQLite was inspected **read-only** and remains schema v10: Codex/Grok/OpenCode (three Agents), three pinned Agent tool facades, 25 Profile ToolBindings, two AgentGrants and one additional Agent workspace grant. `quick_check=ok`, zero foreign-key violations, **zero active Agent Tasks** and **zero pending ACP approvals**. No historical tasks, ToolIDs, user Sandbox configuration, Agent defaults or grants were modified.
- The running Wails native process and current ChatGPT OpenAI MCP Tunnel were intentionally **not restarted** or modified during this phase. There is no new Profile-facing Agent Tool, no owner-facing approval UI change, and no implicit tooling added to an existing Host Agent.
- The proven scope is intentionally **backend-neutral governed Tool supply inside test-controlled Task sessions**. A user-visible owner selection of exact governed placement/ToolIDs and an authenticated *real Grok/OpenCode/Codex* restricted execution acceptance are outstanding. Ordinary production Agent Tasks still have empty `RunConfig.ToolSupply`. Neither Test Suite success nor macOS protected-root denial should be presented as completed mandatory isolation of every native Agent action.

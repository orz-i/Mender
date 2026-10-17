# Phase 45 — Governed Agent tool execution and bypass fence

Status: A accepted, B in progress (2026-10-17).

## Goal

Give a subordinate ACP Agent a genuine, Task-scoped way to invoke **Mender's existing governed Workspace Tools**, while keeping native Agent executors outside the trusted Tool boundary and retaining OpenCode's Agent Loop. This is deliberately narrower than "give OpenCode arbitrary host filesystem/shell permissions".

## Work and commits

| Checkpoint | Deliverable | Minimum acceptance |
|---|---|---|
| A | ADR-0051 + threat matrix / acceptance plan | Docs-first commit |
| B | Runtime-issued ephemeral Task tool capability; exact Agent/Task/Principal/Root/Profile/ToolID validation for every invocation; reuses existing InvokeTool, Policy/Sandbox Router/Audit | Real `workspace.read` and disposable `workspace.edit` through canonical invoke pipeline; negative cross-Task/ToolID/revocation/budget tests |
| C | ACP v1 task-bound MCP stdio service and transport + explicitly reviewed strong placement preflight | Can list/invoke only permitted ToolIDs, no self-delegation, no host fallback, cancellation closes capabilities |
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

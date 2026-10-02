# Phase 22 Plan — Provider Activation Governance & Principal

Status: Complete

## Objective

Make Provider capability acquisition a first-class governed action and replace descriptive Caller attribution with explicit Principal identity across Runtime Policy, Invocation, and Audit.

Architecture is governed by [ADR-0032](../architecture/adr/0032-provider-activation-principal-governance.md).

## Product boundary

Phase 22 strengthens Mender's Tool/Resource runtime boundary. It does not add model calls, prompt construction, planning, retries, memory, or an Agent loop.

The target path is:

~~~text
trusted adapter
    |
    v
GovernanceContext
  Principal + Origin + Scope
    |
    +------------------------------+
    |                              |
    v                              v
provider.activate               tool.invoke
    |                              |
    v                              v
Policy + Approval              Policy + Approval
    |                              |
    v                              v
Provider Manager              Binding constraints
Open/Discover/Route                |
                                   v
                              Execution Guards
                                   |
                                   v
                                Executor
~~~

Both paths produce data-minimized durable Audit evidence.

## Hard-cut rules

Phase 22 must not introduce:

- deprecated Caller aliases;
- old/new JSON fields in parallel;
- missing-action Policy fallback parsing;
- an ungated alternate EnsureConfiguredProvider runtime path;
- a synthetic ToolID for Provider activation;
- fake end-user identity for downstream MCP.

One-way SQLite migrations are allowed. After migration, runtime code reads only the new schemas.

## Contract claims

### SC-1 — Principal is mandatory

Every live GovernanceContext contains a valid Principal.

Initial live Principal kinds:

- local_user;
- service.

CLI/Desktop/Management operations use the Runtime owner's local-user Principal. Downstream Profile MCP uses a Profile-scoped service Principal. Internal operations use an explicit Mender service Principal.

### SC-2 — Caller is removed

Runtime domain/application contracts, Audit projections, OpenAPI, TypeScript SDK, Wails bindings, Web/Desktop views, and CLI JSON contain Principal fields instead of Caller.

Historical Audit caller rows are one-way migrated into audit-only legacy Principal snapshots.

### SC-3 — Policy is action-aware

Policy supports tool.invoke and provider.activate. Every persisted Rule has at least one explicit action.

Policy Match supports Actions, Origins, PrincipalKinds, PrincipalRefs, ScopeKinds, ProfileRefs, ProviderIDs, ToolIDs, FactKnowledge, EffectsAny, and ResourcesAny.

Action validation prevents Tool/fact predicates from leaking into provider.activate.

### SC-4 — Activation is admitted before side effects

Every production path that can cause Provider activation evaluates provider.activate Policy and Approval before Plugin.Open, external process start, discovery Credential resolution, upstream connect/list/discovery, or executor route registration.

### SC-5 — No activation bypass

providerplugin.Manager.Ensure remains a low-level mechanism only. Runtime/adapters call one governed Provider activation application boundary. No public Runtime compatibility alias exposes ungated activation.

### SC-6 — Activation Audit exists

Provider activation writes a dedicated minimal audit record with Principal, action, Provider, scope/origin, Policy, approval, status/timestamps, and bounded failure classification.

### SC-7 — Full adapter parity

CLI, management HTTP, managed MCP lifecycle, Desktop binding, OpenAPI/generated TypeScript SDK, and shared React app compile and test against the new Principal/action contracts.

## Implementation clusters

### Cluster A — Documentation contract

Write ADR-0032, this plan, architecture index/context/principles, and roadmap entry.

### Cluster B — Shared governance identity

Introduce Principal, PrincipalKind, Origin, Scope, GovernanceContext, and Action in a governance-domain vocabulary. Hard-cut Invocation to consume it and remove Caller-owned fields/types. Add stable constructors for local Runtime owner, Profile MCP service, and internal Mender service Principals.

### Cluster C — Action-aware Policy

Refactor Policy evaluation and Match around Governance Action and Principal. Add invariants for tool.invoke and provider.activate. Advance Policy SQLite and rewrite old Rule match JSON to actions=["tool.invoke"].

### Cluster D — Principal Audit migration

Replace Invocation Audit Caller with Principal snapshot. Advance Audit SQLite and migrate historical Caller values to audit-only legacy Principals. Update projections and tests.

### Cluster E — Governed Provider activation

Add one admission boundary:

~~~text
Admit Provider Activation
  -> Policy
  -> Approval
  -> Manager.Ensure
  -> Activation Audit
~~~

Wire all production activation call sites through it and remove the previous public ungated Runtime method. Spy tests prove deny/approval-required paths do not call Open or resolve Credentials.

### Cluster F — Contracts and management UX

Hard-cut OpenAPI, generated TypeScript SDK, Wails types/bindings, Desktop client, Policy React form/detail summaries, Audit/Overview Principal columns, CLI Policy flags/JSON, tests, and i18n.

Policy UI adds structured Action and Principal Kind selectors plus an explicitly advanced Principal Reference input.

### Cluster G — Integration verification

Run focused tests continuously, then:

- pnpm provider-plugin:proto:check
- pnpm sdk:check
- pnpm desktop:bindings:check
- pnpm go:lint
- pnpm go:test
- pnpm go:test:race
- pnpm lint
- pnpm typecheck
- pnpm test
- pnpm build
- pnpm security:check
- final pnpm check

## Persistence migrations

### Policy

Advance Policy schema exactly once. For every pre-Phase-22 Rule, migration-local code parses the previous valid Match JSON, writes the new shape with actions=["tool.invoke"], and preserves existing selectors/decision/identity/timestamps. The normal runtime decoder knows only the new format.

### Invocation Audit

Advance Audit schema exactly once.

For every historical row:

- principal_kind = legacy
- principal_ref = bounded previous caller when present
- empty caller becomes deterministic fallback historical-unknown

Runtime never dual-writes caller.

### Activation Audit

Create a dedicated Provider activation audit table owned by Audit infrastructure with its own stable activation identity and bounded status/failure fields.

## Adapter Principal sources

### CLI

Trusted CLI composition derives Principal from the current OS user identity.

### Desktop

Desktop host/composition derives the same Runtime-owner identity.

### Management HTTP

Loopback management executes under the Runtime owner's local-user Principal. This is an authority assertion, not peer-process authentication.

### Downstream MCP

Each Profile MCP adapter uses a service Principal naming that Profile endpoint. A tunnel does not change this to an end-user Principal because Mender does not currently receive authenticated end-user identity.

## Verification scenarios

Blocking scenarios:

1. invalid/zero Principal rejected before invocation work;
2. Policy Rule cannot omit Actions;
3. migrated old Rule matches tool.invoke but not provider.activate;
4. principal kind/ref selectors match deterministically;
5. provider.activate rejects Tool/fact predicates;
6. activation deny does not call Plugin.Open;
7. activation require-approval does not call Plugin.Open;
8. activation allow opens exactly once and registers executor;
9. managed MCP startup uses control-operation Principal for activation;
10. downstream MCP Tool invocation uses Profile service Principal;
11. Invocation Audit contains Principal and no Caller;
12. historical Audit migration produces legacy Principal snapshots;
13. activation Audit contains no raw config/credential values;
14. Web/Desktop Policy editing round-trips action/principal selectors;
15. generated SDK/Wails artifacts contain no Caller fields.

## Non-goals

- interactive approval;
- OAuth/OIDC end-user authentication;
- Principal groups/roles/inventory;
- kernel/container/VM sandboxing;
- rate limits, quotas, CPU/memory budgets, circuit breakers;
- generic policy expressions;
- changing Workspace Tool contracts.

## Commit strategy

1. docs(governance): define principal activation admission
2. refactor(governance): hard cut to principal action policy
3. feat(governance): admit provider activation before open
4. feat(app): expose principal activation governance
5. docs(governance): complete phase 22

## Completion

Phase 22 is complete.

Implemented:

- added a shared GovernanceContext with mandatory `local_user` / `service` Principal identity, Origin, and Catalog/Profile scope; Invocation no longer owns or exposes Caller;
- hard-cut Policy to explicit `tool.invoke` / `provider.activate` actions plus Principal kind/reference selectors, with action-specific invariants and no missing-action runtime fallback;
- one-way migrated existing Policy Rules to `actions=["tool.invoke"]` and historical Invocation Audit caller values to audit-only `legacy` Principal snapshots;
- introduced one Provider activation admission path that evaluates Policy and the shared ApprovalGate before Provider side effects, including Provider synchronization, rebinding, Profile activation, managed MCP startup, and Tool-call lazy activation;
- removed public ungated `EnsureConfiguredProvider` / MCP compatibility activation entry points; Provider Manager `Ensure` remains an internal low-level mechanism behind admission;
- changed external Provider resolution to use the persisted verified descriptor snapshot without starting the external plugin process before admission; runtime descriptor verification still occurs once execution is admitted;
- added dedicated data-minimized Provider activation Audit with a query surface separate from Invocation Audit;
- hard-cut CLI, management HTTP, OpenAPI, generated TypeScript SDK, Wails bindings, Desktop client, Web/Desktop Policy UX, Overview, and Audit UI to Principal/action contracts with no Caller compatibility fields;
- Policy management exposes structured action/Principal-kind selectors and an explicitly advanced Principal-reference input while no Principal directory exists;
- downstream MCP Tool calls use Profile service Principals; local CLI/Desktop/Management control operations use the Runtime owner's OS-backed local-user Principal without claiming peer-process or remote end-user authentication.

### Verification evidence

Repository gates completed successfully on 2026-10-02:

- `pnpm provider-plugin:proto:check`;
- `pnpm sdk:check`;
- `pnpm desktop:bindings:check`;
- full `pnpm check`, including Go/module isolation, Go lint/tests/race, generated-contract drift checks, frontend lint/typecheck/tests/builds, and Provider Plugin SDK/example checks;
- frontend tests: 69/69;
- `pnpm security:check`;
- govulncheck reported no reachable vulnerabilities for backend, Desktop, Provider Plugin SDK, or example plugin;
- production JavaScript dependency audit reported no known vulnerabilities at the configured high severity threshold;
- static Caller scan shows no Caller-era field in active backend/public contracts; only historical SQLite migration text and ordinary English uses of “caller” remain.

### Commit sequence

- `0a8feb1` — `docs(governance): define principal activation admission`;
- `13baaf2` — `refactor(governance): hard cut to principal action policy`;
- `db436ef` — `feat(governance): admit provider activation before open`;
- `e62d08a` — `fix(governance): clean phase 22 lint`;
- `e065493` — `feat(app): expose principal activation governance`.

Deferred items remain exactly the Phase 22 non-goals: interactive approval, remote end-user OAuth/OIDC, Principal directory/groups/roles, OS/container/VM isolation, and runtime quotas/resource budgets.

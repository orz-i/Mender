# Phase 23 Plan — Runtime Budget & Restricted-host Isolation

Status: In Progress

## Objective

Add hard Runtime execution budgets and an explicit local-process isolation contract after Principal/Policy admission, while keeping Mender a Tool/Resource runtime rather than an Agent scheduler.

Architecture is governed by ADR-0033.

## Contract claims

### SC-1 — Runtime Budget is mandatory

Every Tool invocation loads one RuntimeBudget snapshot before execution.

The snapshot hard-caps argument bytes, effective execution timeout, result bytes, and concurrency.

### SC-2 — Hierarchical invocation capacity

A Tool invocation must atomically acquire global, Principal, Provider, and Tool capacity.

Acquisition is fail-fast. Partial acquisition cannot leak counters.

### SC-3 — Activation capacity

provider.activate acquires a dedicated activation slot only after Policy/Approval admit the action and before the activation operation begins.

Budget rejection proves Provider activation side effects did not begin for that attempt.

### SC-4 — Profile only tightens

Existing Profile ToolBinding serial execution remains a second local constraint. Runtime limits cannot be raised by Profile state.

### SC-5 — Restricted-host is explicit

Local subprocess execution uses host or restricted_host. restricted_host never silently falls back to host.

### SC-6 — workspace.shell uses the isolation launcher

workspace.shell cannot call os/exec directly. The built-in Workspace plugin receives a host-owned launcher and canonical root.

### SC-7 — external Provider launch uses the same contract

External plugin transport accepts a host-supplied launcher while independently verifying the exact plugin executable checksum.

Plugin config/protocol cannot weaken isolation.

### SC-8 — Management parity

Management HTTP, OpenAPI/SDK, Wails/Desktop, shared Web/Desktop UX expose Runtime Budget, Runtime Isolation, backend availability, and bounded live usage.

## Implementation clusters

### Cluster A — Documentation

Write ADR-0033, this plan, architecture index/context/principles, and roadmap entry. Commit independently before code.

### Cluster B — Runtime Budget domain and persistence

Add Runtime governance domain/application/SQLite singleton settings for RuntimeBudget and RuntimeIsolation.

### Cluster C — Invocation budget enforcement

Replace default Unlimited execution with a hierarchical fail-fast controller.

Invocation loads one snapshot, enforces arguments, clamps timeout, acquires Runtime lease after Policy/Approval, preserves Profile serial constraints, and guards result using snapshot MaxResultBytes.

Budget failure maps to canonical runtime_budget_exceeded rejection and Audit classification.

### Cluster D — Provider activation budget

Add activation lease acquisition to provider.activate immediately before the admitted operation.

### Cluster E — Process isolation abstraction

Add host-owned process launch specification and backend detection.

Backends:

- host;
- macOS sandbox-exec restricted_host;
- Linux bubblewrap restricted_host;
- unsupported restricted_host fail-closed stub.

### Cluster F — workspace.shell isolation

Inject launcher through built-in Provider composition. Tool arguments cannot select mode or add roots.

### Cluster G — external Provider isolation

Extend Provider Plugin transport launch boundary so the host can wrap the verified executable while preserving checksum verification over the plugin executable itself.

### Cluster H — Contracts / management UX

Expose GET/PUT Runtime Budget, GET/PUT Runtime Isolation, backend availability/name, and bounded live usage.

Update OpenAPI/generated SDK/Wails/Web/Desktop/i18n/tests.

### Cluster I — Verification

Run focused tests/race throughout, then provider-plugin proto/SDK checks, SDK/Wails drift checks, Go lint/test/race, frontend lint/typecheck/tests/build, security check, and final pnpm check.

## Defaults

Runtime Budget:

- global invocations: 32;
- per Principal: 8;
- per Provider: 8;
- per Tool: 4;
- Provider activations: 4;
- max execution timeout: 120 seconds;
- max arguments: 1 MiB;
- max result: 1 MiB.

Runtime Isolation:

- workspace.shell: restricted_host when backend is available at first initialization, otherwise host;
- external Provider: host.

Persisted restricted_host remains fail-closed if support later disappears.

## Persistence migration

Runtime governance gets an independent forward-only schema.

No migration rewrites Policy/Profile. Budget and isolation are Runtime-owned capacity/environment settings.

## Verification scenarios

1. invalid budget values are rejected;
2. oversize arguments never reach schema validation/executor;
3. requested timeout above Runtime max is clamped;
4. global/Principal/Provider/Tool saturation returns runtime_budget_exceeded without executor entry;
5. released leases allow subsequent calls;
6. concurrent race tests show no counter underflow/leak;
7. activation saturation prevents activation operation entry;
8. lowered limits do not kill existing leases but reject new excess actions;
9. result larger than snapshot MaxResultBytes is rejected;
10. restricted_host unavailable fails closed;
11. workspace shell cannot read a host-home secret outside workspace on supported restricted_host backend;
12. workspace shell can read/write inside configured workspace;
13. external plugin checksum still verifies the plugin executable when wrapped;
14. external Provider restricted_host gets only declared package root plus bounded runtime paths;
15. host mode retains explicit current behavior;
16. management/SDK/Wails/Web/Desktop settings round-trip;
17. generated artifacts do not drift.

## Non-goals

- per-minute/hour/day rate limiting;
- monetary/token budgets;
- CPU/memory cgroups or Windows Job Object quotas;
- network egress governance;
- VM/container provisioning;
- interactive approval;
- killing already-running work after a settings update.

## Commit strategy

1. docs(governance): define runtime budget isolation
2. feat(runtime): enforce execution budgets
3. feat(runtime): isolate local process execution
4. feat(app): manage runtime budget isolation
5. docs(governance): complete phase 23

## Completion

Phase 23 completes when Runtime capacity is bounded by default, local subprocess capabilities use an explicit isolation mode with no silent fallback, workspace.shell and external Provider launch are integrated, management contracts expose settings/status, and repository/race/security gates are green.

# Phase 23 Plan — Runtime Budget & Restricted-host Isolation

Status: Complete

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
- external Provider: host (Phase 23 initial default; **superseded for fresh settings by ADR-0042 / Phase 36**).

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

Phase 23 is complete.

Implemented:

- introduced a persistent Runtime governance bounded context with validated singleton RuntimeBudget and RuntimeIsolation settings;
- removed the Runtime's default unlimited baseline execution guard and replaced it with race-safe, fail-fast hierarchical leases for global, Principal, Provider, and Tool invocation capacity;
- added a separate Provider activation capacity lease acquired only after Policy/Approval admission and before activation side effects;
- enforced one budget snapshot per Invocation for argument bytes, timeout clamping, concurrency, and canonical result bytes;
- mapped budget saturation/oversize arguments to canonical `runtime_budget_exceeded` rejection plus bounded Invocation/activation Audit classifications;
- retained Profile ToolBinding serial execution only as a second tightening layer after Runtime capacity admission;
- introduced explicit `host` / `restricted_host` process isolation with no silent fallback;
- implemented macOS Seatbelt and Linux bubblewrap restricted-host backends plus unsupported fail-closed behavior;
- verified the macOS backend by requiring workspace read/write success and denial of a Runtime-owner HOME sibling secret before reporting availability;
- routed `workspace.shell` through a host-owned launcher so Tool arguments cannot select isolation or add filesystem roots;
- routed external Provider plugin processes through the same isolation boundary while independently verifying the exact plugin executable checksum before wrapper launch;
- persisted isolation independently from Policy/Profile state; workspace shell defaults to restricted_host only when the backend passes its startup probe, while external Providers default to host;
- added a fail-closed Runtime update boundary so `restricted_host` cannot be persisted when the backend is unavailable;
- exposed Runtime Budget, isolation, backend availability/name, and live invocation/activation usage through management HTTP, OpenAPI/generated SDK, Wails/Desktop, and the shared Web/Desktop Runtime page;
- the Runtime page distinguishes unrestricted `host` authority from `restricted_host` containment and explicitly states that restricted_host is not VM/container isolation and does not govern network egress in this phase.

### Verification evidence

Repository gates completed successfully on 2026-10-03:

- `pnpm sdk:check`;
- `pnpm desktop:bindings:check`;
- `pnpm provider-plugin:proto:check`;
- full `pnpm check`, including Provider Plugin SDK/example lint/tests/race/build, Go module isolation, generated-contract drift checks, desktop architecture checks, Go lint/tests/race, frontend lint/typecheck/tests/build;
- frontend tests: 71/71;
- `pnpm security:check`;
- govulncheck reported no reachable vulnerabilities for backend, Desktop, Provider Plugin SDK, or example plugin;
- production JavaScript dependency audit reported no known vulnerabilities at the configured high severity threshold;
- hard-cut static scan confirmed no RuntimeOptions `MaxResultBytes`, baseline Invocation `ExecutionGuard`, or Runtime `executionguard.Unlimited` compatibility path was reintroduced.

Focused verification additionally covers invalid budget values, pre-schema argument rejection, timeout clamping, all four invocation capacity dimensions, idempotent/race-safe release, lowered-limit behavior, activation saturation before operation entry, snapshot result-size enforcement, unavailable restricted-host rejection, workspace HOME-boundary containment, host mode, external wrapper checksum verification, and management/Desktop/Web round trips.

### Commit sequence

- `deacedc` — `docs(governance): define runtime budget isolation`;
- `fbf012b` — `feat(runtime): enforce execution budgets`;
- `0cd1c32` — `feat(runtime): isolate local process execution`;
- `5e083e5` — `feat(app): manage runtime budget isolation`.

### Platform implementation note

The ADR originally preferred passing Seatbelt roots through `sandbox-exec -D`. Darwin 27 testing demonstrated unreliable path-filter behavior for that mechanism. The shipped macOS backend instead canonicalizes paths, strictly escapes them, generates the SBPL inside trusted host code, and denies HOME sibling paths outside the permitted root. The security ownership model is unchanged: untrusted Tool/Provider data cannot provide sandbox policy fragments.

Deferred work remains the Phase 23 non-goals: rate-per-window limits, monetary/token budgets, CPU/memory/disk quotas, network egress policy, VM/container provisioning, interactive approval, and terminating already-running actions after settings changes.

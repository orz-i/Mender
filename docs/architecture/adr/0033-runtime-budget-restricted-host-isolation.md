# ADR-0033: Bound Runtime resource use and isolate local process execution

- Status: Accepted
- Date: 2026-10-02

## Context

ADR-0032 moved Provider activation and Tool invocation behind explicit Principal-aware governance, but an admitted action can still consume unbounded Runtime execution capacity.

Current defenses are fragmented:

- Invocation has a default/request timeout;
- Result Guard has a byte ceiling;
- Profile ToolBinding may serialize one binding;
- optional Provider/Tool serial guards exist;
- external Provider control-plane calls have individual deadlines.

The default Runtime execution guard still admits unlimited concurrent Tool execution, there is no Principal/Provider/Tool quota, argument size is not a Runtime budget, and Provider activation has no shared concurrency budget.

Local process execution is also not contained by one execution-environment contract. workspace.shell starts a host shell with a workspace-contained initial cwd, but the process can access host paths/network permitted to the OS user. External Provider plugins run as verified local executables with bounded environment/protocol handling, but they still inherit host process authority.

Mender needs a bounded Runtime and an explicit local-process isolation boundary without becoming a container orchestrator or Agent loop.

## Decision

Phase 23 introduces two independent Runtime responsibilities:

~~~text
Governance admission
      |
      v
Runtime Budget
      |
      v
Execution Environment
      |
      v
Executor / Provider activation
~~~

Budget answers how much Runtime capacity an admitted action may consume.

Isolation answers what host resources a local child process can reach.

They are intentionally separate.

## Runtime Budget

Mender owns one persistent RuntimeBudget singleton.

Initial limits are:

- MaxInvocations;
- MaxPrincipalInvocations;
- MaxProviderInvocations;
- MaxToolInvocations;
- MaxProviderActivations;
- MaxExecutionTimeoutMs;
- MaxArgumentBytes;
- MaxResultBytes.

Fresh defaults:

~~~text
MaxInvocations            = 32
MaxPrincipalInvocations   = 8
MaxProviderInvocations    = 8
MaxToolInvocations        = 4
MaxProviderActivations    = 4
MaxExecutionTimeoutMs     = 120000
MaxArgumentBytes          = 1048576
MaxResultBytes            = 1048576
~~~

Runtime Budget is a hard upper bound. Profile ToolBinding constraints may tighten execution further but may never raise Runtime limits.

### Budget acquisition semantics

Invocation snapshots the current RuntimeBudget once before argument normalization and uses that snapshot for the complete attempt.

Order:

~~~text
validate GovernanceContext
allocate InvocationID
load RuntimeBudget snapshot
enforce argument bytes
scope / Tool / schema / Facts / Policy / Approval
clamp effective timeout to Runtime max
acquire Runtime budget lease
acquire Profile binding constraint
execute
guard result using snapshot MaxResultBytes
release leases
audit
~~~

Runtime concurrency acquisition is fail-fast. Mender does not create an unbounded waiting queue when the Runtime is saturated.

Budget saturation produces a canonical rejected Invocation outcome with rejection code runtime_budget_exceeded and a bounded Audit classification. It is not an internal error.

Provider activation evaluates Policy/Approval first, then acquires the activation budget immediately before the admitted operation. Saturation prevents Plugin.Open/process start/discovery and produces a dedicated activation Audit failure classification.

### Dynamic settings

RuntimeBudget is persisted and can be updated through management surfaces.

Updates affect new actions only. Existing leases keep the snapshot under which they were admitted.

Lowering a limit below current usage is valid; no running action is killed, but new matching acquisitions fail until usage falls under the new bound.

## Restricted-host process isolation

Mender defines an explicit local process isolation mode:

- host — execute with the existing OS-user process authority;
- restricted_host — execute through a platform isolation backend with explicit permitted filesystem roots.

There is no auto or best-effort runtime mode. restricted_host fails closed if the selected platform backend is unavailable.

Phase 23 applies this boundary to:

1. workspace.shell;
2. external Provider plugin package processes.

Remote MCP/OpenAPI/model HTTP execution does not spawn a governed local child and therefore does not use this process isolation boundary.

### Restricted-host contract

restricted_host must guarantee at least:

- writable host paths are limited to explicit writable roots plus bounded temporary/runtime paths required by the child transport;
- the configured workspace/package root is explicitly visible;
- process lifetime remains tied to the Mender-owned child lifecycle;
- Mender passes a bounded environment rather than the complete host environment;
- failure to construct/apply isolation prevents child execution.

The backend may be stronger than this minimum.

Phase 23 does not claim VM/container isolation, kernel-level CPU/memory accounting, or network egress policy.

### Platform backends

#### macOS

Use the OS Seatbelt sandbox through sandbox-exec when available.

The profile is deliberately restricted_host rather than a claimed hermetic container:

- normal system/runtime reads needed to execute binaries remain available;
- reads/writes under the Runtime owner's home are denied except explicit permitted roots;
- writes outside explicit writable roots and bounded temporary/device paths are denied;
- process execution and network access remain available in Phase 23.

Mender passes roots through a host-owned policy builder rather than accepting policy fragments from Tool or Provider input.

**Implementation note (2026-10-03):** Darwin 27 testing showed that path filters supplied through `sandbox-exec -D` were not reliable for the required read/write boundary. The implemented Seatbelt backend therefore canonicalizes host paths first, applies strict SBPL string escaping, generates literal/subpath rules inside the host, and explicitly denies Runtime-owner HOME siblings outside the permitted root. Tool arguments and Provider protocol/config cannot inject SBPL. The backend probe verifies workspace read/write plus denial of a HOME sibling secret before reporting `restricted_host` as available.

#### Linux

Use bubblewrap when available.

Build a private mount/process namespace with required system paths mounted read-only, explicit workspace/package roots mounted at canonical paths, writable roots bind-mounted read-write, private proc/dev/tmp/runtime scaffolding, and a private PID/session boundary. Host networking is retained in Phase 23.

No host/home tree is mounted merely for convenience.

#### Windows / unavailable backend

restricted_host is unsupported until a backend with equivalent minimum guarantees exists.

Selecting restricted_host fails closed. host remains explicit and available.

## Runtime isolation settings

Mender persists one RuntimeIsolation singleton:

~~~text
WorkspaceShell = host | restricted_host
ExternalProvider = host | restricted_host
~~~

Fresh defaults:

- WorkspaceShell = restricted_host when the backend is available at first initialization, otherwise host;
- ExternalProvider = host (**historical Phase 23 baseline**; replaced for new settings rows by ADR-0042).

The original default asymmetry was chosen for external-plugin compatibility. **ADR-0042 now chooses restricted_host for fresh external Providers when verified support is available**, while preserving any persisted host choice; operators can explicitly select host for plugins requiring additional filesystem access. Neither mode implies network-egress policy or a VM/container boundary.

Persisted restricted_host never silently degrades to host when support later disappears.

## Workspace shell

workspace.shell no longer constructs exec.Cmd directly.

It receives a Mender-owned ProcessLauncher from the built-in plugin composition root.

The launcher receives executable + arguments, canonical working directory, canonical permitted root, bounded environment, and isolation mode.

The shell Tool keeps its existing input/output/time/output-byte contract. Process isolation is host-owned and not controlled by Tool arguments.

## External Provider plugins

Provider plugin transport gains a host-supplied command launcher boundary while preserving exact executable checksum verification.

The host verifies the exact external plugin executable before the isolation wrapper starts it. A wrapper command must not cause Mender to hash only sandbox-exec/bwrap instead of the plugin binary.

The external process Supervisor supplies the installation package working directory as the permitted root and uses RuntimeIsolation.ExternalProvider.

Provider protocol messages, Credentials, and Plugin config cannot request weaker isolation.

## Audit and observability

Invocation Audit adds bounded budget classification only; it does not persist full mutable RuntimeBudget snapshots in Phase 23.

Provider activation Audit distinguishes Runtime budget rejection from Policy/Approval/runtime failures.

Management status exposes current RuntimeBudget, RuntimeIsolation, restricted_host backend availability/name, and current in-flight aggregate counts without exposing Tool arguments.

## Persistence

Add forward-only Runtime governance schema with singleton Budget and Isolation settings.

Existing installations receive the same defaults used for fresh Runtime creation. Migration is data migration, not a runtime fallback parser.

## Hard boundaries

Phase 23 does not add rate-per-window limiting, monetary/token budgets, CPU shares, memory cgroups, disk quotas, VM/container lifecycle, or network egress allow/deny policy.

It does not kill already-running actions when settings are lowered, and Profile/Tool arguments cannot weaken Runtime limits or isolation.

## Alternatives considered

### Reuse Policy rules for numeric budgets

Rejected. Policy decides authorization; Runtime Budget protects finite execution capacity after authorization.

### Keep Unlimited and rely on timeouts

Rejected. Timeout bounds duration but not simultaneous resource consumption.

### Queue when saturated

Rejected because an unbounded request queue itself becomes a resource-consumption surface. Saturation is explicit and fail-fast.

### Treat cwd containment as shell isolation

Rejected. A host shell can still traverse host paths regardless of its initial cwd.

### Silently fall back to host

Rejected because effective security would depend on ambient machine state without being visible in configuration.

### Claim restricted_host is a full sandbox

Rejected. Phase 23 does not promise VM/container isolation or network egress control.

## Consequences

Positive:

- Runtime load has deterministic concurrency and byte/time ceilings;
- one Principal/Provider/Tool cannot consume all Tool execution slots;
- Provider activation cannot start an unbounded number of expensive capabilities;
- workspace.shell can run under an explicit containment contract;
- external Provider isolation can be enabled without changing Provider Plugin protocol semantics;
- management can explain both configured limits and backend availability.

Costs:

- Runtime gains persistent governance settings and dynamic lease accounting;
- Invocation/budget rejection and activation audit contracts expand;
- native process launch becomes platform-aware;
- restricted external plugins may require explicit host mode;
- Windows restricted_host remains unavailable in this phase.

## Follow-up

Phase 23 implements this decision in segmented commits and records verification in its plan.

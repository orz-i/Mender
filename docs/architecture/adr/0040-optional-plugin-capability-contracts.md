# ADR-0040: Versioned optional capability contracts without a Mender job runtime

- Status: Accepted
- Date: 2026-10-13
- Refines: ADR-0035, ADR-0037 and ADR-0039; does not replace their ownership or authorization decisions

## Context

`plugin.Descriptor.Capabilities []string` can name arbitrary operations, but the executable `sandbox.domain.Capability` enum, `SandboxBackend.Capabilities` booleans, `WorkspaceSession` and Sandbox Plugin Protocol v1 are **Workspace-specific** (`file.read`, `file.search`, `file.edit`, `process.shell`). `toolsuite.domain.Definition` has a reviewed, static suite catalogue, but its plain string-set comparison cannot specify protocol compatibility, detached execution or recovery guarantees. SQLite profile grants and the suite binding are likewise restricted to Workspace operations and `sandbox.workspace` respectively. `AgentTask` is a different bounded context, and `invocation.OutcomeKindTask` does not provide a remote execution engine; MCP Tasks is a separately negotiated extension.

A capability announcement must never turn a vendor-controlled declaration into permission to execute. We need to make the difference between *support claimed*, *host-verified executable capability*, *suite eligibility*, *Profile exposure* and *task lifecycle* explicit without creating a generic job manager.

## Decision

1. **Capability identity is an operation, not a Sandbox type or a Tool.** Use an opaque, bounded, dot-separated ID (e.g. `file.read`, `browser.session`, `job.submit`) with a **contract version** `{major, minor}`. Compatible negotiation requires the same ID and major version, and an offered minor version at least as high as requested. Major changes are breaking. Package version, typed extension protocol version and capability contract version are independent.
2. **Execution traits are specific, not one `async` boolean.** An offer/requirement declares `inline` (bounded request/reply) or `detached` (submission decoupled from the request), plus explicit, independently testable guarantees such as `idempotent_submit`, `reconnect_observable`, `persisted_result`, `log_cursor`, `cancel_request`, and `checkpoint_restore`. Detached alone is *not* durable and does not imply machine-reboot recovery or exactly-once remote effects. Unknown or inconsistent traits fail validation, and negotiation requires every declared requirement without coercion.
3. **Declare versus verify.** Capability offers are immutable, bounded metadata of an exact plugin extension, never a trusted isolation/authentication assertion. At binding or execution, Mender must verify offers against a host-reviewed backend and an actual supported typed operation. Sandbox Protocol v1 carries only four Workspace operations: map its verified workspace subset to `v1.0/inline` **without** inferring jobs/browser/durable features. A future typed Sandbox extension protocol can publish additional offers only after a new verified execution port and protocol negotiation; simply adding a string to `Descriptor.Capabilities` is insufficient. No automatic host fallback.
4. **Tool Suite remains a separately reviewed, host-owned definition and resource binding.** It expresses required operation contracts and stable tool source keys. It is neither an additional native extension kind nor an implicit Provider/Tool/Profile grant. A Sandbox may eventually bind several optional suites, but only when each is independently verified and bound to the exact resource; the existing `sandbox.workspace` facade continues unchanged. An unavailable or incompatible suite must never advertise runnable tools. A Browser-only offer cannot satisfy Workspace and a detached `job.submit` does not satisfy `process.shell`.
5. **Execution ownership stays in the backend.** SSH Durable Jobs, Docker Jobs, Kubernetes Jobs and Browser sessions implement their own operation-specific ports and task state. Mender Core retains Plugin Registry, governed Tool invocation, Sandbox admission, short-lived acquire/dispatch, Profile, Policy, Audit, stable references and lifecycle fencing. No global RemoteTask aggregate, generic job Reconciler, remote Scheduler or Agent loop.
6. **Lifecycle obligations are not task state.** Disabling/uninstalling a live backend, deleting a bound suite, and stopping/destroying a Sandbox must be denied when the plugin has outstanding execution/cleanup obligations or cannot provide a trustworthy absence proof. Future implementation must close check-then-delete races with an atomic submission/lifecycle fence; no remote effect is automatically replayed after an unknown acknowledgment. Plugin-owned TaskID, store, reconciliation and result recovery remain plugin-owned.
7. **MCP Tasks is not the implementation dependency.** First expose optional `sandbox.jobs` via regular governed tools; only map to the `io.modelcontextprotocol/tasks` extension when capability negotiation and Task ownership/recovery have explicit acceptance tests. Do not bypass `ErrTaskUnsupported` in the current MCP adapter.

## Stage A implementation boundary

Introduce an inert, versioned capability offer/requirement value contract and strict negotiation; let the existing reviewed Workspace suite use verified `v1.0/inline` offers. Keep `Descriptor.Capabilities` and current HTTP/SDK/Desktop `requiredCapabilities` projection stable. Stage A **does not** register executable `sandbox.jobs`, extend native Sandbox Protocol v1, change Sandbox Profile Grant tables, generalize the `sandbox.workspace` SQLite binding or touch remote machines. The next stage must implement real SSH job mechanics, and only the subsequent supply stage may expand persistent suite bindings, authorization and management UI.

## Rejected alternatives

- A single universal `Backend.Execute(any)` with arbitrary capability strings: would turn advertised metadata into a privileged execution surface and blur Workspace, Browser and Jobs.
- A global Mender RemoteTask table/worker/Reconciler: makes the host an unintended execution authority and duplicates backend state.
- Treat every asynchronous operation as durable or assume MCP Tasks support: incorrect after network and process failures and incompatible with non-negotiating clients.
- Install a separate `tool-suite` native extension for each operation group: duplicates the existing governed Provider facade and creates an unnecessary new trust/runtime boundary.

## Consequences / acceptance

- More explicit version and trait checking at binding; missing, malformed, overclaimed or incompatible offers fail closed with no process/secret access from catalogue reads.
- Exact existing ProviderID/ToolID/Profile ToolBinding, SourceKeys, Workspace grants, Sandbox run generation and leases remain stable.
- Future Jobs/Browser execution require new typed, verified ports and lifecycle acceptance; no automatic widening of `sandbox.domain.Capability` or native protocol v1.
- The plugin may own jobs indefinitely, but the host must still fence its resource lifecycle without retaining a global job state machine.

## Sources

- MCP Tasks extension, `2026-07-28`: https://tasks.extensions.modelcontextprotocol.io/specification/draft/tasks
- MCP protocol release: https://blog.modelcontextprotocol.io/posts/2026-07-28/
- Existing Mender ADR-0039 and `contracts/sandboxplugin/v1/sandbox_plugin.proto`

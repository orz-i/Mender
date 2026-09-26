# Phase 3 Plan — Invocation, Policy, Credential, and Audit

## Objective

Establish one protocol-independent execution pipeline shared by all future adapters.

Phase 3 defines:

- Invocation lifecycle and result variants.
- Policy decisions and approval gates.
- Credential references and execution-boundary resolution.
- Data-minimized audit records.
- Timeout and cancellation behavior.
- Executor routing independent from MCP or CLI.

It does **not** implement MCP, profiles, user-facing credential configuration, or remote authorization.

## Architectural ownership

Phase 3 keeps four domain boundaries explicit:

```text
Catalog
  owns Tool / Provider metadata
       |
       v
Invocation application orchestration
  |          |             |
  v          v             v
Policy    Credential      Audit
```

Invocation may collaborate with other bounded contexts through explicit domain types and application ports. It must not query their persistence tables directly.

## Invocation model

### Invocation ID

`InvocationID` is an opaque Mender-generated stable identifier:

```text
inv_<32 lowercase hex chars>
```

It is distinct from any upstream request, task, or transport identifier.

### Request

The initial application request contains ToolID, JSON arguments, an optional caller label for audit attribution, and an optional timeout.

Arguments are protocol-independent JSON bytes. Phase 3 validates that arguments contain one JSON value but does not implement full JSON Schema evaluation.

### Lifecycle

The shared pipeline is:

```text
Allocate Invocation ID
        |
        v
Resolve Tool from Catalog
        |
        v
Evaluate Policy
        |
        +--> Deny -----------> Audit Denied -> return
        |
        +--> RequireApproval
        |          |
        |          v
        |      Approval Gate
        |          |
        |          +--> deny -> Audit Denied -> return
        |
        v
Create execution timeout/cancellation context
        |
        v
Route to Tool Executor
        |
        +--> executor may resolve Credential References
        |    through a restricted resolver capability
        |
        v
Normalize Result
        |
        v
Audit terminal state
        |
        v
Return Invocation Result
```

Every future CLI, MCP, Skill, Web, and Desktop execution path must enter through this pipeline.

## Result model

Invocation must represent more than immediate success/failure.

### Complete

Execution completed and returned one JSON value.

### InputRequired

Execution cannot continue without additional caller/model input. It carries an executor-scoped request ID, a message, and an optional JSON Schema.

### Task

Execution has transitioned to asynchronous work. It carries an executor-scoped task ID, status, and optional message. Poll/cancel task APIs are deferred until a concrete provider adapter requires them.

Infrastructure/execution errors are returned as Go errors and reflected in Audit. Failure is not modeled as arbitrary tool result content.

## Policy model

Canonical decisions are:

```text
Allow
Deny
RequireApproval
```

Each decision includes a machine-stable reason code and optional human-readable explanation.

The evaluator receives InvocationID, canonical Tool metadata, caller label, and whether credential resolution capability exists. Arguments are not sent to policy by default in Phase 3. Argument-aware policy requires a future explicit redaction/classification design.

`RequireApproval` invokes an application-owned ApprovalGate port. The approval request contains InvocationID, Tool identity/display metadata, policy reason, and caller label. It never contains resolved credentials.

## Credential model

Canonical references use:

```text
credential://<namespace>/<name>
```

Examples:

```text
credential://github/work
credential://slack/team-a
```

Secret values are never embedded in the reference.

The shared executor contract receives a restricted `CredentialResolver` capability. Only executor infrastructure should call `Resolve(ctx, reference)`. The Invocation application pipeline passes the capability through but does not resolve, inspect, log, hash, persist, or return secret values.

User-facing secret-store configuration is deferred.

## Executor boundary

A ToolExecutor is an outbound application port owned by Invocation. It receives InvocationID, canonical Catalog Tool, JSON arguments, and a credential resolver capability. It returns one canonical Invocation result variant.

An ExecutorRouter selects an executor for a Tool. Initial routing is by ProviderID. Phase 4 MCP provider adapters will implement this same port.

## Timeout and cancellation

- Timeout must be positive when explicitly set.
- A repository default applies when no timeout is supplied.
- The application creates a child context for executor work.
- Parent cancellation propagates.
- `context.DeadlineExceeded` maps to Audit status `timed_out`.
- `context.Canceled` maps to Audit status `cancelled`.
- No retry is performed automatically in Phase 3.

## Audit model

Audit is data-minimized by default.

An Audit Record stores InvocationID, ToolID, ProviderID, caller label when supplied, start/finish times, policy decision kind and reason, approval outcome, terminal status, result kind, and stable error classification.

Normal Audit Records explicitly do **not** store:

- Tool arguments.
- Tool result content.
- Credential references.
- Secret values.
- Environment variables.
- Raw upstream protocol frames.
- Stack traces.

Initial terminal statuses are `completed`, `input_required`, `task`, `denied`, `failed`, `cancelled`, and `timed_out`.

An Audit Record is written once per terminal Invocation outcome.

Stable error classes are `tool_not_found`, `policy`, `approval`, `executor_not_found`, `execution`, `cancelled`, `timed_out`, and `internal`.

## Work packages and commit segmentation

1. Governance and domain contracts.
2. Shared Invocation application pipeline.
3. SQLite Audit persistence and local executor infrastructure.
4. Composition and closeout.

Suggested commits:

```text
docs: define invocation security and lifecycle plan
feat(runtime): add invocation policy credential and audit domains
feat(invocation): add shared execution pipeline
feat(audit): add sqlite invocation audit persistence
feat(invocation): add local executor infrastructure
build(runtime): wire shared invocation pipeline
docs: complete invocation policy credential audit phase
```

## Exit checklist

- [ ] Invocation model represents Complete.
- [ ] Invocation model represents InputRequired.
- [ ] Invocation model represents Task.
- [ ] Policy Deny prevents executor execution.
- [ ] RequireApproval calls the approval gate.
- [ ] Approval denial prevents executor execution.
- [ ] Credential resolver is only consumed by the executor boundary.
- [ ] Secret material is absent from Audit domain records and SQLite rows.
- [ ] Parent cancellation is tested.
- [ ] Timeout behavior is tested.
- [ ] Built-in/fake execution traverses the full pipeline.
- [ ] SQLite Audit integration tests pass.
- [ ] Domain packages have no SQLite/MCP dependency.
- [ ] `pnpm check` passes.

# ADR-0007: Centralize execution with data-minimized audit and execution-boundary credentials

- Status: Accepted
- Date: 2026-09-25

## Context

Mender will eventually accept tool calls from CLI, MCP, Agent Skills, Web, and Desktop clients and route them to local or remote providers.

If adapters execute tools directly, policy, approval, timeout, credential handling, and audit behavior will diverge. That would make security depend on the entry point.

Credential values create a separate risk. Passing resolved secrets through general application objects makes accidental logging, persistence, serialization, or prompt exposure more likely.

Audit data creates another risk. Recording complete requests/results is convenient for debugging but can persist credentials, personal data, source code, database results, and other sensitive payloads indefinitely.

## Decision

Mender will have one shared Invocation application pipeline:

```text
Resolve Tool
  -> Evaluate Policy
  -> Approval when required
  -> Apply cancellation/timeout
  -> Execute
  -> Audit terminal outcome
  -> Return canonical result
```

Protocol/UI adapters may translate inputs and outputs but may not bypass these stages.

Credential values are resolved only by execution infrastructure. The Invocation application layer may pass a restricted resolver capability to a ToolExecutor, but it will not resolve or inspect secret values itself.

Normal Audit Records use data minimization. They persist invocation/tool/provider identity, policy/approval outcome, lifecycle status, timestamps, result kind, and stable error classification. Failed records may additionally persist one bounded, single-line diagnostic summary supplied by an executor. The Invocation boundary redacts common authorization/token/API-key/password forms before persistence, and the Audit Domain limits the summary to 512 Unicode code points. Audit still does not persist Tool arguments, full result content, Credential references, or raw protocol traffic.

Invocation supports canonical result variants for immediate completion, additional input requirements, and asynchronous task handles.

## Alternatives considered

### Let every adapter execute providers directly

This duplicates policy, credentials, timeout, and audit behavior and makes security inconsistent.

### Resolve credentials before calling the executor

This gives the application layer direct access to secret values and increases the number of components capable of accidentally retaining or exposing them.

### Persist full invocation arguments and results

This maximizes debugging data but conflicts with least-retention and can silently persist secrets or sensitive tool output.

### Make Audit an executor responsibility

Executors would record different metadata and could omit denied calls that never reached execution.

## Consequences

### Positive

- All adapters share identical execution semantics.
- Denied and approval-rejected calls are auditable.
- Secret values have a narrow runtime exposure boundary.
- Default audit storage has a substantially smaller sensitive-data surface.
- MCP and future provider adapters implement one executor contract.

### Negative / trade-offs

- Full request/response debugging is not available from normal Audit storage. Failure diagnosis is intentionally limited to stable classification plus an optional bounded/redacted summary.
- Tool executors must explicitly use a resolver to obtain credentials.
- Future argument-aware policies need a separate redaction/classification design.
- Async task management requires later APIs beyond the initial Task handle.

## Follow-up

Phase 3 will implement and test the shared pipeline, Audit persistence, and executor infrastructure.

Profile-aware policy and remote identity/authentication will build on this pipeline rather than introduce alternate execution paths.

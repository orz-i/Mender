# ADR-0018: Represent expected invocation rejections and failures as canonical outcomes

- Status: Accepted
- Date: 2026-09-26

## Context

The Phase 3 implementation models successful Tool execution with canonical Result variants:

- Complete;
- InputRequired;
- Task.

Most other conditions are returned as Go errors.

That distinction is too coarse for a tool gateway. In particular, an upstream MCP Tool returning "isError: true" is currently converted into a Go error. When Mender exposes that Tool through downstream MCP, the error can become a server/RPC failure instead of a Tool result that an agent can inspect and recover from.

The same ambiguity applies to expected governance outcomes such as invalid Tool arguments, Profile-scope rejection, policy denial, approval denial, timeout, or result-guard rejection. These are meaningful Invocation outcomes, not necessarily evidence that Mender itself is broken.

At the same time, Mender must not hide internal failures such as persistence corruption, Audit write failure, violated runtime invariants, or impossible mapping states behind normal Tool results.

## Decision

Invocation will expose one protocol-independent InvocationOutcome model.

Initial outcome kinds are:

~~~text
complete
input_required
task
rejected
failed
~~~

The first three retain the semantics of the existing canonical Result variants.

### Rejected

"rejected" means Mender intentionally prevented execution or could not accept the requested invocation before provider execution.

It carries:

- a stable Mender reason code;
- a bounded human/agent-readable message;
- no secret material or arbitrary raw arguments.

Initial rejection families include:

- Tool not available in the requested scope;
- Tool not found;
- invalid arguments;
- policy denial;
- approval denial.

Policy and approval evaluation errors are not the same as a deliberate denial; infrastructure failures in those components remain internal errors or runtime failures according to their nature.

### Failed

"failed" means an invocation entered or attempted execution but did not produce an acceptable successful/continuation outcome.

It carries:

- a failure source;
- a stable Mender code;
- bounded safe diagnostic content/message where available.

Initial sources include:

- "tool": the Tool/provider explicitly reported an application-level Tool failure, including upstream MCP "isError: true";
- "provider": transport/provider execution failed before a valid Tool outcome was produced;
- "runtime": timeout, cancellation, executor unavailability, or another expected operational runtime failure;
- "result_guard": execution returned data that failed Mender postflight validation or limits.

A failure outcome does not imply that its diagnostic is persisted. Audit continues to retain only bounded/redacted summaries and stable classifications.

### Internal Go errors

Go errors from the Invocation application boundary are reserved for failures that prevent Mender from reliably returning a governed outcome, for example:

- Invocation ID allocation failure;
- persistence/Audit failure when the required record cannot be written;
- violated internal invariants;
- malformed state that should have been impossible after validation;
- unexpected application/infrastructure faults for which no safe canonical outcome can be constructed.

The implementation may still use Go errors internally between stages. The application boundary classifies expected operational conditions into InvocationOutcome before returning to adapters.

### Adapter mapping

Adapters map canonical outcomes without redefining their meaning.

#### MCP

- Complete/InputRequired/Task map through protocol-supported success/continuation semantics.
- Tool/provider/runtime/result-guard failures that are safe to expose map to Tool-call error results ("isError: true") rather than automatically becoming transport/server errors.
- Rejections map to a safe Tool-call error result when the request reached Invocation.
- Genuine internal Go errors remain MCP/server errors.

#### CLI and Agent Skill

Machine-readable JSON always represents the canonical outcome when Invocation produced one.

Rejected/failed outcomes may use non-zero process exit status while preserving one structured JSON document on stdout in "--output json" mode. Human diagnostics remain on stderr.

#### Management HTTP / Desktop

A completed Invocation use case returns the canonical outcome body, including rejected/failed kinds. HTTP transport errors are reserved for malformed management requests that never enter Invocation or internal server failures.

The desktop binding exposes the same outcome contract.

### Audit

Audit remains data-minimized.

Phase 9 records enough metadata to distinguish outcome kind, stable reason/failure classification, execution scope/origin, and terminal lifecycle status without storing raw arguments or full outcome content.

Existing persisted Audit rows are migrated forward without rewriting historical payloads into data that was never recorded.

## Alternatives considered

### Keep every non-success path as a Go error

This is simple for Go code but loses Tool-vs-runtime semantics at adapter boundaries and makes agent recovery depend on protocol/server error parsing.

### Add only a ToolFailure Result variant

This fixes upstream MCP "isError" mapping but leaves expected scope/validation/policy/runtime outcomes split across a separate error channel. Adapters would still need inconsistent classification logic.

### Convert every Go error into a normal outcome

This would hide internal corruption and reliability failures from operators and make it difficult for adapters to distinguish a governed Tool failure from a broken Mender runtime.

### Reuse HTTP status codes or MCP error types as the canonical model

Those are adapter/protocol representations and would leak transport semantics into Invocation Domain.

## Consequences

### Positive

- Agents can receive and react to Tool failures without interpreting them as broken MCP transport.
- All adapters share one failure/rejection vocabulary.
- Policy/scope/input rejections become testable application outcomes.
- Internal runtime failures remain visible instead of being disguised as Tool errors.
- Audit can classify what happened without persisting full Tool payloads.

### Negative / trade-offs

- Existing Result APIs, CLI JSON projections, OpenAPI schemas, desktop bindings, and MCP mappings must migrate.
- Callers can no longer assume "err == nil" implies a successful Tool operation; they must inspect outcome kind.
- CLI exit behavior needs explicit compatibility tests.
- Some upstream provider errors cannot be classified more precisely than a bounded provider/runtime failure.

## Follow-up

Phase 9 implements the outcome model before updating adapters, so adapters migrate toward one canonical contract rather than inventing temporary mappings independently.

Future retry policy may consume stable failure codes, but Phase 9 does not add automatic retries or infer retryability when a provider does not declare it.

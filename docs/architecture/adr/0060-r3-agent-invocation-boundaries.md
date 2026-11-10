# ADR-0060: Separate Agent use cases and Invocation stages in R3

- Status: Accepted for R3
- Date: 2026-11-05
- Refines: ADR-0002, ADR-0033, ADR-0038, ADR-0059
- Plan and evidence: R3 implementation (historical)

## Context

R0-R2 restored the Provider and management application boundaries. Agent still
combines resource configuration, Task/Session execution and native permission
handling behind one broad storage dependency. Local filesystem mechanisms are
inside application code, including approval-policy symlink rechecks that the
import-only checker did not detect. Invocation has a single authoritative
entry point but mixes preflight, admission, execution and terminal mapping in
one large implementation.

The user approved R3 on 2026-11-05. This decision implements the previously
reviewed R3 scope; R4-R7 remain separate work.

## Decision

1. Keep the Agent Service as the transport-facing facade. Resource management,
   Task/Session and native permission use cases consume smaller storage ports.
   The existing SQLite Store can implement all of them; no persistence split
   or schema migration is introduced.
2. Exactly one Agent runtime coordinator owns active tasks, cancellation,
   pending permission handoff, shutdown and goroutine accounting. Use-case
   separation must not copy these maps, locks or lifecycle identities.
3. Define a consumer-owned LocalWorkspaceVerifier with path resolution,
   directory observation and regular-file observation operations. The local
   infrastructure adapter implements these using the filesystem. Inject it
   explicitly; missing verification must never silently accept a path.
4. The application retains authorization rules: owner admission before
   inspection, canonical-root equality, approved workspace identity, relative
   preview validation, containment and the existing domain error mapping.
   Preserve the observation order and recheck points, including automatic
   approval and resume. Filesystem observations are cooperative validation,
   not isolation or a guarantee against later external filesystem mutation.
5. Keep NewInvokeTool and InvokeTool.Execute as the sole public Tool invocation
   boundary. Private, named stages organize preflight, admission, bounded
   execution and finalization. One attempt carries the stable invocation ID,
   start time and single Runtime budget snapshot.
6. Centralize terminal Outcome/Audit/Event handling without changing outcomes,
   audit context choices, event order or error joining. Policy evaluation,
   ApprovalGate, credential resolution and durable audit retain their owners.
   No generic workflow framework or additional execution entry is added.

## Preserved behavior

- Profile scope is checked before Tool lookup; schema and reviewed facts precede
  policy, binding tightening and approval.
- The execution deadline begins after approval. Runtime capacity precedes the
  binding lease, route acquisition and executor call.
- Existing execution releases the binding lease, then runtime capacity, when
  the executor returns, before result guarding and durable terminal audit.
  Preserve that concrete critical section rather than changing it during the
  extraction to match a higher-level diagram.
- Audit failure cannot produce a successful response or successful terminal
  observer event. Best-effort observation does not replace durable audit.
- Resource admission, ACP native permissions and Mender Tool authorization stay
  separate. Policy revision fencing, approval expiry, cancellation, recovery,
  unknown outcomes and exact credential capabilities retain their semantics.
- HTTP/CLI/Wails/MCP contracts, IDs, configuration revisions and external
  protocols remain unchanged. Workspace typed execution, Profile delivery and
  frontend capability restructuring remain outside R3.

## Enforcement and validation

Remove the three exact Agent filesystem import exceptions after the verifier
is injected. Extend the architecture check to detect filesystem-bearing
filepath operations so aliases cannot hide the mechanism behind an allowed
lexical-path import. Pure lexical path operations remain permitted.

Reuse existing Agent configuration, task/resume, approval revision/handoff and
Invocation policy/binding/credential/outcome suites. Add focused tests for the
new verifier dependency and shared coordinator, filesystem mechanism behavior,
stage/lease ordering and audit failure without a success event. Run affected
normal/race tests, backend lint, architecture and binding parity checks, then
the repository's complete pnpm check. Report environment-limited fixtures and
the actual OAuth loopback environment separately from verified code behavior.

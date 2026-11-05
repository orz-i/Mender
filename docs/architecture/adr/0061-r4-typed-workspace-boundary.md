# ADR-0061: Type the Workspace execution boundary in R4

- Status: Accepted for R4
- Date: 2026-11-05
- Evidence: [R4 implementation](../../planning/refactor-r4-2026-11-05.md)

Workspace drivers currently decode Tool JSON and construct Invocation Outcomes.
That mixes Tool presentation and backend execution, duplicates defaults, and
makes backend cleanup and errors harder to verify independently.

## Decision

1. `workspace/domain` owns typed Read, Search, Edit and Shell requests/results
   and expected operation errors. It has no Invocation or Sandbox dependency.
2. `workspace/application` owns the consuming Session/LeaseProvider ports,
   shared defaults and one-operation lease cleanup. Drivers retain capability,
   path security, resource bounds, backend-specific behavior and uncertainty.
3. The Workspace Tool infrastructure adapter decodes arguments and constructs
   Invocation Outcomes. A Sandbox bridge resolves and acquires the admitted
   target and adapts its lease to the Workspace application port.
4. Migrate Local, Docker, Daytona, SSH, then External. A temporary legacy adapter
   supports the transition and is removed with the old JSON WorkspaceSession.
5. External infrastructure converts typed values to/from Sandbox protocol v1.
   SDK/proto, IDs, schemas, grants and lifecycle storage do not change. The v1
   request omission/default behavior is retained as a transport compatibility
   policy; built-in defaults belong to Workspace application.

## Invariants

- Policy-approved target facts still pin the exact Sandbox before acquisition.
- No backend fallback or automatic write retry is introduced.
- Session close always uses the original lease. Cleanup failure takes priority
  over a Tool result and retains the existing uncertain durable lease behavior.
- Expected Tool failures remain distinct from transport/execution errors.
  Invalid external results still produce output schema mismatch failures.
- Backend path restrictions, result fields, truncation and timeout differences
  are preserved. A typed contract does not imply identical backend guarantees.
- Optional-field presence crosses typed requests as explicit bits, preserving
  External v1 omitted versus false/empty values. No raw arguments cross the
  application port.
- External results carry only their host-measured, trimmed original byte count
  alongside typed values. Invocation receives that count as internal Outcome
  metadata, so re-encoding whitespace/HTML/numbers cannot change the v1 result
  budget decision. Invalid typed results retain size-before-schema precedence
  using the same Invocation budget snapshot; no second budget read is added.
- R5 Profile delivery/schema and later stages remain separate work.

## Dependency direction

`Workspace Tool adapter → Workspace application → Workspace domain`.
`Sandbox bridge → Sandbox application/domain + Workspace application/domain`.
`Sandbox backend drivers → Workspace application/domain + Sandbox contracts`.
Sandbox application retains generic session lifetime and does not own Tool JSON
or depend on Workspace application. Invocation remains the governed entry point.

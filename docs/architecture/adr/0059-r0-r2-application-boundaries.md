# ADR-0059: Restore application ownership in the R0-R2 refactor

- Status: Accepted for R0-R2
- Date: 2026-11-05
- Refines: ADR-0002, ADR-0010, ADR-0011, ADR-0032, ADR-0039, ADR-0042
- Preserves: ADR-0014, ADR-0038, ADR-0050, ADR-0055, ADR-0058
- Plan and evidence: [R0-R2 implementation](../../planning/refactor-r0-r2-2026-11-05.md)

## Context

Provider management workflows, Agent admission and a cross-resource read
projection have accumulated in `bootstrap`. HTTP and Wails independently
aggregate the same Provider management view. Sandbox creation requests expose
SQLite and backend implementation types. These are ownership problems even
where direct Domain/Application imports still point inward.

The user approved implementing R0-R2 on 2026-11-05. This decision extends the
bounded extraction in ADR-0042; it does not reopen its atomicity or isolation
decisions and does not authorize R3-R7.

## Decision

1. `management/application/query` owns transport-neutral Provider read models
   and query orchestration. HTTP and Wails retain their public transport DTOs,
   status codes and identity assurance boundaries. Projection adapters share
   non-secret configuration handling and presentation rules.
2. An explicitly reviewed read-only SQLite projection may join exact Agent and
   Sandbox facade ownership relations for management attribution. It lives in
   management infrastructure, exposes a narrow query port, uses read-only
   connections, and confers no mutation authority. This is separate from the
   commit-point persistence exception in architecture principle 5.
3. Provider plugin directory reads consume built-in metadata and persisted
   verified installation snapshots only. Missing snapshots produce bounded
   diagnostics. Descriptor verification is an explicit owner command using the
   existing trusted installation probe; listing must not backfill, close plugin
   instances, resolve secrets, or launch a probe.
   Each verification owns an independent temporary process supervisor with the
   existing host isolation/limits. Probe cleanup succeeds before publication;
   snapshot publication is fill-only CAS (same snapshot is idempotent, a
   different snapshot cannot replace the first). Runtime execution processes
   are never owned or closed by verification.
4. `providerplugin/application` owns configured Provider registration, update,
   synchronization, installation rebinding and retirement orchestration.
   Catalog retains canonical Provider/Tool identities. Existing protocol-neutral
   Configuration/Descriptor types and the Provider instance manager stay in
   `providerplugin`; that package must not import its application subpackage.
5. Provider use cases consume ports for Catalog operations, configuration,
   activation, instances, ownership and atomic retirement. Catalog snapshots
   cross an explicit Catalog application boundary. Agent and Sandbox ownership
   queries cross their respective application boundaries. SQLite retirement
   remains one commit point with existing reference guards; advisory reads do
   not replace its race protection.
6. `agent/application` owns Agent admission policy mapping, decision handling
   and admission audit. Preserve current behavior: only `allow` proceeds;
   `deny` and `require_approval` reject; audit failure rejects. This does not add
   interactive admission approval or change ACP native permission handling.
7. Sandbox application creation specifications are independent of SQLite,
   Docker, Daytona and SSH implementation packages. Infrastructure conversions
   preserve the existing validation, connection and persistence behavior.
8. `bootstrap` constructs these use cases and adapters. Existing Runtime entry
   points may remain thin delegates so CLI, HTTP and Wails contracts are stable.

## Invariants and scope

- No schema migration, dependency upgrade, new executor or protocol version.
- Preserve ProviderID, ToolID, SourceKey, configuration revisions, exact
  installation pins, Profile grants and invocation/activation ordering.
- Preserve rollback/compensation, reference guards and fail-closed isolation.
- Preserve existing HTTP/CLI machine output and Wails bindings; the explicit
  descriptor verification command is an additive owner operation.
- R3 Agent/Invocation internals, R4 Workspace typed execution, R5 Profile
  delivery/schema and R6 frontend/client capability restructuring are deferred.
- Do not touch production databases, installed plugins, credentials, profiles,
  remote resources or running native processes to validate this refactor.

## Architecture enforcement

An AST-based backend import check inspects production Go sources, including
platform-specific files. Domain/Application must not import infrastructure,
entry adapters, transport SDKs or direct database/process/filesystem mechanisms.
Domain must not import application packages. The Provider runtime-to-application
reverse edge is forbidden.

Existing direct Agent application filesystem reads are recorded as exact
file/import exceptions with R3 ownership. New exceptions require a reviewed
reason and removal stage; unused exceptions fail the check. This baseline is not
permission to expand a legacy edge or a claim that all bootstrap workflows have
already been extracted.

The import check enforces explicit dependency edges, not arbitrary runtime
side-effect freedom. Query purity, safe projection and process/transaction
ownership also require behavior tests and review.

R3 removed the temporary Agent filesystem edges. R7 retired the empty
exception file and its loader. See the
[R7 integration record](../../planning/refactor-r7-2026-11-05.md).

## Alternatives considered

- **Move files without extracting use cases:** leaves ownership and duplicated
  adapter behavior unchanged.
- **Introduce a new Provider domain and relocate Configuration simultaneously:**
  expands identity/API migration and risks cyclic or duplicated models.
- **Replace shared SQLite with separate databases:** breaks the established
  atomic reference-acquisition/retirement contract.
- **Build a universal management facade or workflow engine:** adds abstractions
  before the concrete duplicated use cases justify them.

## Consequences

The change adds small application ports and boundary mappings. Runtime callers
remain stable, and ownership is testable without transport or storage. A
separate documented read projection exception is needed. Full bootstrap cleanup
and backend/client contract redesign remain later work.

## Validation

Reuse existing Provider lifecycle, compensation, governance, reference-race and
transport projection fixtures. Add tests for changed admission/query/descriptor
behavior and read-only persistence. Run affected Go tests and race checks,
golangci-lint, architecture enforcement and contract/binding parity. Record
pre-existing platform failures separately from new failures; do not suppress or
rewrite unrelated fixtures to claim a clean result.

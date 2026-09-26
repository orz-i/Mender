# ADR-0014: Fail closed on future SQLite schemas and use forward-only migrations

- Status: Accepted
- Date: 2026-09-26

## Context

Catalog, Audit, Profile, and MCP Provider configuration each own SQLite migration tables. Known migrations run transactionally at store startup.

An older Mender binary can currently encounter a database that contains a migration version newer than it knows and continue startup. Even if today's schema happens to remain compatible, silently operating on a future schema makes downgrade behavior undefined.

Mender needs an upgrade policy before distributing versioned binaries.

## Decision

Each SQLite context remains migration owner.

Before applying known migrations, a store checks the maximum version recorded in its migration table. If that version exceeds the newest migration compiled into the binary, the store fails to open.

Migrations remain:

- forward-only,
- transactional per context,
- append-only in version history,
- explicitly tested for upgrade behavior.

No automatic downgrade migration is attempted.

Release notes must identify releases that add database migrations. Destructive or complex migrations require an explicit backup/recovery procedure before release.

A user may downgrade only when the target binary supports every recorded schema version; otherwise startup intentionally fails.

## Alternatives considered

### Ignore unknown future versions

Rejected because it makes rollback appear successful while schema semantics may have changed underneath the old binary.

### Centralize every context into one global schema version

Rejected because it weakens bounded-context ownership and couples unrelated persistence evolution.

### Automatically reverse migrations on downgrade

Rejected because reverse migrations are not generally lossless and would make rollback a destructive implicit action.

## Consequences

Positive:

- downgrade incompatibility is explicit and fail-closed,
- contexts retain migration ownership,
- future migration releases have a defined operational contract.

Trade-offs:

- some rollbacks require restoring a pre-upgrade backup rather than merely reinstalling an older binary,
- migration release notes and recovery discipline become mandatory.

## Follow-up

Phase 8 adds future-version rejection tests to all SQLite contexts and documents the operator upgrade flow.

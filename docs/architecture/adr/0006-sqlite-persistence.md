# ADR-0006: Use database/sql with a pure-Go SQLite driver for local persistence

- Status: Accepted
- Date: 2026-09-25

## Context

Mender is local-first, cross-platform, and expected to ship as CLI/server binaries and later as a Wails desktop application.

The initial Catalog requires transactional relational persistence and full-text search. SQLite satisfies the local deployment model and provides FTS5 without requiring an external database service.

The implementation also needs to preserve Clean Architecture boundaries. Domain and application packages must not depend on a SQLite-specific API or ORM model.

A CGO-dependent driver would complicate cross-compilation and desktop/server distribution.

## Decision

Mender will use Go's standard `database/sql` package with `modernc.org/sqlite` for the initial SQLite infrastructure implementation.

Catalog persistence will:

- Live under the Catalog infrastructure boundary.
- Use explicit SQL rather than an ORM.
- Enable foreign-key enforcement.
- Use FTS5 for initial lexical Tool search.
- Apply schema creation/migrations explicitly at database startup.
- Keep persistence row models private to infrastructure packages.
- Implement application-owned repository/search ports.

The repository may pin a concrete `modernc.org/sqlite` version in `go.mod`; this ADR chooses the driver family and boundary, not a permanent dependency version.

## Alternatives considered

### mattn/go-sqlite3

It is mature and widely used, but its CGO requirement complicates the local-first cross-platform distribution model.

### ORM over SQLite

An ORM can reduce repetitive CRUD code but introduces mapping conventions and runtime behavior that are unnecessary for the initial small schema. Explicit SQL makes transaction and FTS behavior easier to audit.

### Embedded key/value database

A key/value store simplifies embedding but makes uniqueness constraints, structured queries, and FTS/tool metadata search less direct.

### PostgreSQL first

PostgreSQL is appropriate for some future remote deployments, but requiring an external server would violate the first local runtime target.

## Consequences

### Positive

- No CGO requirement for the initial SQLite path.
- CLI/server cross-compilation remains simpler.
- FTS5 is available in the same local database.
- Domain/application code remains persistence-independent.
- SQL and transaction behavior are explicit.

### Negative / trade-offs

- The pure-Go SQLite implementation increases Go module dependency size.
- Explicit SQL requires manual mapping and migration maintenance.
- Remote multi-writer deployments may eventually require a different storage adapter.

## Follow-up

Phase 2 will introduce Catalog-owned migration, repository, and search integration tests.

A future storage change must preserve application port semantics rather than leak database behavior into the domain.

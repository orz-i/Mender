# ADR-0062: Separate Profile selection and delivery configuration in R5

- Status: Accepted for R5
- Date: 2026-11-05
- Baseline: `5e1ad38` (R4)
- Evidence: [R5 implementation](../../planning/refactor-r5-2026-11-05.md)
- Extends: [ADR-0058](0058-profile-unified-delivery-root.md)
- Upgrade policy: [ADR-0014](0014-sqlite-upgrade-compatibility.md)

## Decision

R5-A extracts Profile-scoped Skill delivery preflight, revision fencing and
compensation, CLI recipes and Agent Plugin export into
`profile/application/delivery`. Consuming ports delegate content, trust, digest,
drift and deployment persistence to Skill. Host path observation belongs to a
local infrastructure adapter. Runtime retains thin entry methods and composes
the collaborators once.

Profile-managed MCP listener, HTTP snapshot refresh and process ownership move to
`mcp/infrastructure/managed`. Runtime still closes the manager. Storage failures,
version regression and unstable snapshots fail closed. A request may refresh
its immutable Tool projection without restarting the listener or Tunnel.
The separate Agent Task-token MCP proxy remains an Agent execution adapter; it
does not acquire Profile delivery configuration or share its lifecycle owner.

R5-B migrates MCP mode, Tunnel binding and direct projection selection out of
Profile into explicit MCP delivery configuration. Profile owns membership,
presentation, restrictive policy/concurrency and credential constraints, with
its existing selection revision. MCP delivery owns a separate revision, linked
by immutable ProfileID. Configuring delivery checks both revisions and current
Tool membership atomically. New Profiles do not implicitly configure MCP.

Historical Profiles receive configuration preserving direct/dynamic/hybrid,
Tunnel references and effective projections. The forward-only migration keeps
identities, Skill membership/deployment ownership and reference guards intact.
It performs configuration conversion only; no Server/Tunnel, materialization,
grant or external resource operation runs as a migration side effect.

HTTP, CLI, Wails, SDK and affected screens switch together to the new authority.
Profile no longer publishes MCP-only fields or accepts direct flags in Tool
membership writes. MCP delivery exposes selection/configuration versions
separately. Existing one-time CLI and exported artifacts remain snapshots.
No compatibility authority, second registry or automatic republishing is added.

## Scope and release

Implement R5-A then R5-B and stop before R6. Frontend edits are limited to
consuming the new Profile/MCP delivery contracts; client capabilities and Tools
feature ownership remain later work. All migration/resource verification uses
temporary test databases and local fixtures.

This is a schema-changing release. Before upgrading a real installation, stop
writers/processes, take a consistent backup of the complete shared SQLite
database (including WAL state), and retain the matching managed resources.
Upgrade a disposable copy first. A rollback after schema upgrade requires the
matching old database/resource backup; replacing the executable alone is not a
downgrade. Changes written after upgrade require explicit reconciliation before
restoring that backup. Older binaries must reject the new schema.

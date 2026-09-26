# ADR-0008: Treat MCP as an anti-corruption layer and serve 2026-07-28 stateless HTTP

- Status: Accepted
- Date: 2026-09-25

## Context

Mender needs to consume MCP servers and expose its own controlled Tool catalog over MCP.

The official Go SDK v1.8.0 supports MCP 2026-07-28 and older revisions. The 2026-07-28 protocol removes the session handshake from the modern core and requires stateless Streamable HTTP in the Go SDK for that revision.

MCP also evolves independently from Mender. If SDK types enter Catalog, Invocation, Policy, Credential, Audit, or Profile Domain models, protocol evolution becomes a domain migration.

Mender additionally needs large-catalog discovery modes that are not equivalent to blindly returning every Catalog Tool from `tools/list`.

## Decision

MCP is implemented strictly as an adapter/anti-corruption layer.

MCP SDK types may be used by MCP infrastructure packages but must be mapped to and from canonical Mender models at the boundary.

Mender pins the official Go SDK v1.8.0 for Phase 4.

Remote Mender MCP serving uses Streamable HTTP with stateless mode enabled. New legacy HTTP+SSE server endpoints are not introduced.

Local MCP serving supports stdio.

Upstream providers may use stdio or Streamable HTTP and are represented by adapter-owned connection configuration rather than changing Catalog Domain entities.

A minimal Profile Domain controls server exposure with direct, dynamic, and hybrid modes. Profile membership is enforced before discovery and invocation.

All MCP tool calls into Mender route through the existing shared Invocation pipeline.

MCP tool annotations remain untrusted hints and are not promoted to Policy decisions.

The official MCP Tasks extension is not implemented in Phase 4 because the pinned Go SDK does not expose that extension through the stable core MCP API. Mender's canonical Task result remains ready for a later extension adapter.

## Alternatives considered

### Make MCP types the canonical model

This reduces mapping code but violates the established Tool-domain decision and couples every internal layer to protocol revisions.

### Expose every Catalog Tool directly

This is simple for small catalogs but conflicts with Mender's discovery-first architecture and creates context/tool-list scaling problems.

### Dynamic mode without Profile filtering

This would allow meta-tools to search or invoke tools that the server operator did not intend to expose.

### Stateful Streamable HTTP by default

This preserves older session behavior but prevents negotiation of 2026-07-28 in the official Go SDK and creates unnecessary deployment affinity/state.

### Implement MCP Tasks by accessing SDK internals

Reflection, unsafe access, or copied private protocol types would create an unstable unsupported dependency. Mender will wait for or implement a proper extension boundary instead.

## Consequences

### Positive

- MCP evolution remains isolated at adapter boundaries.
- Local and remote upstream MCP servers use one canonical Catalog/Invocation flow.
- Stateless HTTP supports ordinary horizontal deployment.
- Profile exposure prevents dynamic discovery from becoming a security bypass.
- Direct/dynamic/hybrid modes support both small and large Tool catalogs.

### Negative / trade-offs

- Mapping code is required in both directions.
- Upstream MCP tool renames currently produce new SourceKeys.
- Task extension support is deferred.
- Profile persistence and user management remain for Phase 5.
- Some upstream MCP metadata is not retained until Mender has a domain reason to model it.

## Follow-up

Phase 4 implements the adapters and tests all calls through the shared Invocation pipeline.

Phase 5 persists/manages Profiles through the CLI and exposes stable machine-readable local workflows.

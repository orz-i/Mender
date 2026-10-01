# Phase 21 Plan — Managed Profile Tunnels

Status: In Progress

## Objective

Add a first-class Tunnel management surface so a Profile served by Mender's loopback MCP server can be reached through an external tunnel adapter without weakening the local HTTP security boundary.

V1 implements OpenAI Secure MCP Tunnel.

Architecture is governed by [ADR-0031](../architecture/adr/0031-managed-profile-tunnels.md).

## Product boundary

Tunnel is downstream transport infrastructure:

~~~text
OpenAI surface
     |
     v
OpenAI hosted tunnel endpoint
     ^
     | outbound HTTPS
tunnel-client (Runtime-owned child)
     |
     v
127.0.0.1:<ephemeral>/mcp
     |
     v
Mender Profile -> Invocation -> Policy -> Tool Provider
~~~

It does not add:

- model calls;
- prompts or agent loops;
- public wildcard MCP listening;
- a generic reverse-proxy product;
- secret persistence inside Tunnel rows.

## Phase 21 scope

### Tunnel domain

Persistent Tunnel records contain:

- stable Mender Tunnel ID;
- display name;
- provider kind;
- OpenAI external tunnel ID;
- runtime Credential Reference;
- executable path/name.

V1 provider enum: `openai`.

### Profile binding

Profile gains an optional Tunnel ID.

The management surface lets users select or clear one Tunnel from a Profile without typing opaque IDs manually.

Tunnel deletion fails while referenced.

### Runtime lifecycle

Starting a managed MCP server for a tunneled Profile:

- preserves loopback-only HTTP binding;
- starts the local HTTP endpoint first;
- resolves the Tunnel runtime Credential;
- launches OpenAI `tunnel-client run` against the local endpoint;
- rolls back the MCP server if the child cannot start;
- stops the child on server Stop/Restart/unexpected cleanup/Runtime.Close.

### Management UI

Add Runtime -> Tunnels.

The page is list-first and supports OpenAI Tunnel creation and deletion.

Create form:

- Display name;
- OpenAI Tunnel ID;
- Runtime API-key Credential selector;
- optional executable override, default `tunnel-client`.

The page explicitly says OpenAI Secure MCP Tunnel keeps the MCP server private and makes it available to supported OpenAI products; it does not create a general public MCP URL.

Profile management adds a focused Tunnel selector.

Servers show Tunnel association and process status when present.

### Contracts/adapters

Add matching:

- management HTTP endpoints;
- OpenAPI schema;
- generated TypeScript SDK;
- Wails binding methods/types;
- Desktop client adapter;
- tests.

### Governance

Credential usage detection includes Tunnel references.

No secret value appears in projections or persisted Tunnel rows.

## Verification

Blocking checks:

- Tunnel domain validation and SQLite round-trip/migration compatibility;
- referenced Tunnel deletion conflict;
- referenced Credential deletion conflict;
- Profile Tunnel persistence and invalid-reference rejection;
- Runtime child-process start/stop/restart/rollback tests with a fake tunnel-client executable;
- management HTTP tests;
- generated SDK and Wails binding checks;
- React tests for Tunnels, Profile selector, and Server status;
- i18n parity;
- `pnpm check`;
- `pnpm security:check`.

## Deferred

- managing OpenAI tunnel resources through the OpenAI admin API;
- downloading/upgrading `tunnel-client`;
- readiness polling and local `/ui` integration;
- multiple tunnels per Profile;
- non-OpenAI tunnel providers.

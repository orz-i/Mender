# ADR-0043 — Hard retire dual MCP config persistence and Workspace v1 adapters

- Status: Accepted (2026-10-15)
- Parent: ADR-0024 Provider Plugin Runtime, ADR-0035 Sandbox Router, ADR-0039 Tool Suites, ADR-0042 P2 boundary closure.

## Context and evidence

An already canonical Plugin Platform still carries a **second MCP Provider configuration source**: legacy `mcp_provider_configs`, `mcp_config_schema_migrations` and a one-time `provider_config_imports` marker. Runtime startup imports it, native registration/update dual-write it, and legacy HTTP/Wails/SDK interfaces can mutate it independently of governed canonical Provider management. A legacy MCP-specific delete path performs multi-step compensation instead of the guarded atomic canonical retirement.

Workspace Provider v1→Sandbox v2 startup migration is also still invoked on every Runtime start despite Workspace v2-only being permanently deployed. The 2026-10-15 live read-only inventory had **one legacy MCP row** with a matching canonical `io.mender.provider.mcp` row, **zero Workspace v1 rows**, and no mismatched legacy Provider plugin identity. Both are retired operational formats.

## Decision: hard cut without a runtime bridge

1. **Canonical source:** only `provider_configs` and typed `io.mender.provider.mcp` Plugin Config exist at Runtime. Remove the full legacy MCP config store, startup importer, `legacyMCPProviderConfiguration`, dual-write/update/delete routines, and credential mapping adapters to legacy types. A consumer must use `RegisterConfiguredProvider`, `UpdateConfiguredProvider`, `DeleteConfiguredProvider` (with relevant owner admission) and the canonical Tool invocation path.
2. **Contracts:** remove `POST /providers/http`, `POST /providers/stdio`, legacy flat PUT Provider union variants, and obsolete Wails `CreateHTTPProvider`, `CreateStdioProvider`, `UpdateProvider` methods; keep generic `POST /providers`, `PUT /providers/{id}` (`config` required), `DELETE /providers/{id}`, sync/rebind. Remove SDK legacy methods and regenerate OpenAPI/Wails types. No request reinterpretation or compatibility shim. MCP transport choice remains **inside** the typed Plugin config.
3. **Data:** forward-only **Provider Config schema v4** atomically validates each existing MCP legacy row has a canonical Provider Config with the same ProviderID and MCP PluginID, with valid JSON and config version, then permanently drops `mcp_provider_configs`, `mcp_config_schema_migrations` and `provider_config_imports`. Canonical rows, ToolIDs, Profiles, Policy, Credentials, Tunnel bindings and audit survive unchanged. Missing or conflicting canonical mappings **fail closed without dropping anything**. Fresh databases never initialize legacy MCP stores. Old binaries cannot reopen v4 canonical schema.
4. **Workspace:** remove the one-time Workspace v1 decoder/migrator and its startup invocation; any persisted Workspace Provider not at canonical ConfigVersion 2 is a startup error (not auto-transformed). Guard future raw SQL non-v2 Workspace bindings. Keep v2 canonical configuration and Sandbox runs unchanged.
5. **Tests/quality:** isolated SQLite upgrade fixtures for valid retirement and rollback, guarded reintroduction refusal, generic create/update/delete/sync including MCP HTTP and stdio, stale endpoint/method rejection, unchanged ProviderIDs/ToolIDs, SSH Jobs/Sandbox/P1/P2 invariants, generator parity, full `pnpm check`, `pnpm security:check`, `git diff --check`.

## Operational and destructive boundary

Runtime code migration will delete **only** the named obsolete SQLite tables and their duplicate MCP configuration rows; it does not remove canonical resources, secret storage, old backup snapshots, or SSH Job remote state. Migration is executed on the operator's **next restart** with the new binary, not by mutating SQLite under the running older client. Take a validated pre-upgrade database snapshot before first deployment. Logical SQLite table removal is **not** guaranteed secure erasure of freed pages or historical database backups; cleanup of those backups is an independent retention/security decision. Old CLI/SDK/Wails clients using deleted interfaces must upgrade as part of this hard cut; no compatibility bridge will be added.

## Non-goals

Do not merge Provider/Sandbox/Agent ownership, create a new Agent loop, invent a second MCP adapter, purge still-referenced backups or secrets, or silently downgrade runtime process isolation.

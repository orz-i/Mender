# Phase 39 — Tools workspace and canonical source projection

Status: In progress · 2026-10-16
Architecture: [ADR-0046](../architecture/adr/0046-unified-tools-workspace.md)

## Scope and constraints

Implement **Option B** from the Provider/Catalog audit: unify the *product* entry and keep independent Provider and Catalog bounded contexts. No database reset, no new Source/Plugin inventory, no compatibility routes, no Agent loop. Keep stable ProviderID, ToolID, SourceKey, package installation pins, Credential References, existing Agent/Sandbox Tool Suite bindings, Profile ToolBindings, Policy, grants, audit and execution guards. Do not stage `.agents/`, `.pi/`, `.intent-loop/`.

## Bounded commits

- **A — Docs-first (commit before code).** ADR-0046, this plan, route/product language, schema and acceptance invariants.
- **B — Tools entry and focused create workflow.** `/tools` with Tool catalog/Sources switch in the shared toolbar; retire `/providers` and `/catalog` UI routes. Exact Marketplace create deep links, Overview, Agent/Sandbox diagnostics, tests and i18n. No duplicate Provider plugin picker as an independent discovery flow: Add Source routes through Marketplace filtered provider offerings (including reviewed MCP integrations) before focusing configuration.
- **C — Source attribution.** Host-owned read-only owner projection derived from exact existing Agent/Sandbox facade binding records. HTTP/SDK/Wails expose the same source kind/name/id. Catalog results show real resource name, independent Provider as source, unclaimed facades as diagnostics; Profile source filters follow the same truth. No guessed association from PluginID.
- **D — Complete paged discovery.** Bounded cursor pagination in canonical browse and FTS search with server-side provider and profile scoping. Stable ordering plus input-bound cursor, clear failure on invalid/stale values. Profile Tool Picker uses the paged backend instead of filtering first 100 locally. No full schema loads.
- **E — Acceptance and cleanup.** Focused Go/TS contracts; `pnpm check`, `pnpm security:check`, generated bindings, isolated actual UI; ensure no unintended user-data changes, remove obsolete code/wording and document final evidence. Close with bounded commits.

## Safety / release

Every schema/API shape update must compile across OpenAPI generated TS, HTTP API, CLI and Wails bindings. User data are only read in acceptance; test writes use disposable SQLite. A running Wails app must be rebuilt/restarted by the operator for new source to take effect. The previous routes are hard-retired, not redirected to an alternative implementation. If an unavoidable migration is identified, fail closed with a documented recovery path rather than automatically reassigning IDs.

## Evidence

To be filled only from executed tests and committed changes.

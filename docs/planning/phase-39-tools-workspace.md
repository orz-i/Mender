# Phase 39 — Tools workspace and canonical source projection

Status: In progress · 2026-10-16
Architecture: [ADR-0046](../architecture/adr/0046-unified-tools-workspace.md)

## Scope and constraints

Implement **Option B** from the Provider/Catalog audit: unify the *product* entry and keep independent Provider and Catalog bounded contexts. No database reset, no new Source/Plugin inventory, no compatibility routes, no Agent loop. Keep stable ProviderID, ToolID, SourceKey, package installation pins, Credential References, existing Agent/Sandbox Tool Suite bindings, Profile ToolBindings, Policy, grants, audit and execution guards. Do not stage `.agents/`, `.pi/`, `.intent-loop/`.

## Bounded commits

- **A — Docs-first (commit before code).** ADR-0046, this plan, route/product language, schema and acceptance invariants.
- **B — Tools entry and focused create workflow.** Stage B (`36fe731`) initially introduced a `catalog/sources` tab. The approved refinement replaces that intermediate product UI: `/tools` shows unified named service cards (AnySearch), `/tools/:providerId` shows *that service's Tools*. Remove the temporary tab and its query parameter; no old `/providers`/`/catalog` pages. Marketplace Connect/Create opens a focused form through `/tools?create=1&offeringId=…` or `pluginId=…`; resource-owned entries have owner detail navigation. Do not restore duplicate Provider plugin picker.
- **C — Source attribution.** Stage C (`0bd3e4a`) supplies Host-owned read-only attribution through exact existing Agent/Sandbox binding records (HTTP/SDK/Wails). Every service appears *uniformly* as an entry, never as an internal/external category. User-visible AnySearch instance name and Tool count take priority over underlying MCP/Provider metadata; owned resource Manage navigates to Agent/Sandbox. Unclaimed facade remains neutral and diagnostic. No guessed association from PluginID.
- **D — Service drilldown and complete paged discovery.** Service cards → exact source Tool list, with searched/browsed results strictly scoped to its ProviderID. Bounded cursor pagination in canonical browse and FTS search, server-side Provider/Profile scope, deterministic tie breaks, invalid/stale cursor failure. Profile Tool Picker uses the same paged backend rather than filtering the first 100 local results. No full schemas on list, no auto Profile grant.
- **E — Acceptance and cleanup.** Focused Go/TS contracts; `pnpm check`, `pnpm security:check`, generated bindings, isolated actual UI; ensure no unintended user-data changes, remove obsolete code/wording and document final evidence. Close with bounded commits.

## Safety / release

Every schema/API shape update must compile across OpenAPI generated TS, HTTP API, CLI and Wails bindings. User data are only read in acceptance; test writes use disposable SQLite. A running Wails app must be rebuilt/restarted by the operator for new source to take effect. The previous routes are hard-retired, not redirected to an alternative implementation. If an unavoidable migration is identified, fail closed with a documented recovery path rather than automatically reassigning IDs.

## Evidence

To be filled only from executed tests and committed changes.

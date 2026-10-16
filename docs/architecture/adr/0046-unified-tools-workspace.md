# ADR-0046 — One Tools workspace; keep Provider and Catalog distinct

- Status: Accepted
- Date: 2026-10-16
- Refines: ADR-0039, ADR-0045 and the Phase 2 Catalog contract

## Problem

The current sidebar separately advertises Provider and Catalog. Providers are *configured sources of canonical Tools*, not installed Plugin Packages. The global Catalog is a flat Tool listing and cannot communicate the real Agent/Sandbox owner of a host-managed Provider facade. The Marketplace also offers provider creation, duplicating the user-facing Provider picker. Catalog browsing fetches only 100 Tools and search 30 before client pagination; Profile selection can silently omit matching Tools beyond the first bounded slice.

## Decision

1. **One product entry, two focused views.** The Workspace sidebar has one **Tools** route, `/tools`, with mutually exclusive **Tool catalog** (default) and **Sources** views selected by an explicit `view=catalog|sources` URL dimension. Each view occupies the full viewport; existing list/filter/view/edit interactions remain focused in dialogs. The former `/catalog` and `/providers` page routes retire instead of becoming compatibility bridges. The Marketplace links directly to the Sources view with exact create/offering/installation intent. Agent/Sandbox owned facades remain discoverable at their *owner* and in catalog entries, never independent Sources.
2. **Do not merge persistence or implementation identity.** Keep `catalog.Provider`, `catalog.Tool`, `provider_configs`, `ProviderID`, `ToolID`, `(ProviderID,SourceKey)`, native package installations, tool synchronization, invocation and Policy/Profile governance unchanged. One plugin installation can back many configured Providers. No `sources` table, alternate inventory or Agent loop.
3. **Source attribution is an inert Host projection.** Resolve a Tool's *display owner* from its exact ProviderID through the existing `agent_tool_facades`, `sandbox_tool_suite_bindings` and `sandbox_job_suite_bindings` relations, joined with the Agent/Sandbox resource name. A normal independently configured Provider shows its own display name. An internal Provider lacking a proven owner relation is marked **unclaimed**, not guessed by PluginID, namespace or the target config. Never mutate or repair bindings on list queries. The existing technical ProviderID remains available for diagnostics and invocation but is not presented as an extra installable software item.
4. **Tool catalog is complete, Profile exposure stays separate.** Browse supports opaque continuation cursors and a reliable end marker in HTTP/OpenAPI/TS SDK/Wails; Provider filter is applied by the backend. Lexical search supports the same bounded continuation (stable BM25 order and ToolID tie-break), not client slicing of only the first 30/100. Cursor values are scoped to query/provider/profile and are validated/fail closed. Catalog can render many pages without loading full Tool schemas. Profile Tool Picker uses the same source-filtered, paged discovery contract, retains already-bound ToolIDs, and does not automatically grant a Profile.
5. **Status and ownership are different dimensions.** The Catalog may display source ownership and verified Profile binding information; it must not claim reachability or execution authorization merely because a Tool is synchronized. Catalog browsing never probes upstream endpoints, activates plugins or resolves credentials. The governance chain remains Principal/Profile ToolBinding → Policy/Approval → Resource grants/lease/budget → Executor → audit. No cross-agent/Sandbox mixing.
6. **Product intent, not leaked implementation.** Marketplace = discover/install/connect; Sources = independently configurable tool connections; Tool catalog = all canonical Tools, including Agent/Sandbox suites; Profiles = explicit tool supply. Internal Tool Suite facades and exact Plugin installations are not independent Sources. Keep existing localized terms consistent, no new permanent route aliases, dual-read logic or client-side shadow source registry.

## Invariants / acceptance

- One sidebar Tools item and two mutually exclusive views; all market links, overview/diagnostic links, create forms, tests and translations use the new route. Unsupported previous paths no longer mount a duplicate management page.
- MCP, OpenAPI, Model Tools and approved AnySearch create focused Sources; no generic internal Provider create/edit/sync/delete bypass. Plugin installation lifecycle remains on Marketplace Installed.
- Agent/Sandbox Tool entries show correct resource owner from persisted bindings; internal orphan/unclaimed Providers are explicit. Canonical IDs and existing Profile assignments never change.
- Paginate over >100 Tools for browse and search, including Provider-scoped, Profile picker and zero/invalid cursor cases; tie-break deterministically. No phantom availability claims or side effects on read.
- HTTP, CLI as relevant, SDK, Desktop Wails bindings and shared React UI agree. All Go/TS tests, generation drift, i18n, lints, build and security gates pass, with tracked worktree clean.

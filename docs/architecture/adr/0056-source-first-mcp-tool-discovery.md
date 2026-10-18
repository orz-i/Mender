# ADR-0056 — Profile-scoped source-first MCP Tool discovery

- Status: accepted (2026-10-18)
- Extends [ADR-0008](0008-mcp-adapter.md), [ADR-0046](0046-unified-tools-workspace.md) and [ADR-0055](0055-managed-mcp-profile-hot-refresh.md).

## Problem

`mender_search_tools({"query":"agent"})` currently returns 12 indistinguishable `delegate/status/result/cancel` entries from three distinct Agents. Searches for the actual Agent names (OpenCode, Grok - executor, Antigravity) or Profile-only aliases (Rocky SSH) return no tools: Catalog FTS5 indexes canonical Tool fields, while effective Profile presentation and verified resource attribution live elsewhere. Canonical descriptions can also generate noisy lexical matches. A flat list repeats descriptions, hides authority boundaries and encourages choosing a ToolID by rank.

## Decision

1. **Source is a read-only projection, not a new domain aggregate.** One source is **one configured Catalog Provider**; `sourceId = providerId`. Preserve ProviderID, ToolID, SourceKey, underlying AgentID/SandboxID, Profile ToolBindings and Invocation. Two configured Agents using the same backend remain separate Providers. Multiple tool suites on one Sandbox can each retain a distinct ProviderID. Never collapse by resource name/backend. The MCP source identity agrees with `/tools/:providerId`.
2. **Verified owner only.** The host composes sources using the existing `ListToolSourceOwners` exact facade bindings, provider configuration and `ToolSourceForProvider`. A claimed Agent/Sandbox has `sourceKind = agent|sandbox`, canonical resource ID and current resource display name; an independent configured provider has `sourceKind = provider` and Provider display name. An unclaimed facade, missing config, conflict or broken attribution is labelled `unclaimed`, with a neutral name and stable ProviderID; **never infer** ownership from namespace, plugin type or tool description. A conflicting relationship fails closed instead of silently choosing an owner. Optionally expose provider display name as a secondary distinction for two suites belonging to one resource.
3. **One meta-search entry, two views.** Keep `mender_search_tools`; hard-cut its existing flat response, do not add parallel `list_sources`/legacy endpoints. Input: `{view?: "sources"|"tools", query?: string, sourceId?: string, limit?: 1..100, cursor?: string}`. Default view is `sources`. `sources` returns compact source headers and Profile-visible counts; `tools` with `sourceId` returns the Provider's visible tools; `tools` with `query` and no source returns matches **grouped by source**. Empty `query` is allowed for browsing sources or tools within one source, but unscoped tool browsing requires a query. Unknown/invisible `sourceId` returns no source metadata. Invalid view, cursor and IDs return validation errors, not wider search.
4. **Profile restriction first.** Build discovery only from ToolIDs actually bound in the *current* Profile, joining each Tool to its ProviderID and effective ToolBinding alias/title/description. Calculate source counts after restriction; do not return undisclosed sources, real owner inventory, raw provider configs or other Profile tools. `sourceId` filtering cannot widen access. Source matching considers verified current source name, kind (e.g. `agent`, `workspace`), optional Provider display name and effective tool fields. Canonical tool fields/tags remain searchable. Prefer exact source name, source prefix/name match, alias/title and canonical name/title ahead of description-only hits; deterministic sourceId/ToolID tie breakers. Apply limits **after** Profile scoping and ranking, never to an unfiltered FTS result. Treat long generic descriptions as low signal.
5. **Small responses, on-demand detail.** Source view includes `sourceId`, `sourceKind`, `sourceName`, optional `resourceId`/secondary `providerName`, and `toolCount`; no per-tool Schema or repeated description. Tool view includes concise `toolId`, effective `name`/`title` and optional short summary, with the source metadata once per group. `mender_describe_tool(toolId)` retains full Schema and `mender_call_tool(toolId,...)` retains canonical invocation. Direct/hybrid `tools/list` stays standard flat MCP: preserve exported tool name and Schema, but qualify its human-readable title/description with the verified source identity so duplicate names are distinguishable. Never put source text in executable IDs.
6. **Pagination and freshness.** Bound responses to `limit <= 100` and use one opaque `nextCursor`, not arbitrary unbounded slices. Cursor binds view, normalized query, sourceId, limit, sorted Profile-visible ToolIDs and effective presentations, Catalog revision, and source names/owners; stale or cross-scope cursors fail closed. Order is deterministic. Source projection is read-only and must revalidate rename/deletion on subsequent discovery (not a long-lived name cache). Managed HTTP Profile changes use the existing ADR-0055 refresh fence; individual HTTP requests observe one coherent Profile snapshot. Concurrent reads must not silently mix revisions. No new source database, background worker or poller.
7. **Authorization does not change.** Search/describe do not activate providers or grant execution. Direct call and meta-call continue through Profile, Policy, Approval, Invocation, resource grants and Audit. A ToolID chosen before revoke cannot execute afterward. Never broaden YOLO/Agent trust based on discovery results.
8. **Product boundary.** UI already has `/tools` as configured services and `/tools/:providerId` as their tools. MCP discovery should mirror that grouping; no additional source-management UI and no new user-facing Provider/MCP/ACP category.

## Response examples (schematic)

```json
{"sources":[{"sourceId":"prov_...","sourceKind":"agent","sourceName":"OpenCode","toolCount":4}],"nextCursor":""}
```

```json
{"source":{"sourceId":"prov_...","sourceKind":"agent","sourceName":"OpenCode","toolCount":4},"tools":[{"toolId":"tool_...","name":"delegate","title":"Delegate to Agent"}],"nextCursor":""}
```

```json
{"groups":[{"source":{"sourceId":"prov_...","sourceKind":"agent","sourceName":"OpenCode","toolCount":4},"tools":[{"toolId":"tool_...","name":"delegate","title":"Delegate to Agent"}]}],"nextCursor":""}
```

`toolCount` counts *all supplied tools* within that source; groups only include query-matched tools. `rank` is not identity or required for invocation.

## Integration and verification

- Source resolver belongs at the bootstrap boundary because Catalog/MCP must not join Agent/Sandbox/config tables or parse plugin-specific JSON. MCP receives a protocol-independent read-only source lookup and retains Profile filtering itself.
- Catalog FTS5 is still useful for global administrative search and weighted canonical ranking, but cannot become an authoritative owner index: Agent/Sandbox names and Profile bindings are mutable, differently scoped data. Source name and per-Profile alias matching happens on the scoped discovery projection.
- Test real owner lookups vs unclaimed; multiple agents sharing backend; two same-name suites; source/group/browse/alias/canonical queries; >100 items; Profile hide/revoke; rename/delete; cursor scope/staleness; hot refresh; standard direct `tools/list`; duplicate tool invocation routing.
- Hard-cut the old `{"tools":[...],"rank":...}` search response and update callers/tests/docs together. No compatibility flags. No changes to persistent Profile grants or the operational MCP listener.

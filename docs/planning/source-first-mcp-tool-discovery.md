# Source-first MCP discovery — implementation and acceptance

Based on [ADR-0056](../architecture/adr/0056-source-first-mcp-tool-discovery.md). One focused implementation, no new Phase tree.

## A — contract (documentation commit)

- Freeze ProviderID as `sourceId`, verified exact ownership, source-first default, compact grouped search, opaque scoped cursor, Profile-first filtering and direct-tool presentation.
- Record old flat Meta Search response as deliberately retired; retain Describe/Call signatures.

## B — host projection (coordinator)

- Add bootstrap read-only resolver of `ProviderID -> sourceKind/sourceName/resourceId/providerName`, reuse existing exact owner/configuration query, and supply to MCP Adapter through a typed dependency.
- No new storage, duplicate source tables, activation, calls, or authorization shortcuts.
- Verify managed orphan/ambiguous bindings stay unclaimed/fail closed.

## C — search and pagination (OpenCode)

- Improve canonical FTS weighting/test quality in Catalog without treating provider namespace/plugin ID as owner.
- Add regression tests for lexical relevance, allowed-ID filtering, Provider restriction, >100 results, deterministic ordering and bad cursors.
- Independently verify query/pagination backward compatibility for non-MCP Catalog API.

## D — MCP source-first discovery (Antigravity)

- Implement the `view=sources|tools` Meta Search contract and grouped compact response, current source identity + effective alias/title/description matching, Profile allowlist before projection, cursor scope/staleness and tests.
- Qualify direct/hybrid exported human-readable metadata; keep names/Schema/ToolID and invocation untouched.
- Coordinate dependency interface with B; no edits to Catalog SQLite files.

## E — coordinator integration & evidence

- Inspect every diff and commit, run package tests then `pnpm check` including race detector.
- Non-destructive real tests: search OpenCode/Grok/Antigravity/Rocky/AnySearch and generic agent/workspace/read; same-backend ownership; Profile revoke/rename/hot refresh; describe/call correctly route ToolID. Do not restart Wails/Tunnel or mutate production without an explicit, narrowly scoped acceptance need.
- Preserve .agents/, .pi/, .intent-loop/, .commandcode/ and unrelated untracked files; segmented commits: docs, source projection, Catalog, MCP, integration/acceptance.

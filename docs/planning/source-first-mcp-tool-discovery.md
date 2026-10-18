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

## Deferred finding — unknown Agent outcome after delegation timeout

- During this implementation the OpenCode and Antigravity delegated tasks changed from `running` to `unknown` with diagnostic `Agent outcome unknown after timeout`. A subsequent process snapshot no longer included their task-specific CLI processes, but neither the stored status nor a PID snapshot proves whether each task succeeded, failed, or was still executing in a detached subprocess. Actual repository edits survived.
- **Do not conflate timeout, process disappearance, cancellation, and final result.** Preserve the ambiguous `unknown` state rather than reporting success/failure or automatically resubmitting potentially non-idempotent work.
- Follow-up Agent lifecycle investigation (separate from this discovery change): record exact task/process/session ownership and last heartbeat, inspect whether backend adapters support reliable reconnection, reconcile process exit against durable task result, distinguish a live detached session from an irrecoverable outcome, and ensure generation fencing prevents stale writers after retry. Add crash/network-timeout recovery tests for OpenCode and Antigravity independently.
- This issue is **recorded, not fixed** here. Do not change existing permissions, approval, timeout or process orchestration behavior in the Source-first discovery work.

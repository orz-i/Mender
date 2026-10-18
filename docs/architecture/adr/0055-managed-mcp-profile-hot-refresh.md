# ADR-0055 — Live Profile revisions for running managed MCP HTTP

- Status: accepted (2026-10-18)
- Extends [ADR-0008](0008-mcp-adapter.md) (MCP anti-corruption layer, stateless Streamable HTTP).

## Problem and evidence

A running `StartMCPHTTPServer` creates an `mcpserver.Adapter` once from a `Profile` aggregate. Its SDK direct-tool registry and its `mender_search_tools` / `mender_describe_tool` allowlist then retain that snapshot. The authoritative Invocation layer already rereads Profile bindings, so a revoked tool cannot execute, but MCP discovery incorrectly remains stale. A real Antigravity integration required manually restarting the managed server *and its Tunnel* after CLI-based `profile add-tool` operations. Mutations through desktop/CLI/HTTP or a second process must have the same behavior.

## Decision

1. Keep the **existing MCP stateless HTTP listener, endpoint, server ID, associated Tunnel process, and Profile IDs**. On **each incoming managed MCP HTTP request**, read the *persisted aggregate revision* through a bounded, read-only Profile application query. This revision is already atomically incremented by `SaveProfile` under SQLite compare-and-swap. Do not introduce a second table, cache invalidation bus, watcher process or timer.
2. If the revision matches, reuse the immutable current HTTP/SDK Adapter. On a changed revision, serialize refresh attempts, reread the full Profile by its pinned ProfileID, revalidate original Profile identity, re-admit/activate its providers through the ordinary trusted Profile activation path, and build an entirely new MCP Adapter (direct/dynamic/hybrid projections) before publishing its HTTP handler atomically. The SDK server is swapped **only for subsequent stateless requests**. Requests that already captured the old handler may finish; the Invocation boundary continues to reread current membership and reject revoked ToolIDs.
3. On an unavailable Profile, failed revision read, identity mismatch or a failed refresh, **do not serve stale discovery or direct-tool metadata**. Return a generic HTTP `503` and allow the next request to retry. Never expose raw Profile, Tool or credentials in the diagnostic. Do not increment revision or repair malformed persisted bindings implicitly. No database mutation or policy broadening is performed by refresh itself beyond the existing explicitly governed provider activation path.
4. This behavior belongs to **running managed Streamable HTTP servers** only. `NewMCPServer` used as a standalone snapshot (including stdio clients) keeps its explicit lifecycle: STDIO MCP cannot be silently reattached to a different protocol session. The Go MCP SDK v1.8.0 stateless HTTP mode does not support unsolicited `notifications/tools/list_changed`; changes become visible on the *next client request*. The client may still need to re-query tools (or re-initiate search) if its own UI caches tool descriptions.
5. Use a revision-only query on the Profile application/read port for the steady-state hot path; hydrate full Profile/tool schemas only upon a revision change. Do not watch filesystem timestamps, process restarts, or SDK internals. The Profile aggregate stays the single source of truth, and authorization remains enforced by the Invocation layer at execution time.

## Acceptance criteria

- Live Loopback MCP `tools/list` changes after `AddTool`, `ReplaceTools`, and `RemoveTool` in **direct and hybrid** mode without changing server ID/listener address or starting a second Tunnel process.
- Dynamic `mender_search_tools` and `mender_describe_tool` reflect the updated Profile after new/removed bindings, including a mutation committed through an independent SQLite connection. Removed direct tools cannot execute via prior tool names; removed dynamic ToolIDs are rejected by the existing Invocation fence.
- No refresh on unchanged revisions; concurrent traffic never publishes an older revision after a newer one; in-flight old requests cannot accidentally authorize a revoked ToolID; an unavailable Profile/rebuild failure returns `503` rather than stale results.
- No new admin API, lifecycle state, toggle, compatibility bridge, or automatic Profile grant; existing Wails/CLI contracts and profile Tool membership remain unchanged.
- Deterministic Go tests and Race Detector, then full `pnpm check`. Production acceptance uses a disposable, benign Tool binding (if available), restores it to its previous state, and proves same live ServerID/endpoint/Tunnel PID while the MCP search projection changes, without restarting production services.

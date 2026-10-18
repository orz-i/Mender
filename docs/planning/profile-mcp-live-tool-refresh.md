# Live managed MCP Profile hot refresh — implementation and acceptance

Date: 2026-10-18. Authority: [ADR-0055](../architecture/adr/0055-managed-mcp-profile-hot-refresh.md).

## Intent

Changing a Profile's Tool bindings must update an already-running Mender-managed MCP HTTP server without manually restarting the server or connected Tunnel. Changes from Desktop, owner CLI, HTTP or another process must converge on the next request. Preserve the immutable Profile boundary and authoritative Invocation permission checks, and Mender's role as an Agent tool/resource supplier (not an Agent Loop). No new compatibility code or data migration.

## Executable clusters and gates

| Cluster | Change surface | Check |
|---|---|---|
| Revision query | Profile application read port and SQLite query | no mutation, exact committed revision and not-found error, independent connection visibility |
| Request-time refresh | managed MCP HTTP boundary and atomic projection publication | direct/dynamic/hybrid revisions; no listener/Tunnel restart; 503 on failed refresh |
| Safety / concurrency | deterministic isolated fixtures | revocation cannot execute old Tool, concurrent requests monotonic, race detector |
| Integration | native live request + validation and staged commits | authoritative `pnpm check`; controlled production refresh without downtime if safe |

## Constraints

- A stateless MCP client must initiate another request; `notifications/tools/list_changed` cannot be pushed by the current stateless transport.
- Existing `NewMCPServer` standalone snapshots and stdio transport contracts are not silently changed.
- Treat cross-process SQLite writes as primary, not in-process callbacks. No hidden periodic pollers or automatic Tunnel teardown.
- All production identifiers/credentials and historic task/grant records are preserved. The four existing untracked skill/experiment directories remain untouched.

## Evidence

Pending implementation and tests.

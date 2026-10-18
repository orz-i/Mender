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

## Completion evidence (2026-10-18)

- **Staged commits:** `665370c` documents the failure mode, decisions and checks; `7b16a1a` adds a read-only, committed Profile-revision application port backed by a single SQLite indexed query; `7e7b533` adds request-time, atomic managed MCP HTTP projection replacement and deterministic real-HTTP tests. The standard SDK `NewMCPServer` and STDIO lifecycle remain snapshots and were not changed.
- **No listener or tunnel restart:** the managed HTTP handler checks Profile revisions for each incoming request and publishes fully built new SDK adapters only on newer revisions. In `TestManagedMCPHTTPProfileChangesDoNotRestartTunnel`, a real local HTTP MCP session sees a new Profile ToolBinding, while the same ServerID, listen endpoint, start timestamp and **single active fake Tunnel process** are retained. Inflight calls continue on their captured SDK projection; Invocation continues to recheck live Profile membership before execution.
- **Actual protocol integration:** in `TestManagedMCPHTTPProfileMembershipRefreshDirectAndHybrid`, the **same connected MCP client session** sees new direct Tools after an independent SQLite repository commits them, sees a Tool rename and metadata override after `ReplaceTools`, and no longer sees or can call removed direct Tools. In `TestManagedMCPHTTPDynamicMetaToolsReflectUpdatedMembership`, `mender_search_tools` and `mender_describe_tool` honor newly added and revoked membership; revoked dynamic `mender_call_tool` cannot reach upstream. Tests run with genuine in-process upstream MCP over Streamable HTTP and the Mender-managed listener, not mocks of successful discovery.
- **Fail-closed publication and concurrency:** request revision-query failure or incomplete/invalid projection rebuild returns generic HTTP 503 rather than serving stale results or revealing sensitive error strings. Identity replacement, revision rollback and an actual concurrent revision update during rebuild do not publish an obsolete snapshot. Concurrent requests serialize builds and never regress the atomic published revision. `TestProfileRevisionReadObservesOtherConnectionsWithoutMutation` proves the committed revision reader sees another SQLite connection and does not modify Profile state.
- **Validation:** focused Go tests plus five repeated focused `go test -race` rounds passed; Go lint **0 issues**. Full canonical `pnpm check` exited **0** (`/tmp/mender-profile-refresh-fullcheck.exit`), including full backend unit tests and Race Detector, all plugin SDKs/examples, generators, Wails bindings/architecture, TypeScript/ESLint, i18n **1350/1350**, Vitest **46 files / 279 passed**, and Web/Desktop production builds. Only the known nonblocking Vite chunk-size warning remains. Independent native macOS `go build` exited **0**; its private QA binary was removed after size verification (linker printed 19 nonblocking object deployment-target warnings).
- **Production preservation:** SQLite was inspected **read-only** (no production mutation this phase): Profile revision **5**, **29** ToolBindings, **4** existing Agents, **53** historic Agent Tasks, Agent schema v13, `PRAGMA quick_check=ok`, no foreign-key violations. Untracked `.agents/`, `.commandcode/`, `.intent-loop/`, `.pi/` were untouched and no branch push occurred.
- **Deployment boundary:** the currently running Wails Desktop executable was last built at Mac-local **17:01:20 (+08:00)**, before this source change. Thus the live product has **not yet been verified** to run the hot-refresh code. The owner must rebuild/reconnect normally once, then subsequent Profile Tool changes will hot-refresh on the next MCP request without another server/Tunnel restart. A ChatGPT client that caches old tool descriptions may need to issue another `mender_search_tools` call; the stateless MCP transport cannot deliver unsolicited `notifications/tools/list_changed`. Do not claim automatic client-UI discovery without that request.

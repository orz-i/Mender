# MCP Server Management

Status: Complete

## Objective

Expose Mender Profiles to external Agent/LLM clients from the Desktop management application without requiring a separate terminal command.

The product flow becomes:

```text
Providers -> Catalog -> Profiles -> Servers -> external MCP clients
```

## Page responsibility

Add a dedicated `Servers` page.

The page owns only running downstream MCP server instances and server lifecycle actions. Profile membership remains on Profiles.

Primary UI:

- running server table,
- `Start server` action,
- Stop,
- Restart,
- Copy endpoint,
- Copy client configuration.

The Start dialog chooses:

- Profile,
- loopback bind address.

The first managed transport is Streamable HTTP.

## Runtime ownership

Managed servers belong to the existing `bootstrap.Runtime`.

The Runtime:

1. loads the requested Profile,
2. activates Providers referenced by that Profile,
3. builds the existing Profile-scoped MCP server adapter,
4. binds a loopback HTTP listener,
5. exposes the adapter at `/mcp`,
6. tracks the running instance,
7. shuts it down on Stop or Runtime.Close.

No second invocation pipeline or MCP implementation is introduced.

## Security

Desktop-managed HTTP MCP servers are loopback-only.

Accepted bind hosts:

- localhost,
- loopback IPv4,
- loopback IPv6.

Wildcard and non-loopback addresses are rejected.

Remote MCP serving requires a separate authentication/authorization design and is not enabled by this work.

## Persistence

Server instances are runtime state and are not persisted.

Mender does not auto-start a server after application restart in this iteration. Persistent/autostart server configuration requires an explicit lifecycle and security design.

## stdio

Desktop does not model stdio as a Start/Stop server instance.

stdio MCP transport binds server lifetime to the external client's stdin/stdout process. The existing supported flow remains:

```bash
mender serve mcp --profile <slug> --stdio
```

A future Desktop distribution may provide a copyable stdio client configuration when the release package guarantees an addressable CLI executable.

## Management contract

Add host-neutral client operations:

- list running MCP servers,
- start Streamable HTTP server,
- stop server,
- restart server.

Both loopback Management HTTP and Wails bindings expose the same Runtime operations.

## Closeout

Completed on 2026-09-26.

Validated behavior:

- Runtime-managed Streamable HTTP servers bind only to loopback addresses and expose MCP at `/mcp`.
- Management HTTP and Wails both support list/start/restart/stop through the shared Runtime manager.
- A real MCP client connected to a managed endpoint and observed the Profile-exported Tool.
- Stop/Restart force-close lingering downstream connections after the graceful shutdown timeout, and the race suite covers this lifecycle.
- Downstream endpoints reject non-local Host/Origin and explicit cross-site browser provenance.
- Servers are ephemeral and are closed by `Runtime.Close`; no SQLite/autostart state was added.
- The shared Servers page exposes runtime status, endpoint/config copy, Restart, and Stop.
- Profiles with zero Tool membership are called out in the Start dialog to avoid a misleading “running but no Provider Tools” state.
- `pnpm check` passed with OpenAPI/Wails regeneration, Go lint/tests/race, ESLint, strict TypeScript, 21 Vitest tests, and Web/Desktop production builds.
- macOS `wails3 package` completed successfully with the local ad-hoc signature.

## Exit criteria

- [x] Servers page exists as a dedicated route.
- [x] Start dialog selects Profile and loopback address.
- [x] Running server list exposes endpoint and status.
- [x] Stop and Restart work without restarting the Desktop app.
- [x] Client configuration can be copied for a running server.
- [x] Runtime closes all managed servers during shutdown.
- [x] Repeated Server starts reuse already-active Provider executors.
- [x] HTTP management and Wails clients share one contract.
- [x] Non-loopback binds are rejected.
- [x] Downstream MCP HTTP requests reject non-local Host/Origin and cross-site browser provenance.
- [x] Full `pnpm check` passes.
- [x] macOS Desktop native package smoke succeeds.

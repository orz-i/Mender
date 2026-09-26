# ADR-0016: Manage downstream MCP HTTP servers as ephemeral Runtime-owned instances

- Status: Accepted
- Date: 2026-09-26

## Context

Mender could already expose one Profile through the CLI:

```bash
mender serve mcp --profile default --http 127.0.0.1:8765
mender serve mcp --profile default --stdio
```

The Desktop application had no corresponding lifecycle control. Users could configure Providers, discover Tools, and build Profiles but could not complete the final product flow from the application:

```text
Providers -> Catalog -> Profiles -> downstream MCP server -> external Agent/LLM client
```

The CLI HTTP serve implementation is foreground/blocking and owns the process lifetime. Desktop requires servers that can be started, listed, restarted, and stopped while the main Runtime remains alive.

stdio has different lifetime semantics: the MCP client launches a process and communicates over that process's stdin/stdout. A persistent GUI process cannot expose a meaningful detached stdio endpoint merely by toggling a Start button.

## Decision

Mender's shared `bootstrap.Runtime` owns managed downstream MCP server instances.

A managed server:

1. loads one persisted Profile,
2. activates Providers referenced by that Profile,
3. constructs the existing Profile-scoped MCP adapter,
4. binds a loopback TCP listener,
5. exposes Streamable HTTP at `/mcp`,
6. remains active until explicit Stop/Restart or Runtime shutdown.

Managed servers are **ephemeral runtime state**. They are not persisted to SQLite and are not automatically restarted when Mender launches.

Management HTTP and Wails bindings expose the same lifecycle operations:

- list,
- start,
- restart,
- stop.

The shared React application consumes those operations only through `MenderClient`.

Desktop-managed serving supports Streamable HTTP only. stdio remains client-launched through the existing CLI command.

## Security boundary

Managed HTTP servers accept only:

- `localhost`,
- loopback IPv4,
- loopback IPv6.

Wildcard and non-loopback bind addresses are rejected.

Loopback binding is reinforced with request provenance validation. The downstream endpoint rejects:

- non-local `Host`,
- non-local browser `Origin`,
- explicit `Sec-Fetch-Site: cross-site`.

This reduces hostile browser/DNS-rebinding exposure to local Tool execution.

Remote downstream MCP serving remains disabled until an explicit authentication/authorization design exists.

## Shutdown behavior

Runtime shutdown stops all managed servers before closing Provider connections and persistence stores.

Explicit Stop/Restart first attempts graceful HTTP shutdown. Because Streamable HTTP clients may keep long-lived connections open, Mender force-closes the HTTP server if graceful shutdown reaches its timeout. A successful forced close is considered a successful Stop.

Running Provider executors may remain Runtime-owned after a downstream server stops; they are shared infrastructure and are released when Runtime closes or are replaced by later Provider activation.

## Alternatives considered

### Spawn `mender serve mcp` as a child process from Desktop

Rejected because it would introduce another Runtime/database owner, process supervision, duplicated Provider connections, and child-process lifecycle complexity.

### Implement server lifecycle only in Wails

Rejected because Web management and Desktop would acquire different application semantics. Server lifecycle belongs to the shared Runtime; Wails is only a transport adapter.

### Persist server definitions and auto-start them

Deferred. Auto-start changes application startup, port ownership, security expectations, and recovery behavior. Runtime-only instances are explicit and reversible.

### Treat stdio as another toggleable Desktop server

Rejected because stdio is tied to the client-launched process streams rather than a discoverable network endpoint.

## Consequences

Positive:

- Desktop can complete the full Provider-to-client workflow without a terminal,
- HTTP and Wails management share one implementation,
- MCP serving still uses the existing Profile adapter and Invocation pipeline,
- managed server shutdown is part of Runtime lifecycle,
- no new persistence schema is required.

Trade-offs:

- servers must be started again after Mender restarts,
- Profile changes require server Restart before the running adapter sees the new Profile snapshot,
- stdio client configuration still depends on the CLI distribution,
- remote serving remains unavailable.

## Follow-up

A future persistence/autostart design may add saved server definitions only with explicit startup and security semantics.

A future distribution improvement may expose copyable stdio client configuration when the installed package guarantees a stable CLI executable path.

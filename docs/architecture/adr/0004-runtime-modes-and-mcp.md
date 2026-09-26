# ADR-0004: Support local and remote modes with MCP as an adapter

- Status: Accepted
- Date: 2026-09-25

## Context

Mender must work for local developer workflows and as a remotely deployed tool gateway.

Local users need direct CLI execution without requiring an HTTP service. Protocol clients still need local MCP compatibility. Remote users need MCP over the current HTTP transport.

Treating either local or remote deployment as the primary architecture would create unnecessary constraints for the other.

## Decision

Mender will support two runtime modes over the same application core.

### Local mode

Primary entry points:

- Direct CLI commands.
- MCP over stdio where protocol access is required.
- Agent Skills that use the Mender CLI/runtime.

Local execution must not require a running web server.

### Remote mode

The daemon exposes selected profiles over MCP Streamable HTTP.

The intended route model is compatible with endpoints such as:

```text
/mcp
/mcp/{profile}
```

The management API is separate from MCP protocol endpoints.

MCP is an inbound and outbound adapter, not the internal domain model.

Legacy HTTP+SSE will not be the primary transport for new implementation. Compatibility support, if added, must be isolated in the adapter layer.

Profiles act as both exposure views and security boundaries.

## Alternatives considered

### Always run a local daemon and make CLI an HTTP client

This simplifies one transport path but adds lifecycle and availability requirements to basic local commands.

### Make MCP the only local interface

This forces human and shell automation through a protocol designed primarily for model/tool interoperability.

### Separate local and remote runtimes

This would duplicate execution, policy, credential, and audit behavior and create drift.

## Consequences

### Positive

- CLI remains fast and composable.
- Remote deployment uses the same use cases as local execution.
- Protocol-specific concerns remain at boundaries.
- Profiles provide a natural remote routing and security unit.

### Negative / trade-offs

- Multiple composition roots must remain consistent.
- Some infrastructure implementations differ between local and remote deployments.
- Testing must cover both direct and protocol-mediated invocation paths.

## Follow-up

Initial implementation should prove one tool can be invoked directly by CLI and through an MCP endpoint while traversing the same application invocation pipeline.

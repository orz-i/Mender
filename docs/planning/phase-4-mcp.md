# Phase 4 Plan — MCP Provider and Server Adapters

## Objective

Connect Mender's protocol-independent Catalog and Invocation runtime to the Model Context Protocol without allowing MCP SDK types to leak into Domain or Application packages.

Phase 4 implements both directions:

- MCP as an upstream Provider source and Tool executor.
- Mender as an MCP server exposing a controlled Profile.

The adapter targets the official Go MCP SDK v1.8.0 and protocol revision 2026-07-28 while retaining SDK-supported compatibility with older peers.

## Protocol baseline

Mender uses the official Go SDK package:

```text
github.com/modelcontextprotocol/go-sdk/mcp
```

Phase 4 pins SDK v1.8.0.

For remote serving, Streamable HTTP is configured stateless so 2026-07-28 can be negotiated. Legacy HTTP+SSE is not implemented as a new Mender server surface.

For local serving, stdio remains supported.

The SDK may negotiate older supported protocol revisions when talking to upstream peers. Protocol-version details remain inside the MCP adapter.

## Anti-Corruption Layer

MCP is an external protocol, not Mender's domain model.

Required mapping direction:

```text
MCP Tool
   |
   v
MCP mapper
   |
   v
Catalog DiscoveredTool / canonical Tool

Invocation ExecutionRequest
   |
   v
MCP executor
   |
   v
tools/call
   |
   v
MCP result mapper
   |
   v
Invocation Result
```

Forbidden:

```text
catalog/domain -> mcp.Tool
invocation/domain -> mcp.CallToolResult
profile/domain -> MCP protocol types
```

MCP-specific metadata that Mender does not yet model is intentionally not copied into Domain types merely for lossless transport.

## Upstream MCP Provider model

Phase 4 introduces adapter-owned connection configuration.

### Stdio

A local upstream Provider is configured with:

- Executable.
- argv.
- Optional working directory.
- Explicit environment additions.

It is executed with `exec.CommandContext` / SDK CommandTransport semantics. Configuration uses executable + argv, never an implicit shell command string.

### Streamable HTTP

A remote upstream Provider is configured with:

- Endpoint URL.
- Optional HTTP client.
- Future authorization hook.

The initial adapter does not invent a credential/OAuth storage model. SDK OAuth integration is deferred until a concrete user-facing credential configuration exists.

### Session lifecycle

An MCP Provider connection owns an SDK ClientSession.

One connection object implements both:

- Catalog ToolSource for synchronization.
- Invocation ToolExecutor for calls.

This guarantees list and call semantics use the same upstream server identity.

Connections are explicitly Close-able and must not be persisted inside Catalog Domain.

## MCP Tool mapping

### Identity

Catalog SourceKey for an MCP tool is the MCP tool name.

This is the strongest portable tool identity exposed by MCP core today. Therefore an upstream MCP tool rename is a Catalog identity change.

Mender ToolID remains independent and stable as long as SourceKey is unchanged.

### Metadata

Mapping:

```text
MCP name          -> SourceKey + Tool.Name
MCP title         -> Tool.Title
annotations.title -> fallback Tool.Title
description       -> Tool.Description
inputSchema       -> Tool.InputSchema
outputSchema      -> Tool.OutputSchema
```

MCP annotations are hints and are not treated as Mender security policy.

Input and output schemas are JSON-marshaled at the adapter boundary and validated by the canonical Catalog Schema type.

## MCP invocation result mapping

### Complete

For successful MCP tool calls:

1. Prefer `structuredContent` when present.
2. Otherwise normalize MCP content blocks into protocol-independent JSON.
3. Preserve JSON value semantics without injecting SDK types into Invocation.

An MCP `isError: true` response becomes an execution error, not a successful Complete result.

### InputRequired

MCP 2026-07-28 supports Multi Round-Trip Requests.

The Go SDK exposes input-required data on CallToolResult but its internal result discriminator is not exported. The adapter therefore identifies an input-required result when InputRequests is non-empty and maps the opaque RequestState plus a safe JSON description of requested inputs into Mender's InputRequired result.

Continuing an MRTR exchange is deferred until a dedicated continuation application API is introduced.

### Task

Mender's Invocation Domain already models Task.

The 2026-07-28 Tasks feature is an official MCP extension, but the official Go SDK v1.8.0 does not expose the Tasks extension in its core mcp package. Phase 4 therefore does not pretend to support upstream/downstream MCP Tasks.

A future adapter can add the official extension without changing Invocation Domain.

## Minimal Profile Domain

Phase 4 introduces the Profile aggregate required to enforce MCP exposure boundaries.

Profile persistence and user-facing CLI management are deferred to Phase 5.

A Profile contains:

- ProfileID.
- Stable slug.
- Display name.
- Exposure mode.
- Unique Tool memberships.
- Direct flag per membership for hybrid mode.

### Exposure modes

#### Direct

Every Tool in the Profile is exported as a first-class MCP tool.

#### Dynamic

No underlying Tool is exported directly.

The server exposes Mender meta-tools:

- `mender_search_tools`
- `mender_describe_tool`
- `mender_call_tool`

Search, describe, and call are strictly limited to Profile membership.

#### Hybrid

Tools marked Direct are exported as first-class MCP tools.

The same three meta-tools are also exposed and may discover/call any Tool in the Profile.

This gives small/common tools zero extra discovery round-trip while keeping a large tail lazily discoverable.

## Direct MCP export names

MCP export names must be deterministic, valid, and collision-safe inside one server.

Mender generates direct names from:

```text
<provider-namespace>__<canonical-tool-name>
```

Invalid MCP name characters are replaced with underscores.

If the resulting name exceeds the protocol limit, the adapter truncates the readable prefix and appends a stable short suffix derived from ToolID.

If a collision still occurs, server construction fails rather than silently shadowing a Tool.

Export names are adapter concerns and never replace ToolID.

## Profile-scoped discovery

Catalog Search gains an optional ToolID allow-list scope.

SQLite FTS continues to rank globally, but the application/infrastructure boundary filters returned matches to the allowed ToolIDs before satisfying the requested limit.

This allows Profile dynamic discovery to reuse Catalog search semantics without reading Profile persistence from Catalog.

## Mender MCP server

The server builder receives:

- A Profile.
- Catalog query capabilities.
- Shared InvokeTool pipeline.

It registers only the tools allowed by the Profile.

Every direct and meta-tool call invokes the existing shared Invocation pipeline. The adapter never calls an upstream executor directly.

### Result conversion

Mender Complete -> MCP complete CallToolResult.

Mender InputRequired is represented using MCP input-required semantics only where the SDK public API safely supports it. If the SDK cannot express a required wire detail through a stable public API, the adapter returns a clear unsupported error rather than using reflection or unsafe access to private SDK state.

Mender Task -> unsupported by the Phase 4 MCP adapter until the official Tasks extension is available in the Go SDK surface used by Mender.

## Server transports

### Stdio

`menderd`/future CLI can run an MCP Server over SDK StdioTransport.

### Streamable HTTP

Mender exposes an `http.Handler` built with StreamableHTTPOptions:

```text
Stateless: true
```

This is the required shape for protocol 2026-07-28 in Go SDK v1.8.0.

Authentication/authorization middleware belongs outside the protocol handler and is deferred until remote identity configuration is designed.

## Testing strategy

### Mapper tests

- MCP Tool -> DiscoveredTool.
- JSON Schema round-trip.
- annotations are not promoted to policy.
- result mapping.

### Upstream in-process tests

Use SDK in-memory transports to prove:

- list -> Catalog synchronization.
- call -> shared Invocation pipeline -> upstream MCP handler.

These tests validate adapter behavior without subprocess/network flakiness.

### Stdio client test

Use a test helper subprocess to prove CommandTransport configuration and cleanup.

### Streamable HTTP test

Use `httptest.Server` with stateless StreamableHTTPHandler to prove remote provider synchronization/call and Mender server exposure.

### Mender server tests

For each mode:

- Direct lists/calls only Profile tools.
- Dynamic lists only meta-tools.
- Hybrid lists direct tools plus meta-tools.
- Dynamic search cannot discover a Tool outside the Profile.
- Dynamic call cannot call a Tool outside the Profile.
- All actual calls create Audit through InvokeTool.

## Work packages and commits

1. Governance / Profile / Catalog search scope.

```text
docs: define MCP adapter and exposure architecture
feat(profile): add MCP exposure profile domain
feat(catalog): add scoped tool discovery
```

2. Upstream provider adapter.

```text
feat(mcp): add upstream tool mapping and connection
feat(mcp): add MCP tool executor
```

3. Mender MCP server adapter.

```text
feat(mcp): add profile-scoped MCP server
feat(mcp): add stdio and streamable HTTP serving
```

4. Composition / integration / closeout.

```text
build(mcp): wire MCP provider and server adapters
docs: complete MCP adapter phase
```

## Exit checklist

- [ ] Official Go MCP SDK v1.8.0 is pinned.
- [ ] MCP SDK types exist only in MCP infrastructure/adapter packages.
- [ ] Minimal Profile Domain supports direct, dynamic, and hybrid exposure.
- [ ] Catalog search can be restricted to a ToolID allow-list.
- [ ] MCP Tool schemas map into canonical Catalog schemas.
- [ ] Local stdio upstream MCP provider can synchronize and invoke.
- [ ] Remote Streamable HTTP upstream provider can synchronize and invoke.
- [ ] One upstream MCP Tool executes through Mender's shared Invocation pipeline.
- [ ] Direct server exposure lists/calls only Profile Tools.
- [ ] Dynamic server exposure lists only Mender meta-tools.
- [ ] Dynamic search/describe/call cannot escape Profile membership.
- [ ] Hybrid exposure combines selected direct tools with meta-tools.
- [ ] Mender Streamable HTTP server is stateless.
- [ ] Stdio server transport is available.
- [ ] MCP adapter integration tests pass.
- [ ] Domain packages have no MCP SDK dependency.
- [ ] `pnpm check` passes.

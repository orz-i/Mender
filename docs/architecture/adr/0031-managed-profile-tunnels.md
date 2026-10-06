# ADR-0031: Model external Profile reachability as managed Tunnel adapters

- Status: Accepted
- Date: 2026-10-01

> **Supersession note (2026-10-06):** ADR-0034 removes the persisted/configurable Tunnel executable and routes `tunnel-client` through Runtime-owned process governance. ADR-0031 remains authoritative for Tunnel domain, Profile binding, Credential, reachability, and managed MCP lifecycle semantics.

## Context

Mender can expose a Profile through a Runtime-owned loopback Streamable HTTP MCP server, but ADR-0016 intentionally rejects non-loopback binding. That preserves the local execution boundary, yet it means hosted LLM/agent products cannot directly reach a Profile on a developer machine or private network.

OpenAI now provides Secure MCP Tunnel. A customer-run `tunnel-client` keeps an outbound HTTPS path to OpenAI, receives MCP work, forwards it to a private MCP server, and returns responses through the same tunnel. The private server remains on localhost/private networking; no inbound firewall opening is required.

Secure MCP Tunnel is not a generic public reverse proxy. It makes a private MCP server reachable through supported OpenAI products and APIs. Future Mender integrations may include other tunnel providers with different reachability semantics.

Mender must keep its existing product boundary: it supplies governed Tools, Resources, Profiles, and runtime adapters. It does not become an LLM/Agent loop.

References:

- https://developers.openai.com/api/docs/guides/secure-mcp-tunnels
- https://developers.openai.com/api/docs/guides/tools-connectors-mcp
- https://github.com/openai/tunnel-client

## Decision

Mender will add a dedicated `Tunnel` bounded context and a Runtime-owned tunnel adapter layer.

A Tunnel is persistent non-secret configuration describing how a local Profile MCP endpoint may be reached through an external tunnel provider.

V1 supports one provider:

~~~text
openai
~~~

The initial OpenAI Tunnel configuration contains:

~~~text
Tunnel
  TunnelConfigID
  DisplayName
  Provider = openai
  ExternalTunnelID
  RuntimeCredentialRef
  Executable = tunnel-client
~~~

The OpenAI runtime API key is referenced through the existing Credential inventory. Secret material is resolved only when starting the child process and is injected through `CONTROL_PLANE_API_KEY`; it is never persisted in Tunnel rows, API projections, logs, or audit data.

### Profile owns only the binding

Profile persistence gains one optional `tunnel_id` reference.

Profile does not know how `tunnel-client` works. It only selects zero or one configured Tunnel. The Tunnel bounded context owns validation, provider semantics, process configuration, and lifecycle behavior.

A Profile without a Tunnel behaves exactly as today.

Deleting a Tunnel that is referenced by a Profile fails closed.

### Tunnel execution is coupled to the managed MCP server lifecycle

When Desktop/Web starts a managed MCP HTTP server for a Profile:

1. Mender loads the Profile.
2. Mender starts the existing loopback-only MCP HTTP server.
3. If the Profile has no Tunnel, startup completes as before.
4. If the Profile references an OpenAI Tunnel, Mender resolves its runtime Credential and starts `tunnel-client` with the server's loopback `/mcp` endpoint.
5. If tunnel startup fails synchronously, Mender stops the just-created MCP server and returns an error.
6. Stop, Restart, unexpected server exit, and Runtime.Close also stop the associated tunnel process.

The loopback HTTP security guard remains unchanged. Tunnel support does not permit wildcard or non-loopback MCP listening.

### OpenAI adapter

V1 launches the configured executable with equivalent arguments:

~~~text
tunnel-client run
  --control-plane.tunnel-id=<external tunnel id>
  --control-plane.api-key=env:CONTROL_PLANE_API_KEY
  --mcp.server-url=<managed loopback /mcp endpoint>
  --health.listen-addr=127.0.0.1:0
~~~

Mender supplies a minimal child environment containing the resolved `CONTROL_PLANE_API_KEY` plus the host environment required for normal process execution.

V1 treats successful process start as `running`. Deep readiness polling of `/readyz` is deferred; the process exit state is still supervised and surfaced on the managed Server projection.

The default executable is `tunnel-client`. An explicit executable path may be stored for development/packaging environments.

### Management contracts

Management HTTP, OpenAPI SDK, Wails bindings, and the shared React application expose:

- list Tunnels;
- create OpenAI Tunnel;
- delete Tunnel;
- assign/clear one Profile Tunnel;
- managed MCP Server Tunnel metadata/status.

Tunnel configuration APIs never return resolved secret material.

The new Tunnels page lives in the Runtime navigation group near Profiles and Servers.

### Credential governance

A Tunnel RuntimeCredentialRef must resolve to an existing Credential at creation time.

Credential usage checks include Tunnel references, so a Credential cannot be deleted while a Tunnel depends on it.

This is infrastructure authentication for the tunnel transport. It is distinct from Profile ToolBinding Credential scope used for Tool execution.

## Alternatives considered

### Bind the managed MCP server to a public/wildcard address

Rejected. It would bypass ADR-0016's loopback and request-provenance boundary and would require a separate public authentication/authorization design.

### Put OpenAI tunnel settings directly on Profile

Rejected. That would couple the Profile domain to one transport vendor and make future tunnel providers harder to add.

### Model Tunnel as a Provider plugin

Rejected. Provider plugins contribute canonical Tools upstream into Mender. Tunnels transport a downstream Profile MCP surface outward; the direction and lifecycle are different.

### Store the OpenAI runtime API key in Tunnel config

Rejected. Secret material belongs to the Credential subsystem and must only be resolved at the execution boundary.

### Use `tunnel-client runtimes connect` as Mender's process supervisor

Deferred. Mender already owns the managed MCP server lifecycle. V1 supervises the foreground `run` process directly so stop/restart/close semantics remain under one Runtime owner.

## Consequences

Positive:

- a local/private Profile can be used by supported OpenAI surfaces without weakening loopback binding;
- Tunnel provider semantics stay outside Profile and Provider domains;
- Credentials remain non-secret references at rest;
- server and tunnel lifecycles remain coordinated and reversible;
- the model can grow to additional tunnel providers later.

Trade-offs:

- OpenAI Secure MCP Tunnel is not a generic public URL and must be described accurately in UI/docs;
- `tunnel-client` must already be installed or an explicit executable path configured;
- V1 process-start status is weaker than full tunnel readiness;
- one Profile selects at most one Tunnel in V1.

## Follow-up

- Add readiness/health probing and richer diagnostics from the tunnel-client local admin endpoints.
- Consider packaging/distribution of the OpenAI tunnel-client if licensing and release constraints permit.
- Add additional Tunnel adapters only when their security and reachability semantics are explicit.

# ADR-0015: Persist HTTP header templates and resolve secrets at the Provider connection boundary

- Status: Accepted
- Date: 2026-09-26

## Context

Remote MCP Providers commonly require request headers, especially authorization headers. Existing Mender HTTP Provider configuration persisted only the endpoint, so configurations such as:

```json
{
  "mcpServers": {
    "anysearch": {
      "type": "streamable-http",
      "url": "https://api.anysearch.com/mcp",
      "headers": {
        "Authorization": "Bearer ${ANYSEARCH_API_KEY}"
      }
    }
  }
}
```

could not be represented.

Persisting the resolved token directly in SQLite would violate Mender's existing execution-boundary credential principle and would turn Provider configuration into a secret store before a real credential store exists.

Custom HTTP headers also create redirect risk: an HTTP client must not forward an authorization header to a different origin after a redirect.

## Decision

HTTP Provider configuration may persist header **templates**, not resolved secret values.

Examples:

- non-sensitive literal header: `X-Anysearch-Client: mcp/1.0.0`
- sensitive header template: `Authorization: Bearer ${ANYSEARCH_API_KEY}`

Supported environment-reference syntax is:

- `${NAME}`
- `${env:NAME}`
- `{env:NAME}`

Sensitive header names, including authorization/token/secret/password/API-key-like headers, must use an environment reference. Authorization headers may use a fixed authentication scheme plus exactly one environment reference. Plaintext sensitive values are rejected before persistence.

Templates are resolved only when Mender creates an upstream HTTP MCP connection. The resolved header map is never written back to SQLite or returned by the management API.

Transport-controlled headers such as `Host`, `Content-Length`, `Connection`, and transfer-encoding headers cannot be configured.

When custom headers are configured, Mender rejects HTTP redirects to a different origin. The header injection transport also independently refuses to attach configured headers to a request whose origin differs from the configured Provider endpoint.

MCP Provider configuration schema version 2 adds a `headers_json` column containing only the persisted templates.

The current implementation resolves environment references from the Mender process environment. This is an interim secret source, not a persistent credential store.

## Alternatives considered

### Persist resolved API keys in SQLite

Rejected. Mender does not currently provide encrypted secret storage, rotation, access controls, or a key-management model for such values.

### Allow arbitrary literal Authorization headers with a warning

Rejected because it makes accidental secret persistence easy and weakens the security boundary into a UI convention.

### Put headers only in frontend state

Rejected because synchronization and Tool invocation also happen after restart and through non-browser adapters. Provider connection configuration belongs to the backend adapter layer.

### Forward custom headers through redirects

Rejected because a Provider-controlled redirect could exfiltrate authorization material to another origin.

### Add a platform credential store in the same change

Deferred. A Keychain/credential-store implementation needs its own lifecycle, UX, migration, and cross-platform design. The template resolver provides a narrow integration point for such a future source.

## Consequences

Positive:

- authenticated Streamable HTTP MCP Providers can be represented,
- common MCP JSON configurations can be imported without storing API keys,
- synchronization and later Tool invocation use the same resolution path,
- secrets remain outside SQLite and management responses,
- cross-origin redirect credential leakage is blocked.

Trade-offs:

- the referenced environment variable must be available to the Mender process whenever a Provider connection is established,
- desktop applications launched outside the user's shell may not see shell-only environment variables,
- there is still no persistent credential-management UI or secret store.

## Follow-up

A future credential-store ADR may replace or supplement process-environment lookup while preserving the same connection-boundary resolution rule.

Management UI should make template-vs-secret semantics explicit and surface synchronization failures such as missing environment variables.

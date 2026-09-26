# ADR-0012: Harden local management and upstream Provider execution boundaries

- Status: Accepted
- Date: 2026-09-26

## Context

Mender's management HTTP listener is restricted to loopback, but listener binding alone is not the whole browser security boundary. A hostile web origin or DNS-rebinding flow can still target a service on localhost unless the service validates request provenance.

Mender also launches configured stdio MCP Providers as local processes. The current process launch does not invoke a shell, which avoids shell interpolation, but an unset `exec.Cmd.Env` causes the child to inherit the entire Mender process environment. That can expose unrelated tokens, cloud credentials, agent sockets, and application secrets to a Provider.

Remote MCP HTTP configuration currently accepts plain HTTP for any host and can encode URL userinfo in the persisted endpoint.

Upstream MCP metadata is already treated as untrusted and must remain so.

## Decision

The management HTTP adapter will enforce a local request guard in addition to loopback listener binding.

It accepts only localhost/loopback Host values. Browser Origin, when present, must also be localhost/loopback. Explicit cross-site Fetch Metadata is rejected. No permissive CORS policy is added.

Remote/public management remains disabled until an explicit identity/authentication/authorization design supersedes ADR-0010.

HTTP MCP Provider endpoints use this transport rule:

- loopback/localhost may use HTTP or HTTPS,
- non-loopback hosts require HTTPS,
- URL userinfo is rejected,
- only HTTP(S) schemes are supported.

Stdio Provider processes receive a small non-secret ambient environment allow-list rather than inheriting the entire Mender process environment. Explicit environment injection remains an infrastructure-only capability so a future Credential resolver can supply narrowly scoped values at execution time.

Mender does not describe this as kernel-level sandboxing. Phase 8 records the absence of a portable OS sandbox and refuses to introduce an abstraction that cannot enforce equivalent security semantics across supported targets.

## Alternatives considered

### Treat loopback binding as sufficient

Rejected because listener scope and browser request provenance are distinct boundaries.

### Add a static bearer token immediately

A token can be useful, but browser token bootstrap/storage and multi-process ownership need an explicit local-control-plane authentication design. Host/Origin/Fetch-Metadata validation is an immediate hardening measure that does not invent incomplete identity semantics.

### Continue inheriting process environment for compatibility

Rejected because ambient environment is a broad implicit credential channel and conflicts with Mender's execution-boundary credential principle.

### Clear the child environment completely

This is stronger isolation but breaks ordinary executable/runtime behavior on multiple platforms. A documented allow-list keeps basic process runtime context without leaking arbitrary secret-bearing variables.

### Claim stdio Providers are sandboxed

Rejected. Process launch without a shell and with reduced environment is not equivalent to filesystem/network/syscall isolation.

## Consequences

Positive:

- loopback management resists common hostile-origin/DNS-rebinding paths more explicitly,
- stdio Providers receive less ambient authority,
- remote Provider traffic is encrypted by default,
- Provider endpoint configuration cannot become a hidden password store,
- existing Domain/Application layers remain unchanged.

Trade-offs:

- some stdio Providers that relied on arbitrary inherited environment variables will need future Credential-backed explicit configuration,
- plain HTTP remote test/development Providers must run on loopback or adopt TLS,
- true OS sandboxing remains platform-specific future work.

## Follow-up

Phase 8 adds regression tests around management request provenance, HTTP endpoint validation, and child environment filtering.

A later ADR is required before remote management or platform-specific Provider sandbox implementations are enabled.

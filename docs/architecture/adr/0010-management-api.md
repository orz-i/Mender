# ADR-0010: Use an OpenAPI-first loopback management control plane

- Status: Accepted
- Date: 2026-09-26

## Context

Mender now has a complete local CLI and MCP runtime, but the browser application is still a foundation screen.

The Web UI needs a stable typed boundary to backend capabilities. Reaching directly into Wails, CLI commands, or scattered handwritten fetch calls would couple product UI to one host and undermine planned Web/Desktop reuse.

Remote management authentication has not yet been modeled. Binding an unauthenticated management API publicly would create a high-risk control plane capable of configuring and invoking Tools.

Policy and Credential domains also do not yet have editable configuration models. A UI should not invent CRUD semantics merely to fill navigation space.

## Decision

Mender defines an OpenAPI 3.1 management contract at contracts/api/openapi.yaml.

The management HTTP adapter uses Go standard net/http and delegates to existing Runtime/Application boundaries.

Phase 6 management serving defaults to loopback only. Remote/public control-plane exposure is deferred until authentication, authorization, CSRF/origin, and identity semantics are explicitly designed.

TypeScript API types are generated with openapi-typescript 7.13.0.

Browser requests use openapi-fetch 0.17.0 behind packages/sdk. UI code outside the SDK package may not call fetch directly.

The Web application is host-independent and receives an SDK/API adapter from the browser host. Wails will provide a host adapter in Phase 7 without forking product UI.

Policy and Credential pages are read-only capability/status views until those domains have real editable application models.

## Alternatives considered

Handwritten fetch in React was rejected because it creates scattered transport logic and weak contract checking.

A large generated runtime client was rejected because generated types plus a small typed fetch runtime are sufficient.

Public management binding was rejected because no remote authentication model exists yet.

Policy/Credential CRUD only for the UI was rejected because presentation requirements must not invent Domain semantics.

Shelling out to CLI commands was rejected because the CLI is an adapter, not an application API.

## Consequences

Positive:
- one explicit contract serves Web and future host integrations.
- strict TypeScript verifies API use at compile time.
- browser fetch logic has one auditable boundary.
- Domain/Application layers remain HTTP-independent.
- local management is safer by default.
- UI accurately reflects current Policy/Credential capability.

Trade-offs:
- OpenAPI contract and generated types must stay synchronized.
- backend DTO mapping code is explicit.
- remote browser management is intentionally unavailable until authentication design exists.
- read-only Policy/Credential pages are less feature-rich than premature CRUD.

## Follow-up

Phase 6 implements the management adapter and Web application.

Phase 7 reuses the product UI behind a Wails-specific host transport.

A future security ADR is required before enabling non-loopback management access.

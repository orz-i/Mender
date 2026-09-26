# Phase 6 Plan — Management API and Web UI

## Objective

Build a browser management surface over the same Mender application/runtime semantics already used by CLI and MCP.

Phase 6 delivers:

- An OpenAPI 3.1 management contract.
- A local management HTTP adapter.
- Generated strict TypeScript API types.
- A single frontend SDK/fetch boundary.
- React management pages for Providers, Catalog, Profiles, Policy, Credentials, and Audit.
- Browser integration coverage for core administration workflows.

It does not introduce a second business-logic path or make the browser responsible for domain rules.

## Control-plane boundary

The management API is a local control plane in Phase 6.

Default server binding: 127.0.0.1:8080.

The Phase 6 server must not implicitly bind to all interfaces.

No remote authentication model is invented in this phase. Public/non-loopback management serving is deferred until identity, authentication, CSRF/origin, and remote authorization rules are explicitly designed.

MCP serving remains independent from the management API.

## Contract-first API

Canonical contract: contracts/api/openapi.yaml.

OpenAPI version: 3.1.0.

The contract owns public management DTO names and JSON field semantics. Backend HTTP handlers map HTTP DTOs to Application/Runtime boundaries and never serialize Domain structs merely because their fields currently match.

## Management API surface

Base path: /api/v1.

System:
- GET /system/status

Providers:
- GET /providers
- POST /providers/http
- POST /providers/stdio
- POST /providers/{providerId}/sync
- DELETE /providers/{providerId}

Catalog:
- GET /tools?query=&limit=&profile=
- GET /tools/{toolId}
- POST /tools/{toolId}/call

Profiles:
- GET /profiles
- POST /profiles
- GET /profiles/{slug}
- POST /profiles/{slug}/tools
- DELETE /profiles/{slug}/tools/{toolId}

Audit:
- GET /audit?limit=

Policy:
- GET /policy/status

Credentials:
- GET /credentials/status

Policy and Credential endpoints are read-only in Phase 6 because no editable application models exist yet.

## Error contract

All errors use a stable JSON shape with code and message fields.

Initial HTTP mapping:
- 400 malformed or validation request.
- 404 Provider, Tool, Profile, or config not found.
- 409 namespace, slug, or reference conflict.
- 403 policy or approval denial.
- 502 provider or execution failure.
- 500 internal persistence or composition failure.

Responses never include stack traces or resolved secrets.

## Audit query application

Phase 6 introduces an Audit application query boundary named ListRecords(limit). SQLite returns newest-first terminal records. The management adapter consumes this application query, not the SQLite store directly.

## Runtime management facade

To keep handlers thin, Phase 6 adds a small management facade around existing use cases. It may orchestrate Provider registration/config/sync/delete, Provider activation for Tool call, Profile-scoped search, Audit query, and status projections. It must not duplicate Domain rules.

## HTTP implementation

Use the Go standard library net/http router. No web framework is introduced.

The adapter lives outside Domain/Application packages. The menderd entry point serves /api/v1/* and defaults to loopback.

## Frontend SDK

packages/sdk becomes the only browser API boundary.

Pinned tools:
- openapi-typescript 7.13.0
- openapi-fetch 0.17.0

Generation flow:

contracts/api/openapi.yaml -> packages/sdk/src/generated/schema.ts -> openapi-fetch client -> typed Mender SDK.

No component or domain hook outside packages/sdk calls fetch directly. CI verifies generated schema output is current.

## Frontend host boundary

packages/app owns product UI. apps/web provides BrowserRouter, API base URL/SDK instance, and browser bootstrap. Future Wails host will provide a different transport adapter without forking product UI.

## Information architecture

Primary navigation:
- Overview
- Providers
- Catalog
- Profiles
- Policy
- Credentials
- Audit

Overview shows Provider count, Tool discovery entry, Profile count, runtime status, and recent Audit summary.

Providers supports list, HTTP/stdio registration, sync, and explicit delete confirmation.

Catalog follows progressive disclosure: search compact metadata, select a Tool, inspect schemas, then optionally invoke with JSON arguments.

Profiles supports list/create, membership inspection, add Tool, remove Tool, and hybrid direct membership.

Policy and Credentials are explicit read-only capability/status pages in Phase 6. No fake CRUD is introduced.

Audit shows newest-first minimized records only.

## UI implementation

Use React 19, React Router, Tailwind CSS 4, shadcn/ui with Base UI, and restrained Motion transitions. The UI should remain a dense developer tool with clear operational IDs and monospace schema/result blocks.

## Data loading

Phase 6 does not require a separate server-state framework. Domain hooks may use React effects/state around the typed SDK. Any future query-cache migration must preserve the same SDK boundary.

## Web development

Vite dev server proxies /api to http://127.0.0.1:8080. Production browser requests use relative /api/v1 paths.

## Testing

Contract:
- OpenAPI schema generates strict TypeScript successfully.
- committed generated types are current.

Backend HTTP:
- system status.
- provider add/list/sync/delete.
- tool search/describe/call.
- profile create/add/remove.
- audit listing.
- policy/credential status.
- error shape/status mapping.
- no secret fields.

Frontend SDK:
- mock fetch only inside SDK tests.
- validate request/response handling against generated path types.

React:
- navigation shell.
- Provider loading/add form.
- Catalog progressive disclosure.
- Profile management.
- Policy/Credential read-only state.
- Audit table.

Integration:
- start the Go management handler against a temporary DB and test MCP upstream, then exercise Provider -> Tool -> Profile -> Invoke/Audit through HTTP.

## Commit segmentation

1. Governance / contract:
- docs: define management API and Web UI architecture
- feat(api): add OpenAPI management contract

2. Backend:
- feat(audit): add audit query application
- feat(api): add management HTTP adapter
- build(server): serve local management API

3. SDK:
- feat(sdk): generate typed management client
- test(sdk): verify OpenAPI generation contract

4. Web:
- feat(web): add management shell and overview
- feat(web): add provider catalog and profile workflows
- feat(web): add policy credential and audit views

5. Integration / closeout:
- test(web): cover management workflows
- docs: complete management Web UI phase

## Exit checklist

- [ ] OpenAPI 3.1 management contract exists.
- [ ] Management server defaults to loopback.
- [ ] Management HTTP handlers use Runtime/Application boundaries.
- [ ] Provider add/list/sync/delete API works.
- [ ] Tool search/describe/call API works.
- [ ] Profile create/list/get/add/remove API works.
- [ ] Audit newest-first query API works.
- [ ] Policy status API is explicit and read-only.
- [ ] Credential status API is explicit and read-only.
- [ ] API errors use stable JSON shape.
- [ ] openapi-typescript 7.13.0 is pinned.
- [ ] openapi-fetch 0.17.0 is pinned.
- [ ] generated TypeScript contract is current.
- [ ] packages/sdk is the only browser fetch boundary.
- [ ] Web navigation exposes Overview/Providers/Catalog/Profiles/Policy/Credentials/Audit.
- [ ] Provider administration works in browser.
- [ ] Catalog uses search -> describe progressive disclosure.
- [ ] Profile administration works in browser.
- [ ] Policy/Credential views do not invent unsupported editing.
- [ ] Audit view displays only minimized records.
- [ ] Strict TypeScript checks pass.
- [ ] Go race tests pass.
- [ ] pnpm check passes.

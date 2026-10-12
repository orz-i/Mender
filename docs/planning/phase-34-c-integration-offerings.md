# Phase 34 C — Reviewed integration offerings and AnySearch Remote MCP

Status: Accepted implementation contract (2026-10-12), after ADR-0039 and Phase 34 Stage B.

## Current behavior and why an integration is different from an extension

`io.mender.provider.mcp` already implements Streamable HTTP, Credential Reference headers, Profile-scope governance and canonical Tool synchronization. Another compiled/native AnySearch Provider would duplicate it. The Plugin Catalogue lists technical typed extensions; it does not represent an AnySearch **configuration offering**.

AnySearch's published MCP instructions (https://github.com/anysearch-ai/anysearch-mcp-server) document a Streamable HTTP endpoint `https://api.anysearch.com/mcp`, a static `X-Anysearch-Client: mcp/1.0.0` header and optional `Authorization: Bearer` API key. Anonymous service may be rate limited. These are reviewed product defaults, **not evidence of connectivity**, API charges or entitlement. Do not access a live AnySearch account in automated acceptance.

## Contract and authority

- One inert, source-reviewed **IntegrationOffering** descriptor, distinct from a PluginID/InstallationID:
  - stable `offeringId`, `displayName`, `description`, `kind: remote-mcp`, `driverPluginId`, `transport: streamable-http`, `endpoint`, `headers`, `defaultNamespace`, `authentication: optional-bearer`.
  - only non-secret fields; every descriptor is validated statically (HTTPS endpoint, built-in MCP driver pin, no `Authorization` in static headers). Future registry-sourced entries must enter a separately reviewed/trusted state before being offered as the same authority.
- Read-only `GET /api/v1/integrations` and equivalent SDK/Wails method return exact vetted offerings. Listing cannot launch processes, probe an endpoint, resolve credentials, add a Provider or create Profile access.
- Marketplace merges **offering cards** with existing technical PluginCatalogue projections only for presentation. Their IDs and action types remain distinct. A reviewed preset is `available` to configure, not `connected`/healthy.
- `Connect` for AnySearch opens a focused existing Provider creation flow with vetted endpoint, transport and header **read-only**; user provides a connection name, optionally adjusts namespace in Advanced, and chooses an optional existing Credential Reference (or anonymous). No user-supplied endpoint/command/input schema is embedded in the offering action.
- The resulting Provider remains a canonical `io.mender.provider.mcp` instance; existing Provider Activation, Credential resolution, Tool discover/sync and Profile/Policy gates apply. The default Profile exposure is **none**. Non-presetted MCP remains available from the advanced generic Provider flow.
- The UI route binds by exact `offeringId` from the host catalogue and does not trust arbitrary URL query parameters as endpoints. The generic Provider API intentionally permits an owner to configure other MCP endpoints; that is a separate explicitly supported advanced path, not a privilege acquired from an Offering.
- No new executable extension protocol, no store for redundant installation records, no automatic credential creation or remote registration, no implicit `ToolBinding`/Profile grant, no automatic native Marketplace downloads.

## Verification

1. A safe fixtures-only integration listing returns one exact AnySearch descriptor with no secret and no process/network effects; malformed IDs do not create UI actions.
2. HTTP, Wails and SDK expose identical reviewed metadata; TypeScript validation rejects unknown kinds/transport/auth.
3. Marketplace shows a distinct AnySearch card, `available` status, `Connect` action, and retains native plugin management untouched. Cursor filters/search remain correct.
4. Create flow preselects the built-in MCP driver with sealed endpoint/header, optional credential ref and a focused form; custom MCP creation still exists.
5. Validation and mock-created Provider show correct exact MCP transport config and no automatic Profile binding. Real remote handshake is **operator-only**.
6. `pnpm check`, fixture-based smoke, and package build pass before closure. Record commit hashes per bounded gate; keep dot-skill directories untouched.

## Follow-on

The broader “Providers → Connections” navigation rename and registry ingestion can follow only after stable resource ownership/selection semantics are proven. Stage D handles typed Browser Session/Tool Suite compatibility; neither of these introduces a new Agent loop.

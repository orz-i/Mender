# Phase 31 — Sandbox backends in the unified Plugin Marketplace

Status: **Closed** (2026-10-11). Builds on closed Phase 27 E and Phase 28–30. Implementation is document-first, incremental and hard-cut: no Provider bridge, duplicate Marketplace, or agent loop.

## Current audit

The Sandbox selector exposes four host labels: Local Workspace, Docker, Daytona (experimental), and E2B (planned). The canonical Marketplace already lists Local Workspace, Docker (including dormant/disconnected), and installed external `sandbox.backend` packages. Daytona is **hidden** whenever `MENDER_DAYTONA_EXPERIMENTAL` is not `enabled`, despite existing quarantined code and a resource create form. E2B has only an inert disabled selector tile and **no implementation**. Creating a built-in Sandbox from a Marketplace detail already routes through `/sandboxes/new?create=1&pluginId=...`; external packages use the exact installation pin.

## Intended catalog semantics

| Backend | Marketplace entry | Runtime registration | Resource creation |
| --- | --- | --- | --- |
| Local Workspace | Built-in `io.mender.sandbox.local-workspace@builtin`, enabled | Existing local trusted backend | Existing focused local-root/isolation review |
| Docker | Built-in `io.mender.sandbox.docker@builtin`, enabled or unavailable per verified Engine health | Existing pinned Engine adapter only | Existing locally-verified immutable-image review |
| Daytona | Built-in `io.mender.sandbox.daytona@builtin` **always visible**; unavailable by default; enabled only after explicit host opt-in | Existing controller stays quarantined unless `MENDER_DAYTONA_EXPERIMENTAL=enabled` | Existing Credential/remote quotas/billing review, only when opted in |
| E2B | **Planned preview**, `io.mender.sandbox.e2b@planned`, `sourceKind=planned`, `status=planned`, `enabled=false`, no package/installation/version/capabilities | **No executable backend, no installable package, no fake Registry entry** | **No create action**, ever in this phase |

External `sandbox.backend` packages remain the only truly locally installed Sandbox plugins; they stay typed and pinned by exact InstallationID. No new real E2B integration or Daytona live-service claim is requested. An E2B preview must *not* masquerade as an installed first-party implementation. Only a release with real code, contract and tests may promote it.

## Contracts and UX

1. `GET /plugins`, Wails `ListPluginCatalogue`, generated SDK and Web/Desktop all return identical status/source shapes; `planned` is **catalogue-only**, not a Provider installation state. Typed source and status extend *only* `PluginCatalogueEntry` to include `planned`; no installation lifecycle/enable/delete endpoint accepts it.
2. All four items have stable unique keys, truthful descriptions, and correct kind `sandbox.backend`. A disabled Daytona entry is a metadata snapshot, and listing never touches remote credentials, billing, Docker sockets or processes. Daytona issue shows the exact operator startup flag in localized UI; E2B shows `Planned`, a neutral unimplemented description and **no red operational-error banner**.
3. `createAction` exists only when the backend is actually enabled and has no issue. Built-in routes preserve selected PluginID; local and Docker continue their current flow, Daytona starts its own experimental form and review; E2B has no create action or fake wizard. Pagination/filter/status/source must include planned entries without breaking installed plugin management.
4. Negative tests: disabled Daytona/E2B cannot create or acquire a Sandbox (or use Provider fallback), E2B cannot be registered by a fake builtin, all-key uniqueness, listing inertness, correct HTTP/Wails round-trip and UI status. Positive: enabled experimental Daytona selection reaches credential-gated review; Local Workspace and Docker route unchanged.
5. Ignore `.agents/`, `.pi/`, `.intent-loop/`; do not alter existing Runtime state, DB files, user containers or remote API credentials.

## Delivery

- A: this contract commit.
- B: backend catalogue projection and HTTP/schema/SDK/Wails acceptance, tests.
- C: focused Marketplace planned UX and end-to-end navigation/filters, i18n, full `pnpm check`, security scan and desktop rebuild; close Phase 31 with no subphase inflation.

## Scope boundary

The Plugin Marketplace is Mender's local unified software catalogue, not a public distribution registry. An upcoming E2B adapter is informational only; Daytona remains a deliberate experimental operator opt-in, not auto-connected to a billable remote service.

## Acceptance and closure (2026-10-11)

- `c1c72ae` — contract before implementation, including exact non-executable E2B preview semantics.
- `66435ce` — existing Daytona code appears in the canonical Catalogue even while host experimental opt-in is off; E2B appears once as `@planned`, not `@builtin`; SDK/OpenAPI/Wails source + status types aligned. Inert catalogue/HTTP tests cover one stable key per backend and opt-in transitions; existing Registry test continues to reject attempting to register E2B.
- `0b002ba` — Marketplace grid, filters, status/source labels and previews include `Roadmap / Planned`. Local Workspace and explicitly opted-in Daytona navigate to their own existing focused configuration flows. No E2B create/enable/remove action or fictional package version; Chinese translations included.
- `bd6e81e` — negative HTTP acceptance confirms there is no E2B creation endpoint, even when the informational card is discoverable.

Test gates: `pnpm check` passed with all Go lint/unit/race gates, HTTP/SDK/Wails schema checks, 23 Vitest files / **155 passing tests**, and **962/962** required zh-CN translations. `pnpm security:check` passed on 8 Go modules and production JS dependencies (Go vulnerability feed used its supported fallback). Native macOS desktop `GOWORK=off go build ./...` passed with pre-existing macOS linker target warnings. No new external daemon, Docker action, remote Daytona API call, Credential, billable provisioning, or user DB migration was initiated by Marketplace listing.

The Sandbox create chooser is retained for existing entry points. The Marketplace is now also a complete catalogue of those four options, not a second resource inventory; exact installed external `sandbox.backend` packages remain in the same source. Product-preview E2B remains explicitly unimplemented until a separately governed implementation is accepted. Native packaged WKWebView visual release QA is not claimed by these automated checks.

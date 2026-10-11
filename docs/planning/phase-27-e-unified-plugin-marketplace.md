# Phase 27 E — Unified Plugin Catalogue and typed management closure

Status: **Closed for implementation (2026-10-11)**; ADR-0037 accepted.
Automated acceptance completed; real browser screenshot/mobile visual QA was
blocked by the existing Pi browser journal format and is explicitly retained
as a separate release QA gate, not silently claimed as passed.

## Audited baseline

- `providerplugin` retains an existing Provider descriptor inventory, exact installation management, and Provider config flows.
- `plugin/installation` stores one authoritative all-kind package inventory; runtime supports `provider.tools`, `sandbox.backend`, and `tunnel.transport` separately with typed process dialers and immutable pinned resources.
- `packages/app/src/domains/plugins/marketplace.ts` registers only the Provider adapter, combining the Provider catalogue and Provider-only installation list. No multi-kind backend catalogue yet.
- `contracts/api/openapi.yaml`, `packages/sdk/src/client.ts`, `backend/internal/desktop/binding`, `apps/desktop/frontend/src/desktop-client.ts` provide Provider-specific plugin installation actions; external Sandbox/Tunnel create flows remain CLI/Runtime-only.
- Sandbox creation already has focused selection → config → limits → review; Tunnel creation is a dialog; Provider creation is a focused flow. Retain the “one information type per viewport; lists are previews” interaction rule.

## Contract: one authoritative catalogue, typed resource workflows

A **Plugin Catalogue** entry is one built-in typed extension or a verified pinned native installation. Projection must be inert (no process launch, credential resolution, network probe, provisioning, managed server startup). Each entry has stable key, PluginID, extensionKind (`provider.tools`/`sandbox.backend`/`tunnel.transport`), ExtensionID, packageVersion, sourceKind, status, enabled, descriptive capabilities, supported config metadata, InstallationID/digest if native, and resource usage count/diagnostic if safely available. Health/issue must not imply an advertised capability actually works or an endpoint is public. Distinguish unverified installation from unavailable plugin.

**One generic installation management surface** handles exact package status/disable/delete based on verified extensionKind and owning resource usage checks. Provider/Sandbox/Tunnel resources retain their own contracts and IDs. Replace the Provider-specific installation list/actions as UI authority (do not implement a second shadow inventory), while retaining Provider runtime discovery for its Tool descriptor/config schema where necessary.

Type-owned create endpoints:
- Provider: existing `/providers` and pluginId+installationId pin, no handwritten ToolID.
- Sandbox: `/sandboxes/external` with `name,installationId,config` (bounded JSON), and `/sandboxes/{id}/reconcile` for admitted manual reconciliation; built-in local/Docker/Daytona stay in the existing guided flow.
- Tunnel: `/tunnels/external` with `displayName,installationId,externalTunnelId,runtimeCredentialRef`; built-in OpenAI stays in the same focused Tunnel dialog. Raw secret values and an arbitrary MCP endpoint are never accepted.
- HTTP/Desktop/Wails/SDK must use exactly the same typed data semantics, resource binding, lifecycle and errors. No unauthorized install via a remote Web path or native executable upload: local Desktop chooser remains the only UI package-install entry in this phase; Web can discover, inspect, operate and configure already installed extensions.

Marketplace UX:
- Type/search/source/status/refresh/install on a single toolbar row; card grid with cursor pagination; details with separate overview/capabilities/technical panels and actions; enabled-only Add linking to exact owning resource flow.
- `provider.tools` → Provider create selected PluginID+InstallationID; `sandbox.backend` → Sandbox create with selected built-in or exact native driver; `tunnel.transport` → Tunnel dialog with selected built-in or exact native driver. Each focused flow shows only its own validated form, with lifecycle consequence review and Credential selector, never plugin-injected scripts.
- Pin exact InstallationID and enforce that it is still enabled on submit (backend admission); do not silently fall back from native Sandbox/Tunnel to host/OpenAI. Destructive actions must preview usage and be guarded both at the API and the persisted SQLite boundary.
- Native installer is explicitly a trusted local action. Web/remote management cannot accept a client-origin filesystem path.

## Work packages — commit independently

1. **E0 — decision & acceptance**: this doc, explicit data shape & hard-cut/no compatibility bridge rule.
2. **E1 — catalogue + installation HTTP**: read-only unified backend catalogue, one generic installation management API with typed dispatch and usage projection, contracts and regression tests.
3. **E2 — typed resource API parity**: external Sandbox & Tunnel create/reconcile, stable existing projections and error handling; OpenAPI schema, SDK and Wails parity, negative cases.
4. **E3 — UI**: share one catalogue source for all kinds, keep cards and isolated detail/creation surfaces, exact plugin pin, safe local Desktop installation chooser; translation parity.
5. **E4 — closure**: unit/integration/Wails/Desktop UI tests, contract drift, all Go Race/lint, `pnpm check`, `pnpm security:check`, production Web/Desktop build. No user real Sandbox/Tunnel, Docker daemon, remote provider account or Credential secret used.

## Non-goals

No Agent loop, arbitrary dynamic React/HTML, public native-plugin installation via web, OCI registry, multi-extension binary or guessed cloud capabilities, no generic platform reachability claim from a local loopback health probe. No permanent provider-vs-generic installation dual-read in the Marketplace. No endless Phase E subphase expansion. Never commit `.agents/`, `.pi/`, `.intent-loop/`.


## Phase E closure evidence (2026-10-11)

| Acceptance | Evidence | Result |
| --- | --- | --- |
| Document-first scope | `de4217f` (this contract before code) | Complete |
| Unified typed host Catalogue | `16925f5`, `backend/internal/bootstrap/plugin_catalogue.go` | Process-free; all Provider/Sandbox/Tunnel extensions |
| Canonical installation state | `plugin_management.go`, HTTP `/plugin-installations`, Wails and SDK | Exact verified kind dispatch, reference-based disable/delete guards |
| Typed resource HTTP | `backend/internal/management/httpapi/external_resources.go`, OpenAPI schemas, generated TS SDK | External Sandbox and Tunnel creation, shared identity/status/credential handling |
| Native desktop installer | `desktop/binding/Service.InstallLocalPlugin`, Wails generated TypeScript binding, OS folder chooser | Trusted local action only; no remote path install API |
| Marketplace Web/Desktop | `bb51d8e`, `packages/app/src/domains/plugins/` | One backend catalogue, card grid, cursor pagination, type/source/status/search, detail tabs |
| Selected create UX | `sandbox-external-create-page.tsx`, focused built-in Sandbox, Provider and Tunnel creation | Exact software pin, relevant form only, review/credential selector |
| UX and API hardening | `ab12333`, dedicated removal confirmation, builtin Sandbox back navigation, typed wrong-kind/invalid Tunnel errors | WKWebView-safe; no category fallback |
| Cross-kind fixture acceptance | `TestCanonicalCatalogueInstalledThreeKindsAndUsage`, `TestUnifiedPluginCatalogueHTTPExternalResourceFlow` | Real test-built native fixtures: descriptors, inert listing, exact resource usage, disable conflict and positive HTTP create |
| Existing governance preservation | Sandbox Router/lease/Policy and Tunnel/Profile reference tests, SQLite migration tests | No bypass or resource reassignment |
| UI/desktop contract | `management-pages.test.tsx`, `desktop-client.test.ts`, Wails binding generation | No raw secrets or installed package paths exposed by browser UI |

### Final automated gates

- `pnpm check`: protobuf + Go SDK/examples generation, type-safe Wails/HTTP
  contracts, Go lint/unit/Race, Web/Desktop TypeScript lint and typecheck,
  i18n parity, **140/140** React tests, Web/Desktop production frontend build.
- `pnpm security:check`: `govulncheck` across backend, desktop and all
  provider/sandbox/tunnel Go SDK and examples, plus production JS audit.
- `GOWORK=off go build ./...` for the macOS desktop Go app passed; macOS
  deployment-target linker warnings persist but do not fail the build.
- Isolated disposable-host smoke: newly built `menderd` with a temporary
  SQLite DB on loopback, matching Vite proxy on a separate loopback port:
  `GET /api/v1/plugins`, Web proxy `GET /api/v1/plugins`, and `GET /plugins`
  returned HTTP 200. The catalogue had all three built-in extension kinds,
  with identical direct-API/proxy JSON. Both test processes, database,
  temporary Vite config and binary were cleaned.
- Positive HTTP/resource fixtures did not contact any actual Sandbox cloud,
  Docker daemon, OpenAI account, public relay, user-owned resource or
  production Credential.

### Outstanding release-specific checks (not misreported as complete)

**Native visual browser QA:** the Pi agent browser refused the current
project session because it contains legacy browser records. Even a fresh
browser mode was blocked; consequently no desktop/mobile screenshot audit or
real WKWebView click-through has been claimed. RTL tests cover the
Marketplace/detail/creation and in-app confirmation interactions, and the
disposable Vite/HTTP integration is green. Screenshot-driven layout,
responsive mobile breakpoints and packaged/signed Wails release QA remain
for the next **release verification**, rather than extending Phase 27 into
an unbounded E1–E100 series.

**Backend/cloud limitations:** the native external Sandbox and Tunnel
examples exercise deterministic local fixtures only. Real remote account
reachability, billing caps, OS-level native-plugin isolation and external
package distribution/registry are not implied by a Marketplace card or
`enabled` status. Existing built-in Docker/Daytona opt-in and OpenAI
downstream handshake gates remain authoritative.

### Implementation commits

- `de4217f` — E acceptance plan/contract
- `16925f5` — Unified backend catalogue and typed HTTP/SDK/Wails resource management
- `bb51d8e` — Marketplace and focused resource creation UI
- `ab12333` — HTTP/inventory E2E and desktop-safe UX hardening

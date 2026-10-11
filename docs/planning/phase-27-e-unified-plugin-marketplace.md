# Phase 27 E — Unified Plugin Catalogue and typed management closure

Status: In progress (2026-10-11); ADR-0037 accepted. Previous Phase A/B/C/D implemented and independently committed.

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

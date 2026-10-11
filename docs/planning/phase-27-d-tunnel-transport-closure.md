# Phase 27 D — Tunnel Transport Plugin closure

Status: In progress (2026-10-11). Governing ADR-0037 and ADR-0031.

## Intent and acceptance

Make Tunnel a real `tunnel.transport` extension kind. Preserve existing
OpenAI secure tunnel config/Profile linking, credential references, and local
managed MCP server guarantees. Add one native external Tunnel transport
protocol/SDK and a deterministic no-network sample exercising a real managed
loopback MCP listener. Keep Mender Tool and Resource only; no Agent loop.

## D1 — typed domain and forward-only persistence

- `TunnelBinding{PluginID, ExtensionID, InstallationID}` records exact software
  identity separately from TunnelID and provider-facing ExternalTunnelID.
- Built-in OpenAI transport is `io.mender.tunnel.openai/main`, with empty
  InstallationID, and retains `provider=openai` and required Credential Ref.
  External extension uses `provider=external` and a non-empty exact
  InstallationID. No untrusted config or Profile argument chooses an executable.
- SQLite v3 hard-cuts existing Tunnel rows in a transaction. It preserves
  TunnelID, ExternalTunnelID, Credential Reference and Profile foreign
  references; creates a WAL-consistent pre-upgrade backup; rejects future
  schema versions and wrong-kind/unknown/forged extension bindings.
- New external Tunnel creation requires a verified, **enabled** exact
  `tunnel.transport/main` installation. SQL triggers enforce installation
  availability on insertion and block disabling or removing a referenced
  package, including Tunnel resources no longer linked by a Profile.
- No permanent legacy read path or runtime derivation of plugin identity from
  the old provider field.

## D2 — typed transport runtime

- Mender owns a typed Tunnel Transport registry, common package installation,
  process supervisor, Credential resolver, managed endpoint choice,
  Profile→Tunnel binding, start/stop/watch/rollback.
- The existing OpenAI `tunnel-client` invocation moves into a trusted built-in
  transport. It keeps loopback-only health and secret-as-environment semantics.
- External `tunnel.transport` Protocol v1 is separate from Provider/Sandbox
  RPC: descriptor, config inspection, connect(host-selected loopback MCP
  endpoint + TunnelID + narrowly scoped Credential bytes), status/health,
  disconnect, no Listen/Bind/Policy/Principal management methods.
- A running Tunnel session is identified by a bounded opaque session ID; host
  never gives the plugin a public bind address. A transport's reachability
  semantics and health evidence are provider-specific; launched child !=
  confirmed downstream ready.
- A transport is obtained **before** resolving secrets: disabled/unknown/
  wrong-kind installation fails without touching the Credential store or
  launching executable.
- A native Go plugin retains OS-account authority; gRPC/mTLS/manifest checksum
  is not OS isolation. Do not promise external auth, public access or readiness
  unless measured.
- Catalogue/config/Startup reads never invoke native plugins or network;
  actual connect occurs only when starting a managed Profile MCP endpoint.

## D3 — managed server semantics

- Strict host-selected loopback HTTP MCP endpoint; reject external URLs and
  disallowed schemes in both OpenAI and external adapters.
- Start MCP Server first, then connect Tunnel; failures stop/rollback the
  managed server and terminate any started transport session.
- Explicit start/readiness/health, degraded/crashed, stop/restart, bounded
  teardown and a process-exit watcher. Never automatically reopen/retry
  uncertain external reachability.
- Tunnel ID, external ID, Credential Ref remain in management projections.
  Profile deletion/reference checks and Credential usage checks remain.
- External fixture launches against an actual loopback managed MCP server and
  reports deterministic health; no real OpenAI account or API key needed.

## D4 — deliverables and verification

- Architecture/plan before implementation, then distinct commits for domain
  + migration, SDK/protocol, host adapters and final validation.
- Test: v2→v3 migration (ID + Profile link + Credential preserved),
  forward-only failures/backup/restart, exact package pin/status, wrong kind,
  package tamper, no launch on metadata reads, secret not logged/persisted,
  loopback enforcement, fake external connect/health/stop/crash,
  managed-server startup rollback, no leaked listeners, runtime shutdown,
  disabled-package create/remove races, linked Tunnel and Credential deletion
  restrictions, CLI/API/SDK/Wails existing OpenAI parity.
- `pnpm check`, `pnpm security:check`, Go Race for touched packages,
  native sample Go SDK/protobuf check, desktop Go build.
- Real OpenAI tunnel connectivity and generic Marketplace UI deferred to
  integration acceptance / Phase E respectively. No real Tunnel/Credential
  changes during tests; no edits to `.agents/`, `.pi/`, `.intent-loop/`.

## Scope limits

No automatic download registry, arbitrary native plugin confinement, new
Tunnel brand adoption, multi-extension package or externally accessible MCP
listener. Do not invent public Tunnel URLs or pass raw secrets in persisted
package/resource config. Incompatible external plugins remain unavailable
rather than silently falling back to OpenAI.

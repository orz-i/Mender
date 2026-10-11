# Phase 27 D — Tunnel Transport Plugin closure

Status: **Closed (2026-10-11)**. Governing ADR-0037 and ADR-0031.

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


## Final Phase D acceptance evidence

| Gate | Source and observation | Result |
| --- | --- | --- |
| Typed Tunnel identity | `tunnel/domain/{binding,tunnel}.go`; exact OpenAI built-in or installed third-party binding | Pass |
| Forward-only v3 hard cut | `tunnel/infrastructure/sqlite/migrations.go`, `migrations_v3_test.go` | Preserved historical TunnelID, ExternalTunnelID, Credential Ref, Profile link; backup/rollback/restart |
| Installation usage | `plugin/installation/infrastructure/sqlite/migrations.go` | Atomic create/disable/delete guards; referenced and unbound Tunnels block removal |
| Separate typed wire contract | `contracts/tunnelplugin/v1/`, `sdk/tunnelplugin/go/` | Go gRPC v1, handshake, descriptor and config inspection |
| External native test fixture | `examples/tunnel-plugin-example/` | Strict literal loopback `/mcp`, no public Tunnel or cloud I/O |
| Shared host | `tunnel/infrastructure/external/{dialer,runner,session}.go` | Exact package digest, version and descriptor verified before Credential resolution |
| OpenAI built-in path | `tunnel/infrastructure/process/runner.go`, `pluginregistry/registry.go` | Unchanged OpenAI command arguments and Credential environment |
| Managed MCP lifecycle | `bootstrap/mcp_servers.go` | Listener first; Tunnel connect second; failure rollback, health cache, bounded stop/crash handling |
| Lifecycle/health E2E | `bootstrap/tunnel_plugin_e2e_test.go` | Ready/degraded/exited, re-connect, stop, invalid URL and deterministic Connect failure |
| Governance E2E | `bootstrap/tunnel_plugin_e2e_test.go`, original `mcp_servers_test.go` | Profile/Credential reference protection, active Tunnel deletion prevention, wrong-kind rejection |
| Secure management | `bootstrap/tunnel_plugin_installations.go`, `cli/tunnel.go` | Generic package CLI, typed Tunnel inventory/add/delete, fail-closed state changes |

Quality checks:
- `pnpm provider-plugin:proto:check` (all three typed protocols) and
  `pnpm tunnel-plugin:sdk:check`, `pnpm tunnel-plugin:example:check` passed.
- `go test -race ./backend/...` and `golangci-lint run ./backend/...` passed.
- `pnpm lint`, `pnpm typecheck`, `pnpm test` (132/132),
  `pnpm build` (Web and Desktop frontend) passed.
- `pnpm security:check` scanned the new Tunnel SDK/example Go modules
  and existing modules plus production JavaScript dependencies, with no known
  vulnerabilities reported.
- `GOWORK=off go build` in `apps/desktop` succeeded, with existing macOS
  minimum-deployment linker warnings but no error.

Verified code commits: `db21a7f` (exact binding and SQLite migration),
`262a398` (Tunnel v1 SDK/example), `f14904e` (managed runtime, CLI and tests).

**No real OpenAI account or Tunnel API key was accessed.** The native example
was built and run using temporary test files, environment fixture credentials,
and a real *local-only* managed MCP endpoint. It is not evidence of public
reachability or real OpenAI tunnel registration.

## Explicit carry-forward

**Phase E** owns a unified backend Plugin Catalogue plus Web, Desktop/Wails
and TypeScript SDK management UI flows for plugin discovery, installation,
details, creation, filtering and resource usage. This Phase D supports the
actual external native transport through Runtime and CLI; it does not claim
a completed Marketplace UI.

An OpenAI `tunnel-client` process that merely started is reported
`starting`, not `ready`; OpenAI control-plane registration still needs
explicit real-world verification. Third-party native plugin binaries run
with the OS account's authority unless a separate OS-enforced sandbox is used.
The example reports readiness for **local loopback MCP reachability only**.

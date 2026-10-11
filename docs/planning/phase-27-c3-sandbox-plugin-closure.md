# Phase 27 C3 — Sandbox Backend Plugin execution closure

Status: **Closed (2026-10-11)**. Governing ADR-0037/0035/0036.
Head implementation: `272c810` (plus C3 protocol `8ca5704`, prior contract `e99d22f`).

## Outcome and bounded delivery

Make Sandbox an **actually executable extension kind**, rather than only a built-in
registry/manifest label. A checked-in native example must use the public
`sandboxplugin/v1` protocol without importing `backend/internal`. It must be
possible to install, validate, configure, provision, acquire a bounded workspace
session, run an operation, reconcile a reported uncertain outcome and destroy a
Sandbox through Mender-owned admission and lifecycle state.

Mender stays a Tool/Resource runtime and **not** an Agent loop.

## C3 contracts

1. Extend Sandbox backend classification with exactly one explicit
   `external` classification, never fall back to Docker/Daytona/local.
   `BackendBinding` holds a namespaced PluginID, the exact `main` extension
   and an immutable non-empty InstallationID. Built-ins continue using their
   v9 exact bindings and original verified implementations.
2. `sandbox.backend` gRPC v1 is a separate public, versioned protocol: typed
   descriptor, Validate/InspectConfig, lifecycle operations with stable
   SandboxID/generation/idempotency operation key and opaque external run ID,
   Open/ExecuteWorkspace/Close session. It cannot define Policy, allocate Mender
   Principal, choose a Sandbox or override a persisted lifecycle transition.
3. The shared `plugin/process.Supervisor` verifies exact package bytes and
   selects the `sandbox.backend` typed dialer; Provider gRPC v1 is unchanged.
   Catalogue reads/startup do not launch plugins or resolve credentials.
   Runtime descriptor must match installation-verified identity/capabilities.
4. Mender owns the authoritative Sandbox database, run/lease CAS, admission
   policy, budget and audit. Provision/Destroy/Start/Stop/Reconcile and
   Acquisitions are host operations only. No Tool or Profile can provision, bind
   or rebind an execution target. No native plugin is claimed to be isolated
   from the host OS account; installed third-party binaries are explicitly
   trusted.
5. Forward-only v10 migration adds an immutable external Sandbox config table
   keyed by SandboxID. Create Sandbox/Run/config in a single transaction.
   Mender stores bounded non-secret JSON, config version and exact installation
   pin; it does not persist raw credential values. First external protocol
   variant rejects credential refs entirely; credential-backed cloud
   provisioning requires explicit host-scoped credential handoff in a later
   extension version rather than granting a general host CredentialResolver.
6. Remote operations use explicit fail-closed outcomes and are never replayed
   after deadline/cancel/transport uncertainty. Unknown and orphaned runs
   remain visible until a separately admitted reconcile. No implicit
   substitution to host execution.
7. Disable/remove of an external Sandbox driver fails while any Sandbox
   references that exact InstallationID, **including destroyed records still
   retained for audit/cleanup**; no remote instance can be silently stranded
   behind a disabled package. Retargeting is not automatic.
8. Built-in lifecycle-specific configuration remains host-owned. A typed
   controller dispatch replaces the Bootstrap vendor switch only after all
   existing local/Docker/Daytona workflows have integration tests.

## Work packets / commits

- **C3.1 — protocol and SDK:** Go/protobuf generation, typed handshake, public
  server helper, native mock extension with deterministic local-only state.
  No host resource changes.
- **C3.2 — runtime integration:** external plugin process connector,
  installation/descriptor validation, typed Registry resolution and
  immutable external config persistence; dry-runnable managed provision flow,
  no real cloud.
- **C3.3 — lifecycle orchestration:** govern before remote operations,
  atomically preserve/transition authoritative Run status, execute only behind
  Router lease; safe unknown/reconcile/delete and package usage guards. CLI
  headless management for install/create/actions; API/UI generalization stays
  in Phase 27 E unless needed for correctness.
- **C3.4 — acceptance/closure:** fake native extension E2E (including denied
  operations, duplicate create, tamper, wrong kind, uncertain remote result,
  restart, inactive plugin, no Host fallback), existing Sandbox Profile grant
  and lease tests, all Go Race/tests/lint, proto/SDK/build/management checks,
  `pnpm check`, `pnpm security:check`; explicit Phase C closure evidence.

## Non-goals

No E2B/Modal/Cloudflare account, real network, Docker Engine test daemon or
Daytona calls. No unrestricted raw shell or host mount via plugin metadata.
No dynamic React/HTML. No native binary OS sandbox claims. No compatibility
bridge, speculative Phase C subphase proliferation, automatic retry of
side-effecting calls, or modification of `.agents/`, `.pi/`,
`.intent-loop/`.

## Acceptance gate

Phase C is only complete when a third-party plugin package can be installed
and a governed fake external Sandbox can execute and reconcile through the
**same** Router, persistent lease, exact binding, authority and audit path as
built-ins. Otherwise status remains partially complete with explicit blockers.


## Closure evidence (2026-10-11)

| Contract | Implementation evidence | Result |
| --- | --- | --- |
| Distinct external protocol | `contracts/sandboxplugin/v1/sandbox_plugin.proto`, `sdk/sandboxplugin/go/`, `examples/sandbox-plugin-example/` | Pass |
| Shared process host | `sandbox/infrastructure/external/dialer.go`, `plugin/process`, `bootstrap/runtime.go` | Exact kind/version/digest/descriptor verified |
| Durable exact binding | `sandbox/domain/BackendBinding`, plugin registry and Router | No wrong-kind, absent driver or Host fallback |
| Governed lifecycle | `bootstrap/sandbox_external.go`, typed controllers, Sandbox Policy/admissions | Admission before native RPC, owner identity enforced |
| Crash/uncertainty | `sandbox_external_operations`, `TransitionExternalRun` and manual `sandbox.reconcile` | Stable op ID, uncertain state retained, no automatic re-provision |
| Config and migrations | SQLite v10 external configs, immutable pin triggers, v8/v9 snapshot creation | Stable Sandbox IDs, grants, leases, Docker/Daytona templates and audits |
| Safe package management | Atomic plugin installation disable/delete and Sandbox insert guards | Concurrent disable/create, referenced (including destroyed) removal rejected |
| Full execution | `TestExternalSandboxWorkspaceProviderCanonicalToolInvocation` | Catalog → Invocation → Workspace → Router → external plugin + audit |
| Multi-process execution | `TestExternalSandboxLeaseContentionAcrossRuntimes` | Second Runtime rejected before external process launch |
| Negative tests | Sandbox E2E tamper, denied provision, unknown/reconcile, restart and no fallback | Pass |

**Quality checks:** `pnpm check` (Go lint, unit/Race, SDK/protobuf,
OpenAPI/TS/Wails drift, i18n, 132 frontend tests, Web/Desktop frontend
builds), `pnpm security:check` (including new Sandbox SDK/example Go modules),
and `GOWORK=off go build ./...` within `apps/desktop` passed.
The standalone native sample was built and exercised inside temporary
test-managed directories; **no user-owned Sandbox, real remote account,
Docker daemon, Daytona API, or Credential secret was accessed.**

The macOS desktop Go build reported SDK deployment-target linker warnings,
but exited successfully. Those warnings do not establish a packaged/signed
release application; packaging verification belongs to a release gate.

### Explicit carry-forward

- **Phase D:** make Tunnel Transport a typed installable extension. The
  existing OpenAI tunnel remains governed; no new protocol exposure is
  implied by Sandbox C3.
- **Phase E:** unified plugin catalogue and Web/Desktop typed management
  flows. C3 only provides an executable backend Runtime and CLI workflow.
- Production cloud backends (E2B/Daytona/other), OS-level confinement for
  third-party native code, provider bill budgeting and direct credential
  handoff require separate provider-specific admission/remote acceptance.
  The fake Sandbox is neither a container/VM nor a general cloud provider.
- A crashed external operation is not retried automatically; orphan or
  uncertain remote state requires explicit operator reconciliation. Native
  plugin code runs with its OS account's authority, so installing an external
  package is a trusted operator action.

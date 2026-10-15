# ADR-0042 — P2 durable resource contracts and native plugin isolation posture

- Status: Accepted for bounded Phase 36
- Date: 2026-10-14/15
- Refines ADR-0002, ADR-0033, ADR-0039, ADR-0041; supersedes **only** ADR-0033's *fresh external Provider default-host* choice.
- Mender remains the LLM/Agent Tool + Resource aggregation, governance and supply plane, not an agent loop.

## Audited realities

1. `agent_tool_facades` owns an exact Agent→canonical Provider binding. `agent_facade_provider_config_update_guard` blocks changes to `plugin_id`, `config_json`, and `plugin_installation_id`, but overlooks `config_version`. `ClaimToolFacade` also validates the plugin ID and config AgentID but does not assert config version. Sandbox managed suites correctly include config version. The cross-context SQLite contract is an intentional exception to bounded-context repository independence: Destroy/retire must serialize with reference acquisition in one SQLite writer transaction.
2. `DefaultIsolation(true)` chooses `restricted_host` for Workspace and Tunnel but `host` for **all** external Provider plugins. This was deliberate in ADR-0033 for compatibility, but silently gives newly installed native extensions OS-user-level access even on supported restricted hosts. A plugin package checksum pins the code identity, not its filesystem authority. `restricted_host` is a filesystem restriction, **not** a VM/container boundary or network-egress filter.
3. `bootstrap/runtime.go`, `desktop/binding/service.go` and the Providers React page have grown large, with mixing of configuration conversion, presentation and side-effect sequencing. Wide API rewrites, identity migrations and new frameworks would increase risk more than reduce it.

## Bounded decisions

### 1. Resource-binding persistence contract

- Bump Agent SQLite schema to v5. Upgrade the existing immutable Provider config trigger **transactionally** so a claimed Agent facade rejects updates to `config_version` as well as PluginID, InstallationID and ConfigJSON. Keep revision/health metadata updateable so safe reload/inspection remains possible. Agent facade claim verifies exact built-in config version 1. No mutation of existing Agent/Provider/Tool IDs or profile grants.
- Cross-context SQL constraints live at explicit Runtime-composition/persistence seams or owning SQLite migration boundaries, not domain layer imports. They must remain installed after Runtime restart and on upgraded DBs. Test both standalone SQLite and two Runtime/connection behavior, rejected mutating operations, positive metadata-only updates, and failed-upgrade rollback. No permissive compatibility bridge.
- Do **not** blindly split the shared physical database. Atomicity of mixed Provider/Agent/Profile/Sandbox references requires a single writer and commit-point guards.

### 2. External native plugin process authority

- For a **fresh Runtime settings row only**, choose `external_provider=restricted_host` if and only if the host's verified restricted-host backend is available. Otherwise retain the explicit `host` mode and describe its risk. This is the same supported-platform behavior as Workspace/Tunnel. A selected `restricted_host` fails closed; never degrade silently.
- Preserve persisted `host` settings on upgrades/reopen: users may have installed native extensions dependent on host filesystem access. No mass setting migration, retroactive process termination, automatic activation, secret resolution or plugin restart.
- Show the currently persisted `host` authority **in the dedicated isolation viewport**, including a specific native Provider risk disclosure; changing the external Provider setting from `restricted_host` to `host` requires deliberate checkbox confirmation before the existing save operation. Merely opening/closing the view or saving unchanged settings does not mutate permissions. All strings have English + Chinese coverage. API/CLI remain host-owner-governed, retain current contract and can explicitly select host.
- Runtime process isolation remains host-owned; plugin manifest/Tool arguments cannot request weaker mode. This closes the default/UI-governance risk, **not** a claim of complete sandbox isolation or safe execution of hostile native binaries.

### 3. Complexity containment

- Extract a coherent pure Provider configuration adaptation surface from the oversized React Providers page, with unit tests for MCP/OpenAPI/Model Tools and generic dispatch. Keep unchanged ProviderID/ToolID, form semantics and i18n error strings.
- Extract a bounded Runtime composition constructor/factory, preserving the Invocation dependency order and cleanup semantics. Avoid speculative framework, large UI redesign, or moving existing Wails exposed methods/contract.
- Document cross-context dependency ownership and component file roles. Ensure differences are reviewable and proven by existing strict build/test checks.

## Gates

1. Forward-only migration v4→v5 and fresh schema v5; old binaries fail closed on newer schema; canonical identities stable; claimed Agent config_version changes rejected, non-authority metadata allowed; duplicate startup idempotent.
2. Fresh restricted capable → external plugin restricted; unavailable → host; persisted explicit host remains host; unavailability under selected restricted still prevents launch. UI warns in focused viewport and refuses unacknowledged host escalation; labels localized.
3. Extracted code behavior is test-equivalent for Provider forms, runtime composition and no leaked secrets.
4. `pnpm check`, `pnpm security:check`, `git diff --check`, typecheck, Vitest, Go Race, generator parity, clean tracked tree.
5. No production process, actual Rocky, production database, plugin installation, credential or Profile grants touched during implementation. Deployment and live smoke require a separately started new binary.

## Non-goals

No brand-new plugin permission protocol, per-plugin OS sandbox policy, networking firewall, agent loop, browser backend, general Provider migration, mass UI rewrite or removing a still-supported host override.

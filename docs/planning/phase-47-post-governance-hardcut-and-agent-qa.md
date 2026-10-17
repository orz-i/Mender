# Phase 47 — Post-governance hard-cut cleanup and real Agent acceptance

Status: **B1 hard-cut cleanup and canonical quality gate accepted** (2026-10-17); full Phase 47 real dual-Agent acceptance pending Mender tunnel re-connection. Phase 48 owner approvals closed separately. No production SQLite data was deleted. Authority: ADR-0052, ADR-0053 and Phase 46–48 accepted behavior.

## Goal and non-negotiable boundary

Remove provably dead code, obsolete runtime compatibility branches, redundant implementations, and *verified orphaned or incompatible* historical state following the Phase 43–46 Agent governance hard cut. Keep Mender's job as a tool/resource aggregator, governance plane and supplier to independently running Agents. No Mender-owned Agent Loop.

**Protect production:** at this Phase's initial inventory, a Wails process owned `~/Library/Application Support/Mender/mender.db` with Agent v11, 3 existing Agents, 13 historical Tasks, 2 AgentGrants and zero Tool placements. Phase 48 independently upgraded the active production schema to v12 and more Task history was recorded. The latest Phase 47 B1 read-only check confirms Agent v12, 3 Agents, 17 historical Tasks, zero unattended permission policies/Tool placements, `quick_check=ok` and no foreign-key violations. These facts do not authorize bulk deletion. Historical schema-migration receipts and old Task history are **not** intrinsically incompatible data. Preserve all Agent/Provider/Tool/Profile/Sandbox/Task IDs and grants unless a row-by-row owner-reviewed deletion manifest identifies an actual invalid record. No clearing SQLite tables just to make the runtime pass. Create a private consistent backup and before/after row-count/fingerprint ledger before any production data change. Never copy or disclose secrets.

**Protect code and UI:** preserve `.agents/`, `.pi/`, `.intent-loop/`, `.commandcode/`; don't stage them. DDD/Clean Architecture, canonical Policy/Invocation/Sandbox/Task boundaries, owner-only controls, fail-closed ACP, no Host fallback, no broadened Seatbelt, no new compatibility bridge. Keep migration support if needed by a valid clean install or recovery; removing it requires an explicit new baseline + fixtures and a hard validation gate, not a grep-based guess.

## A — Read-only evidence / real delegation

1. Record git status, schema versions, current joins and integrity, stale references, runtime call sites, candidate files and tests.
2. Distribute two **real** bounded code-review tasks using Mender-exposed OpenCode and Grok Agent `delegate → status/result`, after verifying provider→AgentID bindings. Existing registrations are explicitly `host` and **do not imply** Phase 46's restricted governed-tool assurance. Such tasks are analysis-only, never authorize code/shell writes. If ACP asks for native access and the owner doesn't approve, record failure. Never promote those to governed execution by automatic Host fallback.
3. Independently verify findings in the local repository; Agent prose is evidence of a delegation result, not proof of file modification.

## B — Hard-cut implementation (only proven candidates)

Document each removal with: symbol/path, old purpose, modern owner, all references, migration/test dependency, expected behavior change and rollback/test evidence. Perform targeted, small code edits, do not add adapters or retain two paths. Scope any data deletion to explicit confirmed invalid rows and backup/foreign-key validation. No silent retirement of legitimate historical Tasks.

### Evidence-backed B1: retire Provider-v1 plugin installation importer

- Native, read-only inspection of **both active Mender SQLite locations** (`~/Library/Application Support/Mender/mender.db` and `~/.mender/mender.db`) found modern `plugin_installations` (0 rows each), **no** `provider_plugin_installations` and **no** `provider_plugin_installation_schema_migrations`, and `PRAGMA quick_check=ok`. This does not authorize deleting archived backups. Production also has 0 foreign-key violations.
- Consistent private snapshot: `/tmp/mender-phase47-audit/production-readonly-20261017.db`, mode 0600; 58 user tables counted and SHA-256 fingerprints saved in a private `protected-ledger.json`; do not copy this DB or its credential metadata into the repository.
- Proven old runtime-only paths: `sqlite/store.go::Open → backupLegacy`; `sqlite/migrations.go::backupLegacy/transferLegacyProviderInstallations`; `sqlite/legacy_descriptor.go::decodeLegacyProviderDescriptor`; `sqlite/legacy/manifest.go::Parse`. Only the one-time Provider v1→generic conversion uses these decoders. Modern plugin installation store and Sandbox/Tunnel pin triggers remain.
- **Decision:** remove the legacy import/backup/decoder and its positive migration tests. If *either* old Provider installation table **or** its old migration marker exists, `Open` must reject that SQLite file with a stable incompatible-schema explanation, **without** deleting/migrating rows, creating snapshots or touching the modern inventory. The modern new DB and already-migrated v1 generic inventory continue to open idempotently, retaining exact installation identities and Sandbox/Tunnel pin guards.
- Actual Grok delegation: `atask_6ad7d4457ab4130dd6625dfdea0684d9` **completed** a bounded technical review of the supplied real source facts and approved this fail-closed hard cut under strict no-data-loss tests. Its earlier direct-source read request `atask_5aaecc77c288e45b5b1352450465d14b` **failed**. The no-tool Grok health Task `atask_f5e908e6c08d8457c9b04adf8e2b8e4a` **completed**. Do not claim Grok examined local files directly.
- Explicitly retained: Agent schema v1–11 migration code (legitimate forward upgrades), history and grants; provider-config v4 hard-cut checks; current external plugin integrity/extension verification and plugin management; all modern Sandbox/Tunnel installation references.

### B1 implementation and in-place evidence — 2026-10-17

- **Commit:** `dc90163 refactor(plugin): hard-cut retired Provider-v1 installation importer`. Removed the startup `backupLegacy`/automatic `transferLegacyProviderInstallations` path and its one-use Provider-v1 manifest/descriptor parsing package. The checked patch changed 7 tracked Go files, **176 lines added / 838 deleted**; no new bridge, no hidden fallback or unreviewed data migration.
- **Fail closed:** a legacy installation data table, a standalone legacy schema marker, both together, or mixed modern+old schema now produces an actionable incompatible-schema error and leaves the old rows intact. The mixed-schema test preserves a real modern enabled installation with its exact identity/status. Fresh modern initialization, reopen idempotency, future version rejection and no obsolete automatic backups all have focused regression tests.
- **Modern invariants:** direct bootstrap integration tests `TestExternalSandboxPluginGovernedLifecycleAndExecution`, `TestNativeTunnelTransportManagedMCPAndReferences`, `TestCanonicalCatalogueInstalledThreeKindsAndUsage`, `TestCanonicalPluginCatalogueAllTypesInertAndStable` passed. Sandbox/Tunnel installation reference and SQL triggers remain intact; `pnpm go:lint` returned zero issues.
- **User data proof:** both observed Mender SQLite locations have **zero** old Provider-v1 installation tables and a current modern plugin-installation inventory. Live production SQLite has schema v12 with 3 Agents, 17 preserved Tasks, zero owner auto-authorization settings, zero installation rows and `quick_check=ok`. An isolated current-production-consistent SQLite backup was opened by the newly built CLI: all **60 original table row counts and SHA-256 fingerprints were identical** before/after, with no FK errors. Only the private clone was writable; not one production SQLite row was deleted.
- **Orphan check:** eleven read-only referential checks across Agent→Task, ACP approvals, AgentGrants/Profiles, workspace, facades/providers, provider configs, Profile/Tool bindings and installed Sandbox/Tunnel packages found **zero** orphan references. Consequently no historical data removal was authorized. `agent_schema_migrations` receipts are upgrade history, not obsolete entities.
- **Reviewed and retained:** v1–v12 Agent forward migrations (including strict v9 workspace upgrade), and `localfs`'s pinned legacy digest preamble for verification of previously converted *already-installed* packages. Deleting them without a new exhaustive schema and artifact contract would break legitimate users. The owner-invoked Sandbox tool-suite adoption path remains useful for current explicit owner inventory repairs; not an automatic compatibility bridge.
- **Real delegation accounting:** Mender actually received OpenCode `atask_491dc20b3c0a7bb36cb022cd92bb12b2` and Grok `atask_5aaecc77c288e45b5b1352450465d14b` read-only repository audit requests earlier in this phase; both ultimately **failed** (no claim of successful repo review). Grok `atask_6ad7d4457ab4130dd6625dfdea0684d9` **completed** a scoped analysis from supplied verified source facts; this is a real Mender-delegated task but **not** local file access. The benign Grok health check `atask_f5e908e6c08d8457c9b04adf8e2b8e4a` completed. Current Mender tunnel is not connected (tool reports last poll older than 300 seconds), so a second actual parallel OpenCode/Grok audit cannot be reported as done; needs user reconnect/eligible scoped authorization.

## C — Acceptance

### Canonical B1 quality and production-safe rehearsal — 2026-10-17

- **Final canonical `pnpm check`: exit 0**. Receipt: `/tmp/mender-phase47-hardcut-b1-final/check.exit`, full log: `/tmp/mender-phase47-hardcut-b1-final/check.log`. Included full backend Go tests and Go Race Detector, Go lint (0 issues), plugin SDK/examples/protocol checks, Wails bindings and desktop architecture, React/TypeScript/ESLint, i18n **1309/1309**, Vitest **46 files / 268 tests**, and Web/Desktop production builds. The native Wails Go app also compiled separately. The existing macOS linker deployment-target warnings and Vite bundle-size warning are non-blocking.
- **Production copy opened by real new CLI**: used the current SQLite database's consistent, mode-0600 copy at `/tmp/mender-phase47-hardcut-b1-final/production-modern-plugin-regression.db` and newly built CLI to execute the full Runtime open path. SHA-256 and row counts across all **60 existing business tables** were identical before and after. `plugin_installations=0`, `quick_check=ok`, FK violations 0. No production SQLite, AgentGrant, plugin installation, Profile, Task or workspace data was modified.
- **Scope of cleanup accepted**: retired Provider-v1 installation import/backup/descriptor conversion removed in `dc90163`; all modern install, Sandbox/Tunnel/pin and Agent runtime boundaries unchanged. No orphaned/incompatible user rows were identified and therefore **zero historical rows deleted**. Old Provider-v1 tables will now trigger explicit startup incompatibility rather than silent migration, with original table contents retained intact.
- **Not accepted as complete real multi-Agent acceptance**: the original OpenCode and Grok read-repository delegated Tasks both ultimately failed. Grok did complete an additional factual review solely from supplied source evidence, not direct file access. Currently neither a local production Wails nor its external Mender tunnel process is observed, and `mender_search_tools` reports that tunnel-client has not polled recently; new real OpenCode+Grok re-delegations were not possible. Do not mark this functional validation passed until the owner reconnects the tunnel, sets a scoped approval mode, and independently checks new TaskIDs, requested ACP decisions, results and audit. No Host auto-trust, credentials or sandbox permissions were modified by this cleanup.

- Stable Tool/Provider/Agent/Profile identities; 4 delegated Agent facade Tools per Agent; no new global tools from cleanup.
- Run focused regression first, then canonical `pnpm check` including Go Race Detector, frontend, i18n and Wails binding/architecture parity.
- Real scenario: both Agent delegations produce observable Task IDs and terminal outcomes or precisely classified fail-closed errors; optionally exercise positive restricted OpenCode disposable Read/Edit using opt-in test, and negative Grok restricted path, without changing the user's source tree.
- State what was cleaned, what remains required, what is unsupported, production DB side effects, and exact commits. Close this phase without an expanding stage taxonomy.

## Live inventory at entry

- Branch `main`; HEAD `e1aad3d`, ahead of `origin/main`; only Skill/agent environment directories untracked.
- Wails dev process and tunnel-client running after user restart.
- Production SQLite Agent v11, 3 Agents (Codex/Grok/OpenCode, all configured in explicit Host mode), 13 Tasks, 2 AgentGrants, 0 placements, 1 additional Agent workspace. Current profile count 1; Provider count 11; Catalog Tool count 37. No foreign-key violations or SQLite corruption.
- Mender profile exposes **two real Agent delegation Tool suites**; provider `prov_0ee3ba44c2f8f3d3d444a4b9001ff9b4` targets OpenCode and `prov_1633739d42de01f3d438ce26b2e9f99c` targets Grok. Use their exact existing delegate/status/result Tools.
- Agent schema migration receipts include versions 1..11. They are compatible history, not proof of orphaned live data.

## Checkpoints

- [x] Documentation and read-only manifest committed
- [x] OpenCode/Grok original real delegation TaskIDs and failed/positive outcomes recorded; new dual-Agent repository pass blocked on tunnel reconnect
- [x] Targeted obsolete importer removal with regression proof; no invalid user rows found or deleted
- [x] Native/quality and schema integrity validation for B1 passed
- [x] B1 final scope and residual limitations recorded; Phase 47 dual-Agent re-delegation remains pending a live Mender tunnel

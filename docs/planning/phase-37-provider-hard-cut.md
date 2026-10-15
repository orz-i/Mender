# Phase 37 — hard-cut deprecated MCP and Workspace compatibility

- Status: **CODE + LIVE PROVIDER HARD-CUT ACCEPTED** (2026-10-15). Developer Wails restart, production schema v4 migration and data-integrity smoke passed. Pre-existing SSH Job observation returned unknown, tracked separately; packaged release remains untested.
- Decision: [ADR-0043](../architecture/adr/0043-hard-retire-provider-compatibility.md)
- User outcome: “继续收敛边界，硬切的方式，清理数据，清理代码”.
- Commit docs first, bounded commits thereafter. Preserve `.agents/`, `.pi/`, `.intent-loop/` outside Git.

## Work clusters / done checks

- **M1 — SQLite single source:** schema v4 validates canonical coverage, drops legacy MCP/marker tables atomically; invalid legacy mapping or Workspace v1 blocks upgrade with no partial deletion. Unit/fixture tests.
- **M2 — runtime hard cut:** delete import/double-write and legacy Workspace start migrator, fully remove retired Go adapter/configstore; generic Provider registration, activation, governed deletion and refs remain.
- **M3 — transport/API hard cut:** remove legacy HTTP/Wails routes/methods/DTOs and generated SDK shapes; only generic typed Provider `config` accepted. Update callers/tests. Tests reject deleted endpoints and preserve canonical MCP HTTP/stdio behavior.
- **M4 — close:** full checks, security, clean tracked tree, data upgrade rehearsal on **isolated copied snapshot only**, no app interruption, no live data mutation, document actual before/after and next restart requirements.

## Safety

During implementation, the prior Mender process continued using the old dual-store runtime; no live DB was touched by development tests. The operator has since restarted a new Runtime, which performed the validated one-way migration. Do not erase historical backups, active credentials, Profiles, Agent history or SSH obligations. Reject missing canonical data instead of reimporting legacy configuration.

## Implementation and closure evidence

### Commits and hard boundaries

- b0016e2 — document-first ADR-0043 and Phase 37 contract.
- 0f65a5d — hard-cut canonical Provider Config schema v4, retire obsolete MCP double store/imports and Workspace v1 starter migration, remove legacy HTTP/Wails/SDK Provider APIs, update Wails/OpenAPI generated types and regression fixtures.
- Removed ~3,245 lines and added ~572 lines in the central functional commit (net ~2,673 fewer lines). Wails exported methods reduced from 117 to 114. The direct MCP connection fixture remains test-only; production activation uses governed Provider plugins.
- Generic configured Provider API still handles built-in MCP (streamable-http and stdio), OpenAPI, Model Tools, Workspace and external package pins. Canonical ProviderID, ToolID, Profile, Policy, Credential, Agent, Sandbox, Tunnel and SSH Job identity semantics are unchanged.

### Migration gates and evidence

1. SQLite migration v4 verifies legacy MCP Provider rows map to well-formed canonical built-in MCP ConfigVersion 1 before dropping obsolete tables. It refuses missing/wrong-Plugin/wrong-version canonical coverage, invalid transport configuration or non-v2 Workspace configs, with transactional rollback.
2. Fresh Provider Config DB contains none of the old MCP/import tables. Successful v4 upgrade drops exactly mcp_provider_configs, mcp_config_schema_migrations and provider_config_imports, and installs two Workspace v2-only raw SQL guards. Reopening v4 is idempotent.
3. A temporary SQLite backup of the **currently live production database** was created in a private temporary location for a rehearsal. The new version's real configstore.Open migrated **that COPY only**. Pre-migration canonical record counts (Providers/Tools/Profile ToolBindings/Agent Tasks/Provider Configs) were 6/25/21/2/6; after migration the counts were exactly 6/25/21/2/6. Schema advanced 3→4, legacy MCP rows 1→table removed, all 3 retired tables were absent, both guards present and PRAGMA integrity_check=ok. The isolated snapshot and ephemeral test program were cleaned up. **No live database was opened for writes or altered by this rehearsal.**
4. Regressions cover that stale POST /providers/http and /providers/stdio no longer create Providers, flat PUT without typed config returns 400, and canonical Provider create/update/delete continues to work for both MCP transports. Workspace v1 startup now fails closed and preserves Tool/Profile rows rather than inventing a Sandbox or remapping ToolIDs.

### Verification

- go test ./backend/... — pass.
- go test -race ./backend/... — pass, including SSH Jobs, Sandbox, Agent, Profile, Provider and management API.
- pnpm go:lint — pass, 0 issues.
- pnpm lint, pnpm typecheck — pass.
- pnpm test — pass, 35 Vitest files / 210 tests; i18n 1207/1207.
- pnpm build — pass, both Web and Desktop bundles; Vite still emits nonblocking large-chunk warnings.
- Provider/Sandbox/Tunnel Protocol Buf generation and SDK/example lint, test/race/build checks — pass.
- pnpm go:modules:check, pnpm sdk:check, pnpm desktop:bindings:check, pnpm desktop:architecture:check — pass; generated types match the new hard-cut OpenAPI/Wails contracts.
- pnpm security:check — pass. No reached Go vulnerability symbols; one required but unused dependency module remains flagged in metadata. Production JavaScript dependency audit: no known vulnerabilities.
- The aggregate pnpm check remote call returned no reliable terminal result; its individual constituent gates were executed and passed. This documentation intentionally does not claim a green aggregate command.
- git diff --check — pass; all tracked source changes committed, skill folders .agents, .pi, .intent-loop preserved untracked.

### Rollout boundary

- **Requires operator rebuild/restart with new Runtime**. The running older Wails binary is still using the previous schema v3; its currently open production DB retains old tables until the new Runtime migrates it on a clean start. Do not concurrently run old and new binaries against the same DB.
- After stopping the current Runtime, take an exact retained pre-upgrade SQLite backup. The first new startup removes only the validated obsolete tables and records Provider Config schema v4. Old binaries fail closed after v4; downgrades require restoring that backup.
- No live provider/credential/profile/SSH job data, historical backups or remote TaskIDs were deleted by this implementation turn. Logical DROP TABLE does not guarantee forensic erasure from SQLite freed pages or existing backups.
- The operator has now rebuilt/restarted; the verified post-restart read-only result is recorded below. Third-party plugin activation, packaged Wails and disruptive remote SSH tests remain outside this acceptance.

### Post-restart Phase 37 live read-only acceptance (2026-10-15)

After the operator rebuilt/restarted and refreshed the connector, Mac Pi observed a new Wails development Desktop process (PID 70520, launched at 17:39:17 Mac local time) and its child OpenAI tunnel-client (PID 71013, launched at 17:40:08). lsof confirmed that this exact Mender Desktop process has the live `~/Library/Application Support/Mender/mender.db` open.

**Read-only production database results:**

- `PRAGMA integrity_check = ok`; Provider Config schema **4** (v4 migration row present), Agent **5**, Profile **5**, Sandbox **18**.
- All three retired tables (`mcp_provider_configs`, `mcp_config_schema_migrations`, `provider_config_imports`) absent; legacy table/view count **0**.
- Both Workspace v2-only SQL guards installed; all seven P1 Provider/Profile/Policy reference guards present; Agent Facade immutable configuration guard still pins `config_version`.
- Canonical inventory matches the pre-restart snapshot: **6 Providers / 25 Tools / 21 Profile ToolBindings / 2 Agent Tasks / 6 Provider configs**. There is still one canonical `io.mender.provider.mcp` Provider Config v1 and two canonical Workspace v2 configs. No Workspace v1 configs.
- No missing/orphan Provider configs, dangling Profile ToolBindings, dangling Policy ProviderIDs/ToolIDs, or invalid Provider config JSON.
- Persisted isolation choices remain `workspace_shell=restricted_host`, `external_provider=host`, `tunnel_client=restricted_host`; no silent permission change or restart-side reauthorization.

**Connected Mender smoke:**

- ChatGPT's refreshed Mender MCP connection successfully discovered SSH Jobs status and Workspace read/search Tools, demonstrating Tool catalogue exposure and connector reachability after the hard cut.
- **Separate SSH Job evidence caveat:** the previously observed terminal task `sjob_1618d76ba93daa0c1dcbfd26f6a13532` now returns `{state: "unknown", cancelRequested: false}` from **both** `jobs.status` (repeated) and `jobs.result`. Its provider still binds to a `ready` SSH Sandbox. This is not evidence that the old command ran again or failed, but it is **not** a successful remote terminal-result smoke. V2 correctly refuses to assert completion without verifiable remote evidence. The previous `completed/exitCode=0` observation must not be repeated as a current fact. Remote availability/evidence should be investigated separately before claiming SSH Jobs end-to-end recovery is healthy.

**Acceptance verdict:** Phase 37's **Provider hard-cut migration, data preservation, elimination of duplicate MCP state and Profile tool discovery pass on the real restarted developer Runtime**. SSH remote job terminal observation remains **unverified/unknown**; packaged Wails and third-party native plugin compatibility remain outside this acceptance. These checks made zero production database writes, no changes to Provider/Policy/Secret/SSH state and did not launch/replay remote commands. Documentation-only changes are committed separately.

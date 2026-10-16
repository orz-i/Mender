# Marketplace post-rebuild acceptance and residual retirement — 2026-10-16

Status: Automated/API/isolated-browser acceptance closed (2026-10-16). Packaged Wails window still requires operator verification after rebuilding the new changes.
Context: ADR-0045, Phase 38

## Intent

Accept the newly rebuilt Mender Marketplace/Provider management on a running Desktop and a disposable local runtime. Remove verified historical entry points, duplicate product projections, orphaned code and disposable test residue without making a second Provider facade path or a permanent migration compatibility bridge.

Mender remains the LLM/Agent Tool/Resource supply and governance platform; it does not implement an Agent loop.

## Observed baseline (read-only)

- Wails dev Desktop process is running with the existing SQLite database under ~/Library/Application Support/Mender/mender.db. PRAGMA integrity_check is ok, PRAGMA foreign_key_check reports zero violations.
- Live inventory counts: 6 Providers (2 Agent Delegation, 2 Workspace, 1 SSH Jobs, 1 MCP), 25 Tools, 6 Sandboxes, 2 Agents, 1 Tunnel, 1 Profile, 21 Profile Tool bindings, 2 Credential references, and zero native Plugin installations. These are **user-owned canonical records, not debris**.
- Historical provider_plugin_installations, mcp_provider_configs and other retired compatibility tables are absent. There are multiple nonidentical, dated pre-upgrade SQLite backups; they are recovery assets rather than automatically disposable duplicates.
- Plugin Market UI and Provider direct links are filtered, but ListAvailableProviderPluginCatalog still exports private Agent/Workspace descriptors and the generic backend RegisterConfiguredProvider rejects only the private Jobs facade. CLI still offers provider add workspace. These are real second entry points which can bypass resource-owned workflows.
- Historical one-way upgrade migrations still exist in startup code. They must not be confused with a live dual-read compatibility bridge; retire only after a supported-source/version policy and regression evidence.

## Acceptance and safety contract

1. **Runtime correctness** — on isolated test state verify plugin discovery exposes MCP/OpenAPI/Model Tools; reviewed AnySearch connects via existing MCP; Tool Suite/internal providers/E2B are not market offerings; installed plugin management remains distinct; exact selected creation forms; unavailable items do not advertise creation. Verify Wails process liveness and UI where native/browser access permits. No live provider connection, paid Agent task, tunnel start or remote Sandbox provision in acceptance.
2. **One owner per managed facade** — no generic HTTP, Wails, CLI or exported Runtime path may create Agent Delegation, Workspace or SSH Jobs Providers. Agent/Sandbox Tool Suite owner-only ensure/adopt/retire workflows must continue working, preserving existing ProviderID/ToolID/SourceKey and Profile/Policy bindings. Keep technical plugin descriptions available to internal validation, diagnostics, migration and existing-resource projection without making them selectable in user Provider creation.
3. **Safe history cleanup** — inspect real DB using read-only integrity, FK, orphan-reference and historical-schema probes. Never delete active Providers, Sandboxes, Tools, Jobs, Profile, Credentials, Tunnel, audit history or user-controlled content. Do not reset/reseed databases. If old backup files are reorganized, preserve all original bytes, file permissions and per-file hashes in a recoverable archive; don't delete distinct recovery snapshots just because they are old.
4. **Code retirement** — remove demonstrably dead UI/CLI configuration branches, obsolete helper logic and now-inapplicable tests/docs; preserve canonical one-way migrations that still implement recognized upgrade paths unless tests prove true removal is safe. No dead wrapper/compat adapter added to bridge cut behavior.
5. **Verification & commit discipline** — document first; isolated commits for host admission, user entry points and residual cleanup, then tests and acceptance evidence. Run focused Go/TS/UI tests, API invariants, race/lint/build/security gates where available. Git staging must exclude .agents/, .pi/, .intent-loop/ and real runtime SQLite/credentials. Any unverified Wails click path is explicitly reported rather than claiming success.

## Work packages

- **A — Contract & inventory:** record inspected running process, DB references, cleanup criteria, rollback/backup policy; commit docs only.
- **B — Host hard admission:** seal private Provider facade identities at the Runtime/API boundary; retain only resource-owner operations; make user-selectable Provider inventory independent of the internal runtime descriptor registry. Add negative admission and positive suite regression tests.
- **C — Dead surface retirement:** remove CLI generic Workspace Provider creation and residual frontend paths/unused helpers; keep direct MCP/OpenAPI/Model Tools and exact native installation semantics; adjust docs/tests. No duplicate inventories.
- **D — Live acceptance/data hygiene:** run isolated UI/API acceptance against current build, audit production DB read-only for orphans and migration debris, only archive safely identifiable backup clutter if needed, run quality gates, commit evidence and summarize real limitations.

## Evidence and closure

### Bounded commits (in implementation order)

- adbea60: document-first real-state audit and deletion safeguards.
- 6374198: host admissions disallow generic Agent Delegation, Workspace and SSH Jobs Provider creation/update, while preserving exact managed resources and technical catalogue.
- 8fbc491: retire legacy provider add workspace CLI, obsolete Workspace Provider form/selector, duplicate Sandbox loading, dead test and translations; use sandbox tools <id> --prepare instead.
- 130b869: reject generic managed Provider synchronization while keeping resource-owner synchronization and activation governance.
- b6a94aa: HTTP conflict regression, stricter unknown-plugin-kind checks, remove unused market display branches and translations.

### Real user database — read-only audit

- Application database in ~/Library/Application Support/Mender/mender.db passed PRAGMA integrity_check (ok) and foreign_key_check (zero violations).
- Preserved 6 Providers, 25 Tools, 6 Sandboxes, 2 Agents, 1 Tunnel, 1 Profile, 21 Profile ToolBindings and 2 Credentials. None were reset, deleted, provisioned or reassigned. Three SSH job obligations remain protected.
- Provider/config, Provider/tool, Profile/tool, Agent/facade, Sandbox/facade and Sandbox/run orphan checks returned zero.
- Retired provider_plugin_installations, mcp_provider_configs, mcp_config_schema_migrations and provider_config_imports tables are absent.
- Ten distinct, valid root-level historical SQLite migration snapshots (6,422,528 bytes) were MOVED, not deleted, into ~/Library/Application Support/Mender/backups/legacy-migrations/. The restricted-permission archive-manifest.json records original paths, sizes and SHA-256 hashes. Every archived hash was rechecked. Four existing manually named backups were not touched. Root-level migration backups now number zero.
- One-way forward migrations were deliberately preserved; they are still needed for supported upgrades and do not constitute a runtime dual-read compatibility bridge.

### Live acceptance and isolation

- Real Wails dev process remains running. Only read-only checks were performed against its persistent database.
- A separate freshly compiled backend with temporary SQLite and loopback API returned ready. It accurately reported MCP/OpenAPI/Model Tools as creatable, internal Agent/Workspace/Jobs facades as action none, unavailable backends as unavailable and no E2B catalogue entry.
- An isolated Chromium browser against this temporary runtime passed Marketplace Discover for eight actionable items (Grok Agent, Local Workspace, MCP, Model Tools, OpenAI Tunnel, OpenAPI, SSH Remote, AnySearch). It excluded private facades, planned E2B and unavailable drivers. The three Provider create buttons opened their distinct focused forms; reviewed AnySearch Connect prepopulated the expected MCP endpoint; Installed plugins correctly showed zero native installations. No JavaScript exceptions observed. No form was submitted.
- Native Pi agent-browser had a pre-existing incompatible legacy journal, so packaged Wails window clicking was NOT claimed. After acceptance, isolated QA listeners and the temporary Vite config were cleaned up without terminating the Wails or other user servers.

### Verification

- Full relevant Bootstrap, HTTP, Desktop Binding and CLI Go packages: pass.
- go test -race ./backend/internal/bootstrap ./backend/internal/management/httpapi ./backend/internal/cli: pass.
- Go lint: zero issues; OpenAPI SDK generation, Desktop Wails bindings and architecture drift checks: pass.
- pnpm lint, pnpm typecheck, pnpm test: pass; 39 Vitest files, 219 tests, 1208/1208 Chinese translations.
- pnpm build: pass for Web and Desktop frontend; pre-existing JavaScript chunk size warnings remain.
- pnpm security:check: pass; zero reachable Go vulnerabilities, one uncalled transitive module finding, no known production JavaScript vulnerabilities.
- The monolithic pnpm check request was interrupted during all-package Go Race and never returned a final status. All regular backend Go tests completed without reported failure; affected packages passed a separately run Race test, and remaining frontend checks were individually rerun and passed. Do not report the interrupted monolithic command as successful.
- No real remote SSH/Agent task, chargeable sandbox, tunnel or Credential used for acceptance.

### Operational follow-up

The user rebuilt Wails before this cleanup. The newer implementation commits require a fresh Wails rebuild/restart to take effect in that native process, followed by optional operator window verification. Runtime data did not require a schema reset or migration. The tracked Git tree must remain clean; .agents/, .pi/ and .intent-loop/ are excluded from commits.

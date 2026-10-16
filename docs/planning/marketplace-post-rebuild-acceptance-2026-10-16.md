# Marketplace post-rebuild acceptance and residual retirement — 2026-10-16

Status: In progress
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

## Evidence

(To be populated with executed commands, exact commits, checks and any unverified environment constraints.)

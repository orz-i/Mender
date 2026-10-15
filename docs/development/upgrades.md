# Upgrade and Downgrade

Mender uses forward-only, context-owned SQLite migrations.

Catalog, Audit, Profiles, canonical Provider plugin configuration, Agent/Sandbox resources, and Runtime governance share the configured SQLite database file with context-owned migration versions. The legacy MCP configuration store has been retired; there is only one runtime Provider configuration authority.

## Migration ownership

Current migration tables are:

- `catalog_schema_migrations`,
- `audit_schema_migrations`,
- `profile_schema_migrations`,
- `provider_config_schema_migrations`,
- `provider_plugin_installation_schema_migrations`,
- `agent_schema_migrations`,
- `sandbox_schema_migrations`,
- `runtime_governance_schema_migrations`.

The retired `mcp_config_schema_migrations` and `provider_config_imports` tables are **not** recreated.

Each bounded context applies only its own migrations.

Migration execution is transactional.

The canonical Provider plugin configuration schema is **version 4**. Version 2 added nullable `plugin_installation_id` to `provider_configs`; external Provider rows require an exact Installation ID so package upgrades cannot silently retarget a configured Provider. Version 3 added nullable `credential_refs_json` and `connection_summary` host projections. New/updated Provider configs persist normalized Credential References and bounded connection summary metadata from `InspectConfig` alongside the exact config revision; no resolved secret bytes are stored. A null Credential metadata column on a migrated older row means “not yet normalized”, not “verified empty”. Existing Provider rows retain ProviderID, PluginID, config version/revision, plugin-owned JSON, ToolIDs, Profile bindings, Policy targets, and Audit identity.

The external Provider plugin installation schema is version 2. Version 1 stored non-secret installation identity and package metadata including PluginID, package version, source kind/reference, content digest, manifest JSON, administrative status, and installation timestamp. Version 2 adds nullable `verified_descriptor_json`, a bounded snapshot of the runtime descriptor verified during installation. Normal plugin catalog reads use this snapshot instead of starting third-party processes. Legacy version-1 rows may be verified/backfilled lazily. Plugin executable/package files remain in the managed plugin root rather than SQLite.

Phase 14 OpenAPI and Phase 15 Model Tools still use the generic Provider table as built-in plugins. Phase 16 introduced exact external InstallationID pinning; Phase 17 adds offline host-governed Provider metadata and verified installation descriptors without changing canonical Provider/Tool identity.

Downgrading to a binary that supports only provider-config schema version 2 fails closed during Provider config store startup after version 3 has been recorded; binaries that understand only plugin-installation schema version 1 likewise fail closed after installation schema version 2 is recorded. Restore the pre-upgrade database backup before running such an older binary; do not delete migration rows or rewrite/null new columns to force a downgrade. A binary that understands current schemas but lacks a particular built-in or installed external plugin still fails closed when that Provider is activated rather than substituting another implementation.

**Provider Config version 4 is a destructive hard-cut upgrade.** It checks that every legacy `mcp_provider_configs` ProviderID has a valid canonical `io.mender.provider.mcp` Plugin configuration at version 1, and that no built-in Workspace Provider remains at ConfigVersion 1. It then atomically removes obsolete `mcp_provider_configs`, `mcp_config_schema_migrations`, and `provider_config_imports` tables, installing SQL guards against reintroducing Workspace v1. A missing/invalid canonical mapping or remaining Workspace v1 causes an error **before any legacy table is removed**. The Runtime no longer imports or dual-writes old MCP configs, nor interprets obsolete flat MCP HTTP/stdio Provider create/update API requests. Only typed plugin configs through generic Provider CRUD remain. An old binary that supports Provider Config schema 3 will fail closed after version 4 is recorded; use a verified backup for any downgrade.

The current Audit schema is version 2. Version 2 adds `error_summary TEXT NOT NULL DEFAULT ''`. Existing Audit rows migrate with an empty summary. New failed Invocations may store only the bounded/redacted diagnostic summary produced at the Invocation boundary; arguments and full result payloads remain absent from Audit storage.

## Before upgrading

For a release that adds a migration:

1. stop every Mender process using the database,
2. identify the exact database path,
3. create a backup copy while no process has the file open,
4. retain the previous Mender binary/package until the upgrade is verified,
5. review release notes for migration-specific recovery requirements.

CLI/daemon database resolution normally uses:

1. `--db`,
2. `MENDER_DB`,
3. `~/.mender/mender.db`.

Desktop defaults to:

```text
<UserConfigDir>/Mender/mender.db
```

and also supports `MENDER_DB` for explicit development/test control.

Because all Runtime-owned SQLite stores use the same database path, back up the database as one unit.

## Upgrade

After backup:

1. install the new signed/verified artifact,
2. start Mender,
3. let each context apply any known forward migration,
4. verify Runtime startup succeeds,
5. verify Provider and Profile state,
6. run a representative Tool search and, where appropriate, a controlled invocation,
7. retain the backup until the release is considered stable.

Migrations run during store startup. There is no separate hidden migration daemon.

## Downgrade

Mender does not run reverse migrations automatically.

Every SQLite store checks the highest recorded context schema version before applying migrations.

If an older binary sees a schema version newer than it understands, it fails closed during startup.

This means a downgrade has two possible paths:

- if no newer schema version was written, install/run the older binary normally;
- if the database was migrated beyond the older binary's supported version, stop Mender and restore the pre-upgrade database backup before starting the old binary.

Do not delete rows from a `*_schema_migrations` table merely to force an old binary to start. The underlying schema/data may already have changed.

## Destructive or complex migrations

A future migration that removes data, rewrites large tables, changes irreversible semantics, or performs long-running transformation requires before release:

- an explicit backup requirement,
- recovery instructions,
- realistic migration test fixtures,
- release-note callout,
- expected runtime/downtime guidance,
- validation that partially applied work rolls back transactionally or has an explicit recovery path.

## Release notes

Any release adding a migration must state:

- which context migration version changes,
- whether existing data is rewritten,
- whether downgrade requires restoring a backup,
- any expected operational delay or disk-space increase.

## No automatic updater

Phase 8 does not add an in-app auto-updater.

Artifact acquisition, signature/checksum verification, installation, and restart remain explicit operations. This keeps upgrade behavior auditable while signing and migration policy stabilize.

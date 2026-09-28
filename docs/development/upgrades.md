# Upgrade and Downgrade

Mender uses forward-only, context-owned SQLite migrations.

Catalog, Audit, Profiles, Provider plugin configuration, and the transitional MCP compatibility configuration share the configured SQLite database file but maintain separate schema migration tables.

## Migration ownership

Current migration tables are:

- `catalog_schema_migrations`,
- `audit_schema_migrations`,
- `profile_schema_migrations`,
- `provider_config_schema_migrations`,
- `provider_plugin_installation_schema_migrations`,
- `mcp_config_schema_migrations`.

Each bounded context applies only its own migrations.

Migration execution is transactional.

The current generic Provider plugin configuration schema is version 2. Version 2 adds nullable `plugin_installation_id` to `provider_configs`. Existing built-in Provider rows migrate with a null Installation ID and retain their ProviderID, PluginID, config version/revision, plugin-owned JSON, ToolIDs, Profile bindings, Policy targets, and Audit identity. External Provider rows require an exact Installation ID so package upgrades cannot silently retarget a configured Provider.

The external Provider plugin installation schema is version 1. `provider_plugin_installations` stores non-secret installation identity and package metadata including PluginID, package version, source kind/reference, content digest, manifest JSON, administrative status, and installation timestamp. Plugin executable/package files live in the managed plugin root rather than SQLite.

Phase 14 OpenAPI and Phase 15 Model Tools still use the generic Provider table as built-in plugins. Phase 16 is the first Provider-config schema migration since Phase 13 because exact external installation pinning requires a new persisted identity.

Downgrading to a binary that supports only provider-config schema version 1 now fails closed during Provider config store startup after version 2 has been recorded. Restore the pre-upgrade database backup before running that older binary; do not delete the migration row or null/rewrite installation IDs to force a downgrade. A binary that understands schema version 2 but lacks a particular built-in or installed external plugin also fails closed when that Provider is activated rather than substituting another implementation.

The legacy MCP Provider compatibility schema is version 4. Versions 1–3 contain the historical HTTP/stdio projection, including non-secret header templates and Credential References. Version 4 is an ownership sentinel with no additional secret-bearing columns: generic `provider_configs` is the Runtime source of truth, while the old MCP table is temporarily dual-written for first-party compatibility contracts. Pre-Phase-13 binaries that understand at most version 3 therefore fail closed after this migration instead of modifying the compatibility projection during an unsupported downgrade.

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

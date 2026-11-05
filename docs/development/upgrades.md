# Upgrade and Downgrade

Mender uses forward-only, context-owned SQLite migrations.

Catalog, Audit, Profiles, MCP delivery, canonical Provider configuration,
Agent/Sandbox resources and Runtime governance share the configured SQLite
file with context-owned migration versions.

## Migration ownership

Relevant migration tables include:

- `catalog_schema_migrations`,
- `audit_schema_migrations`,
- `profile_schema_migrations`,
- `mcp_delivery_schema_migrations`,
- `provider_config_schema_migrations`,
- `plugin_installation_schema_migrations`,
- `agent_schema_migrations`,
- `sandbox_schema_migrations`,
- `runtime_governance_schema_migrations`.

The retired `mcp_config_schema_migrations` and `provider_config_imports` tables are **not** recreated.

Migrations are transactional. The Profile-to-MCP delivery handoff uses a
single transaction for both version records and the historical conversion.

The canonical Provider plugin configuration schema is **version 4**. Version 2 added nullable `plugin_installation_id` to `provider_configs`; external Provider rows require an exact Installation ID so package upgrades cannot silently retarget a configured Provider. Version 3 added nullable `credential_refs_json` and `connection_summary` host projections. New/updated Provider configs persist normalized Credential References and bounded connection summary metadata from `InspectConfig` alongside the exact config revision; no resolved secret bytes are stored. A null Credential metadata column on a migrated older row means “not yet normalized”, not “verified empty”. Existing Provider rows retain ProviderID, PluginID, config version/revision, plugin-owned JSON, ToolIDs, Profile bindings, Policy targets, and Audit identity.

The generic plugin installation schema is version 1. It records package
identity, digest, administrative status and the verified descriptor snapshot.
Package files live in the managed plugin root. A Provider installation with a
missing snapshot remains unavailable in the catalog until explicitly verified
with `mender plugin verify <installation>`. Listing uses persisted metadata.
A database containing the retired Provider-only inventory requires offline
migration with a compatible prior release before current Runtime startup.

Phase 14 OpenAPI and Phase 15 Model Tools still use the generic Provider table as built-in plugins. Phase 16 introduced exact external InstallationID pinning; Phase 17 adds offline host-governed Provider metadata and verified installation descriptors without changing canonical Provider/Tool identity.

An older binary fails closed on unsupported versions. A binary that supports
the current schema but lacks a particular built-in or installed plugin still
fails closed when that Provider is activated.

**Provider Config version 4 is a destructive hard-cut upgrade.** It checks that every legacy `mcp_provider_configs` ProviderID has a valid canonical `io.mender.provider.mcp` Plugin configuration at version 1, and that no built-in Workspace Provider remains at ConfigVersion 1. It then atomically removes obsolete `mcp_provider_configs`, `mcp_config_schema_migrations`, and `provider_config_imports` tables, installing SQL guards against reintroducing Workspace v1. A missing/invalid canonical mapping or remaining Workspace v1 causes an error **before any legacy table is removed**. The Runtime no longer imports or dual-writes old MCP configs, nor interprets obsolete flat MCP HTTP/stdio Provider create/update API requests. Only typed plugin configs through generic Provider CRUD remain. An old binary that supports Provider Config schema 3 will fail closed after version 4 is recorded; use a verified backup for any downgrade.

The current Audit schema is version 2. Version 2 adds `error_summary TEXT NOT NULL DEFAULT ''`. Existing Audit rows migrate with an empty summary. New failed Invocations may store only the bounded/redacted diagnostic summary produced at the Invocation boundary; arguments and full result payloads remain absent from Audit storage.

**Profile schema 7 and MCP delivery schema 1** move mode, Tunnel association
and direct projection out of Profile. The transaction preserves effective
projection, Profile and Tool identities, selection revisions and Skill
deployment ownership. It does not start servers or rewrite managed artifacts.
Profile v6 binaries reject the migrated database. Before production rollout,
verify an upgrade on a restored copy with representative data. See
[ADR-0062](../architecture/adr/0062-r5-profile-delivery-schema-boundaries.md).

## Before upgrading

For a release that adds a migration:

1. stop every Mender process using the database,
2. identify the exact database path,
3. back up a consistent copy of the shared SQLite database, including any
   required WAL state, and the matching managed resource directories,
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

Restore the backup to a disposable location and verify it with the target
binary before upgrading the live database.

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
- if the database was migrated beyond the older binary's supported version,
  stop Mender and restore the matching pre-upgrade database and managed
  resources before starting the old binary. Reconcile any writes made after
  upgrade before restoring the backup.

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

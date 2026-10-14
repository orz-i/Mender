package sqlite

import (
	"context"
	"database/sql"
	"fmt"
	"time"
)

const latestSchemaVersion = 15

func migrate(ctx context.Context, db *sql.DB) error {
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin sandbox migrations: %w", err)
	}
	defer func() { _ = tx.Rollback() }()
	if _, err = tx.ExecContext(ctx, `CREATE TABLE IF NOT EXISTS sandbox_schema_migrations(version INTEGER PRIMARY KEY,applied_at TEXT NOT NULL)`); err != nil {
		return fmt.Errorf("sandbox migration table: %w", err)
	}
	var recorded sql.NullInt64
	if err = tx.QueryRowContext(ctx, `SELECT MAX(version) FROM sandbox_schema_migrations`).Scan(&recorded); err != nil {
		return fmt.Errorf("sandbox migration version: %w", err)
	}
	if recorded.Valid && recorded.Int64 > latestSchemaVersion {
		return fmt.Errorf("sandbox schema version %d is newer than supported %d", recorded.Int64, latestSchemaVersion)
	}
	if !recorded.Valid {
		for _, statement := range []string{
			`CREATE TABLE sandboxes(
    id TEXT PRIMARY KEY,
    display_name TEXT NOT NULL,
    backend TEXT NOT NULL,
    isolation TEXT NOT NULL,
    local_root TEXT NOT NULL,
    owner_kind TEXT NOT NULL,
    owner_ref TEXT NOT NULL,
    created_at TEXT NOT NULL
   )`,
			`CREATE TABLE sandbox_runs(
    sandbox_id TEXT PRIMARY KEY REFERENCES sandboxes(id) ON DELETE RESTRICT,
    generation INTEGER NOT NULL CHECK(generation > 0),
    state TEXT NOT NULL,
    external_id TEXT NOT NULL,
    updated_at TEXT NOT NULL
   )`,
		} {
			if _, err = tx.ExecContext(ctx, statement); err != nil {
				return fmt.Errorf("apply sandbox migration 1: %w", err)
			}
		}
		if _, err = tx.ExecContext(ctx, `INSERT INTO sandbox_schema_migrations(version,applied_at) VALUES(1,?)`, time.Now().UTC().Format(time.RFC3339Nano)); err != nil {
			return fmt.Errorf("record sandbox migration 1: %w", err)
		}
	}
	var admissionApplied int
	if err = tx.QueryRowContext(ctx, `SELECT COUNT(*) FROM sandbox_schema_migrations WHERE version = 2`).Scan(&admissionApplied); err != nil {
		return fmt.Errorf("check sandbox migration 2: %w", err)
	}
	if admissionApplied == 0 {
		if _, err = tx.ExecContext(ctx, `CREATE TABLE sandbox_admissions (
			operation_id TEXT PRIMARY KEY,
			sandbox_id TEXT NOT NULL REFERENCES sandboxes(id),
			action TEXT NOT NULL,
			principal_kind TEXT NOT NULL,
			principal_ref TEXT NOT NULL,
			origin TEXT NOT NULL,
			scope_kind TEXT NOT NULL,
			scope_reference TEXT NOT NULL,
			policy_kind TEXT NOT NULL,
			policy_reason TEXT NOT NULL,
			approval TEXT NOT NULL,
			status TEXT NOT NULL CHECK (status IN ('admitted','denied','failed')),
			started_at TEXT NOT NULL,
			finished_at TEXT NOT NULL
		)`); err != nil {
			return fmt.Errorf("apply sandbox migration 2: %w", err)
		}
		if _, err = tx.ExecContext(ctx, `CREATE INDEX idx_sandbox_admissions_sandbox ON sandbox_admissions (sandbox_id, started_at DESC)`); err != nil {
			return fmt.Errorf("create sandbox admission index: %w", err)
		}
		if _, err = tx.ExecContext(ctx, `INSERT INTO sandbox_schema_migrations(version,applied_at) VALUES(2,?)`, time.Now().UTC().Format(time.RFC3339Nano)); err != nil {
			return fmt.Errorf("record sandbox migration 2: %w", err)
		}
	}
	var detached int
	if err = tx.QueryRowContext(ctx, `SELECT COUNT(*) FROM sandbox_schema_migrations WHERE version = 3`).Scan(&detached); err != nil {
		return fmt.Errorf("check sandbox migration 3: %w", err)
	}
	if detached == 0 {
		// An admitted provision must be audited *before* its Sandbox exists;
		// admission history must also survive resource destruction.
		if _, err = tx.ExecContext(ctx, `CREATE TABLE sandbox_admissions_v3 (
			operation_id TEXT PRIMARY KEY,
			sandbox_id TEXT NOT NULL,
			action TEXT NOT NULL,
			principal_kind TEXT NOT NULL,
			principal_ref TEXT NOT NULL,
			origin TEXT NOT NULL,
			scope_kind TEXT NOT NULL,
			scope_reference TEXT NOT NULL,
			policy_kind TEXT NOT NULL,
			policy_reason TEXT NOT NULL,
			approval TEXT NOT NULL,
			status TEXT NOT NULL CHECK(status IN ('admitted','denied','failed')),
			started_at TEXT NOT NULL,
			finished_at TEXT NOT NULL
		)`); err != nil {
			return fmt.Errorf("create sandbox admission v3: %w", err)
		}
		if _, err = tx.ExecContext(ctx, `INSERT INTO sandbox_admissions_v3 SELECT * FROM sandbox_admissions`); err != nil {
			return fmt.Errorf("transfer sandbox admissions: %w", err)
		}
		if _, err = tx.ExecContext(ctx, `DROP TABLE sandbox_admissions`); err != nil {
			return fmt.Errorf("replace sandbox admission table: %w", err)
		}
		if _, err = tx.ExecContext(ctx, `ALTER TABLE sandbox_admissions_v3 RENAME TO sandbox_admissions`); err != nil {
			return fmt.Errorf("rename sandbox admission table: %w", err)
		}
		if _, err = tx.ExecContext(ctx, `CREATE INDEX idx_sandbox_admissions_sandbox ON sandbox_admissions (sandbox_id,started_at DESC)`); err != nil {
			return fmt.Errorf("recreate sandbox admission index: %w", err)
		}
		if _, err = tx.ExecContext(ctx, `INSERT INTO sandbox_schema_migrations(version,applied_at) VALUES(3,?)`, time.Now().UTC().Format(time.RFC3339Nano)); err != nil {
			return fmt.Errorf("record sandbox migration 3: %w", err)
		}
	}
	var leaseApplied int
	if err = tx.QueryRowContext(ctx, `SELECT COUNT(*) FROM sandbox_schema_migrations WHERE version = 4`).Scan(&leaseApplied); err != nil {
		return fmt.Errorf("sandbox migration 4 presence: %w", err)
	}
	if leaseApplied == 0 {
		if _, err = tx.ExecContext(ctx, `CREATE TABLE sandbox_execution_leases (
            sandbox_id TEXT PRIMARY KEY REFERENCES sandboxes(id) ON DELETE RESTRICT,
            lease_token TEXT NOT NULL UNIQUE,
            generation INTEGER NOT NULL CHECK(generation > 0),
            principal_kind TEXT NOT NULL,
            principal_ref TEXT NOT NULL,
            scope_kind TEXT NOT NULL,
            scope_ref TEXT NOT NULL,
            acquired_at TEXT NOT NULL
        )`); err != nil {
			return fmt.Errorf("sandbox migration 4 leases: %w", err)
		}
		if _, err = tx.ExecContext(ctx, `INSERT INTO sandbox_schema_migrations(version,applied_at) VALUES(4,?)`, time.Now().UTC().Format(time.RFC3339Nano)); err != nil {
			return fmt.Errorf("sandbox migration 4 record: %w", err)
		}
	}
	var lifecycleApplied int
	if err = tx.QueryRowContext(ctx, `SELECT COUNT(*) FROM sandbox_schema_migrations WHERE version = 5`).Scan(&lifecycleApplied); err != nil {
		return fmt.Errorf("sandbox migration 5 presence: %w", err)
	}
	if lifecycleApplied == 0 {
		if _, err = tx.ExecContext(ctx, `CREATE TABLE sandbox_lifecycle_events (
            event_id INTEGER PRIMARY KEY AUTOINCREMENT,
            sandbox_id TEXT NOT NULL REFERENCES sandboxes(id),
            action TEXT NOT NULL,
            previous_state TEXT NOT NULL,
            next_state TEXT NOT NULL,
            generation INTEGER NOT NULL CHECK(generation > 0),
            principal_kind TEXT NOT NULL,
            principal_ref TEXT NOT NULL,
            origin TEXT NOT NULL,
            scope_kind TEXT NOT NULL,
            scope_ref TEXT NOT NULL,
            finished_at TEXT NOT NULL
        )`); err != nil {
			return fmt.Errorf("sandbox migration 5 lifecycle table: %w", err)
		}
		if _, err = tx.ExecContext(ctx, `CREATE INDEX idx_sandbox_lifecycle_events_id ON sandbox_lifecycle_events(sandbox_id,event_id DESC)`); err != nil {
			return fmt.Errorf("sandbox migration 5 lifecycle index: %w", err)
		}
		if _, err = tx.ExecContext(ctx, `INSERT INTO sandbox_schema_migrations(version,applied_at) VALUES(5,?)`, time.Now().UTC().Format(time.RFC3339Nano)); err != nil {
			return fmt.Errorf("sandbox migration 5 record: %w", err)
		}
	}
	var dockerTemplatesApplied int
	if err = tx.QueryRowContext(ctx, `SELECT COUNT(*) FROM sandbox_schema_migrations WHERE version = 6`).Scan(&dockerTemplatesApplied); err != nil {
		return fmt.Errorf("sandbox migration 6 presence: %w", err)
	}
	if dockerTemplatesApplied == 0 {
		for _, statement := range []string{
			`CREATE TABLE sandbox_docker_templates (
                sandbox_id TEXT PRIMARY KEY REFERENCES sandboxes(id) ON DELETE RESTRICT,
                image_digest TEXT NOT NULL,
                user_id INTEGER NOT NULL CHECK(user_id BETWEEN 1000 AND 65534),
                group_id INTEGER NOT NULL CHECK(group_id BETWEEN 1000 AND 65534),
                cpu_milli INTEGER NOT NULL CHECK(cpu_milli BETWEEN 100 AND 8000),
                memory_mib INTEGER NOT NULL CHECK(memory_mib BETWEEN 256 AND 16384),
                workspace_mib INTEGER NOT NULL CHECK(workspace_mib BETWEEN 64 AND 2048),
                pid_limit INTEGER NOT NULL CHECK(pid_limit BETWEEN 16 AND 512),
                created_at TEXT NOT NULL
            )`,
			`CREATE TRIGGER sandbox_docker_template_immutable_update BEFORE UPDATE ON sandbox_docker_templates BEGIN SELECT RAISE(ABORT,'Docker Sandbox template is immutable'); END`,
			`CREATE TRIGGER sandbox_docker_template_immutable_delete BEFORE DELETE ON sandbox_docker_templates BEGIN SELECT RAISE(ABORT,'Docker Sandbox template cannot be deleted'); END`,
		} {
			if _, err = tx.ExecContext(ctx, statement); err != nil {
				return fmt.Errorf("sandbox migration 6 Docker templates: %w", err)
			}
		}
		if _, err = tx.ExecContext(ctx, `INSERT INTO sandbox_schema_migrations(version,applied_at) VALUES(6,?)`, time.Now().UTC().Format(time.RFC3339Nano)); err != nil {
			return fmt.Errorf("sandbox migration 6 record: %w", err)
		}
	}
	var daytonaTemplateApplied int
	if err = tx.QueryRowContext(ctx, `SELECT COUNT(*) FROM sandbox_schema_migrations WHERE version = 7`).Scan(&daytonaTemplateApplied); err != nil {
		return fmt.Errorf("sandbox migration 7 presence: %w", err)
	}
	if daytonaTemplateApplied == 0 {
		for _, statement := range []string{
			`CREATE TABLE sandbox_daytona_templates (
                sandbox_id TEXT PRIMARY KEY REFERENCES sandboxes(id) ON DELETE RESTRICT,
                credential_ref TEXT NOT NULL,
                image_digest TEXT NOT NULL,
                target TEXT NOT NULL CHECK (target='us'),
                cpu INTEGER NOT NULL CHECK(cpu BETWEEN 1 AND 2),
                memory_gib INTEGER NOT NULL CHECK(memory_gib BETWEEN 1 AND 4),
                disk_gib INTEGER NOT NULL CHECK(disk_gib BETWEEN 3 AND 10),
                ttl_minutes INTEGER NOT NULL CHECK(ttl_minutes BETWEEN 15 AND 120),
                created_at TEXT NOT NULL
            )`,
			`CREATE TRIGGER sandbox_daytona_template_immutable_update BEFORE UPDATE ON sandbox_daytona_templates BEGIN SELECT RAISE(ABORT,'Daytona Sandbox template is immutable'); END`,
			`CREATE TRIGGER sandbox_daytona_template_immutable_delete BEFORE DELETE ON sandbox_daytona_templates BEGIN SELECT RAISE(ABORT,'Daytona Sandbox template cannot be deleted'); END`,
		} {
			if _, err = tx.ExecContext(ctx, statement); err != nil {
				return fmt.Errorf("sandbox migration 7 templates: %w", err)
			}
		}
		if _, err = tx.ExecContext(ctx, `INSERT INTO sandbox_schema_migrations(version,applied_at) VALUES(7,?)`, time.Now().UTC().Format(time.RFC3339Nano)); err != nil {
			return fmt.Errorf("sandbox migration 7 record: %w", err)
		}
	}
	var delegationApplied int
	if err = tx.QueryRowContext(ctx, "SELECT COUNT(*) FROM sandbox_schema_migrations WHERE version = 8").Scan(&delegationApplied); err != nil {
		return fmt.Errorf("sandbox migration 8 presence: %w", err)
	}
	if delegationApplied == 0 {
		for _, statement := range []string{
			`CREATE TABLE sandbox_profile_grants (
                sandbox_id TEXT NOT NULL REFERENCES sandboxes(id) ON DELETE RESTRICT,
                profile_id TEXT NOT NULL,
                capability TEXT NOT NULL CHECK(capability IN ('file.read','file.search','file.edit','process.shell')),
                granted_at TEXT NOT NULL,
                PRIMARY KEY(sandbox_id,profile_id,capability)
            )`,
			`CREATE TABLE sandbox_profile_grant_events (
                event_id INTEGER PRIMARY KEY AUTOINCREMENT,
                sandbox_id TEXT NOT NULL REFERENCES sandboxes(id) ON DELETE RESTRICT,
                profile_id TEXT NOT NULL,
                capability TEXT NOT NULL CHECK(capability IN ('file.read','file.search','file.edit','process.shell')),
                action TEXT NOT NULL CHECK(action IN ('grant','revoke')),
                principal_kind TEXT NOT NULL CHECK(principal_kind='local_user'),
                principal_ref TEXT NOT NULL,
                origin TEXT NOT NULL,
                occurred_at TEXT NOT NULL
            )`,
			`CREATE INDEX idx_sandbox_profile_grant_events ON sandbox_profile_grant_events(sandbox_id,event_id DESC)`,
		} {
			if _, err = tx.ExecContext(ctx, statement); err != nil {
				return fmt.Errorf("sandbox migration 8: %w", err)
			}
		}
		if _, err = tx.ExecContext(ctx, "INSERT INTO sandbox_schema_migrations(version,applied_at) VALUES(8,?)", time.Now().UTC().Format(time.RFC3339Nano)); err != nil {
			return fmt.Errorf("record sandbox migration 8: %w", err)
		}
	}
	// Phase 27 C2: a Sandbox binds to exact extension software, not to a
	// historical vendor kind inferred on every runtime read. This transaction
	// retains all stable Sandbox/run/lease/grant and template IDs unchanged.
	var bindingApplied int
	if err = tx.QueryRowContext(ctx, "SELECT COUNT(*) FROM sandbox_schema_migrations WHERE version=9").Scan(&bindingApplied); err != nil {
		return fmt.Errorf("sandbox migration 9 presence: %w", err)
	}
	if bindingApplied == 0 {
		for _, statement := range []string{
			"ALTER TABLE sandboxes ADD COLUMN backend_plugin_id TEXT NOT NULL DEFAULT ''",
			"ALTER TABLE sandboxes ADD COLUMN backend_extension_id TEXT NOT NULL DEFAULT ''",
			"ALTER TABLE sandboxes ADD COLUMN backend_installation_id TEXT NOT NULL DEFAULT ''",
			`UPDATE sandboxes SET backend_plugin_id=CASE backend
                WHEN 'local_workspace' THEN 'io.mender.sandbox.local-workspace'
                WHEN 'docker' THEN 'io.mender.sandbox.docker'
                WHEN 'daytona' THEN 'io.mender.sandbox.daytona'
                ELSE '' END, backend_extension_id='main'`,
		} {
			if _, err = tx.ExecContext(ctx, statement); err != nil {
				return fmt.Errorf("sandbox migration 9 binding: %w", err)
			}
		}
		var missing int
		if err = tx.QueryRowContext(ctx, "SELECT COUNT(*) FROM sandboxes WHERE backend_plugin_id='' OR backend_extension_id=''").Scan(&missing); err != nil {
			return err
		}
		if missing != 0 {
			return fmt.Errorf("sandbox migration 9: %d resources have unknown backend plugin identity", missing)
		}
		// ALTER TABLE defaults are required solely for the one-time copy.
		// Triggers prohibit any legacy INSERT/UPDATE that omits the binding.
		// The historical backend classification is not a dispatch authority.
		for _, statement := range []string{
			`CREATE TRIGGER sandbox_extension_binding_insert BEFORE INSERT ON sandboxes
            WHEN NEW.backend_plugin_id='' OR NEW.backend_extension_id!='main' OR
                 (NEW.backend='local_workspace' AND (NEW.backend_plugin_id!='io.mender.sandbox.local-workspace' OR NEW.backend_installation_id!='')) OR
                 (NEW.backend='docker' AND (NEW.backend_plugin_id!='io.mender.sandbox.docker' OR NEW.backend_installation_id!='')) OR
                 (NEW.backend='daytona' AND (NEW.backend_plugin_id!='io.mender.sandbox.daytona' OR NEW.backend_installation_id!=''))
            BEGIN SELECT RAISE(ABORT,'Sandbox extension binding is required'); END`,
			`CREATE TRIGGER sandbox_extension_binding_update BEFORE UPDATE ON sandboxes
            WHEN NEW.backend_plugin_id='' OR NEW.backend_extension_id!='main' OR
                 NEW.backend_plugin_id!=OLD.backend_plugin_id OR NEW.backend_extension_id!=OLD.backend_extension_id OR
                 NEW.backend_installation_id!=OLD.backend_installation_id OR NEW.backend!=OLD.backend OR
                 (NEW.backend='local_workspace' AND NEW.backend_plugin_id!='io.mender.sandbox.local-workspace') OR
                 (NEW.backend='docker' AND NEW.backend_plugin_id!='io.mender.sandbox.docker') OR
                 (NEW.backend='daytona' AND NEW.backend_plugin_id!='io.mender.sandbox.daytona')
            BEGIN SELECT RAISE(ABORT,'Sandbox extension binding is immutable'); END`,
		} {
			if _, err = tx.ExecContext(ctx, statement); err != nil {
				return fmt.Errorf("sandbox migration 9 guard: %w", err)
			}
		}
		if _, err = tx.ExecContext(ctx, "INSERT INTO sandbox_schema_migrations(version,applied_at) VALUES(9,?)", time.Now().UTC().Format(time.RFC3339Nano)); err != nil {
			return fmt.Errorf("record sandbox migration 9: %w", err)
		}
	}
	var externalApplied int
	if err = tx.QueryRowContext(ctx, "SELECT COUNT(*) FROM sandbox_schema_migrations WHERE version=10").Scan(&externalApplied); err != nil {
		return fmt.Errorf("sandbox migration 10 presence: %w", err)
	}
	if externalApplied == 0 {
		for _, statement := range []string{
			// A stable host-generated idempotency key is durably written before
			// any external side effect. No operation is automatically replayed.
			`CREATE TABLE sandbox_external_operations (
                operation_id TEXT PRIMARY KEY,
                sandbox_id TEXT NOT NULL REFERENCES sandboxes(id) ON DELETE RESTRICT,
                generation INTEGER NOT NULL CHECK(generation>0),
                action TEXT NOT NULL CHECK(action IN ('sandbox.provision','sandbox.start','sandbox.stop','sandbox.destroy','sandbox.reconcile')),
                state TEXT NOT NULL CHECK(state IN ('issued','complete','uncertain')),
                created_at TEXT NOT NULL,
                finished_at TEXT NOT NULL DEFAULT ''
            )`,
			`CREATE INDEX idx_sandbox_external_operations_sandbox ON sandbox_external_operations(sandbox_id,created_at DESC)`,
			// Immutable non-secret config and exact install pin; no external
			// package can change it by updating an existing Sandbox row.
			`CREATE TABLE sandbox_external_configs (
                sandbox_id TEXT PRIMARY KEY REFERENCES sandboxes(id) ON DELETE RESTRICT,
                config_version INTEGER NOT NULL CHECK(config_version>0),
                config_json TEXT NOT NULL CHECK(json_valid(config_json) AND length(config_json)<=4096),
                created_at TEXT NOT NULL
            )`,
			`CREATE TRIGGER sandbox_external_config_update BEFORE UPDATE ON sandbox_external_configs BEGIN SELECT RAISE(ABORT,'Sandbox external config is immutable'); END`,
			`CREATE TRIGGER sandbox_external_config_delete BEFORE DELETE ON sandbox_external_configs BEGIN SELECT RAISE(ABORT,'Sandbox external config cannot be deleted'); END`,
			"DROP TRIGGER sandbox_extension_binding_insert",
			"DROP TRIGGER sandbox_extension_binding_update",
			`CREATE TRIGGER sandbox_extension_binding_insert BEFORE INSERT ON sandboxes
                WHEN NEW.backend_plugin_id='' OR NEW.backend_extension_id!='main' OR
                    (NEW.backend='local_workspace' AND (NEW.backend_plugin_id!='io.mender.sandbox.local-workspace' OR NEW.backend_installation_id!='')) OR
                    (NEW.backend='docker' AND (NEW.backend_plugin_id!='io.mender.sandbox.docker' OR NEW.backend_installation_id!='')) OR
                    (NEW.backend='daytona' AND (NEW.backend_plugin_id!='io.mender.sandbox.daytona' OR NEW.backend_installation_id!='')) OR
                    (NEW.backend='external' AND (NEW.backend_installation_id='' OR NEW.backend_plugin_id LIKE 'io.mender.%'))
                BEGIN SELECT RAISE(ABORT,'Sandbox extension binding is required'); END`,
			`CREATE TRIGGER sandbox_extension_binding_update BEFORE UPDATE ON sandboxes
                WHEN NEW.backend_plugin_id='' OR NEW.backend_extension_id!='main' OR
                    NEW.backend_plugin_id!=OLD.backend_plugin_id OR NEW.backend_extension_id!=OLD.backend_extension_id OR
                    NEW.backend_installation_id!=OLD.backend_installation_id OR NEW.backend!=OLD.backend OR
                    (NEW.backend='local_workspace' AND NEW.backend_plugin_id!='io.mender.sandbox.local-workspace') OR
                    (NEW.backend='docker' AND NEW.backend_plugin_id!='io.mender.sandbox.docker') OR
                    (NEW.backend='daytona' AND NEW.backend_plugin_id!='io.mender.sandbox.daytona') OR
                    (NEW.backend='external' AND (NEW.backend_installation_id='' OR NEW.backend_plugin_id LIKE 'io.mender.%'))
                BEGIN SELECT RAISE(ABORT,'Sandbox extension binding is immutable'); END`,
		} {
			if _, err = tx.ExecContext(ctx, statement); err != nil {
				return fmt.Errorf("sandbox migration 10: %w", err)
			}
		}
		if _, err = tx.ExecContext(ctx, "INSERT INTO sandbox_schema_migrations(version,applied_at) VALUES(10,?)", time.Now().UTC().Format(time.RFC3339Nano)); err != nil {
			return fmt.Errorf("record sandbox migration 10: %w", err)
		}
	}
	var sshApplied int
	if err = tx.QueryRowContext(ctx, "SELECT COUNT(*) FROM sandbox_schema_migrations WHERE version=11").Scan(&sshApplied); err != nil {
		return fmt.Errorf("sandbox migration 11 presence: %w", err)
	}
	if sshApplied == 0 {
		for _, statement := range []string{
			// SSH is remote account access, NOT remote isolation. Only a
			// Credential Reference and pinned public host key are stored here.
			`CREATE TABLE sandbox_ssh_remote_templates (
                sandbox_id TEXT PRIMARY KEY REFERENCES sandboxes(id) ON DELETE RESTRICT,
                credential_ref TEXT NOT NULL,
                host TEXT NOT NULL,
                port INTEGER NOT NULL CHECK(port BETWEEN 1 AND 65535),
                login_user TEXT NOT NULL,
                workspace_root TEXT NOT NULL,
                pinned_host_key TEXT NOT NULL,
                created_at TEXT NOT NULL
            )`,
			`CREATE INDEX idx_ssh_remote_credential_ref ON sandbox_ssh_remote_templates(credential_ref)`,
			`CREATE TRIGGER sandbox_ssh_template_immutable_update BEFORE UPDATE ON sandbox_ssh_remote_templates BEGIN SELECT RAISE(ABORT,'SSH Sandbox template is immutable'); END`,
			`CREATE TRIGGER sandbox_ssh_template_immutable_delete BEFORE DELETE ON sandbox_ssh_remote_templates BEGIN SELECT RAISE(ABORT,'SSH Sandbox template cannot be deleted'); END`,
			"DROP TRIGGER sandbox_extension_binding_insert",
			"DROP TRIGGER sandbox_extension_binding_update",
			`CREATE TRIGGER sandbox_extension_binding_insert BEFORE INSERT ON sandboxes
                WHEN NEW.backend_plugin_id='' OR NEW.backend_extension_id!='main' OR
                    (NEW.backend='local_workspace' AND (NEW.backend_plugin_id!='io.mender.sandbox.local-workspace' OR NEW.backend_installation_id!='')) OR
                    (NEW.backend='docker' AND (NEW.backend_plugin_id!='io.mender.sandbox.docker' OR NEW.backend_installation_id!='')) OR
                    (NEW.backend='daytona' AND (NEW.backend_plugin_id!='io.mender.sandbox.daytona' OR NEW.backend_installation_id!='')) OR
                    (NEW.backend='ssh_remote' AND (NEW.backend_plugin_id!='io.mender.sandbox.ssh-remote' OR NEW.backend_installation_id!='')) OR
                    (NEW.backend='external' AND (NEW.backend_installation_id='' OR NEW.backend_plugin_id LIKE 'io.mender.%'))
                BEGIN SELECT RAISE(ABORT,'Sandbox extension binding is required'); END`,
			`CREATE TRIGGER sandbox_extension_binding_update BEFORE UPDATE ON sandboxes
                WHEN NEW.backend_plugin_id='' OR NEW.backend_extension_id!='main' OR
                    NEW.backend_plugin_id!=OLD.backend_plugin_id OR NEW.backend_extension_id!=OLD.backend_extension_id OR
                    NEW.backend_installation_id!=OLD.backend_installation_id OR NEW.backend!=OLD.backend OR
                    (NEW.backend='local_workspace' AND NEW.backend_plugin_id!='io.mender.sandbox.local-workspace') OR
                    (NEW.backend='docker' AND NEW.backend_plugin_id!='io.mender.sandbox.docker') OR
                    (NEW.backend='daytona' AND NEW.backend_plugin_id!='io.mender.sandbox.daytona') OR
                    (NEW.backend='ssh_remote' AND NEW.backend_plugin_id!='io.mender.sandbox.ssh-remote') OR
                    (NEW.backend='external' AND (NEW.backend_installation_id='' OR NEW.backend_plugin_id LIKE 'io.mender.%'))
                BEGIN SELECT RAISE(ABORT,'Sandbox extension binding is immutable'); END`,
		} {
			if _, err = tx.ExecContext(ctx, statement); err != nil {
				return fmt.Errorf("sandbox migration 11 SSH templates: %w", err)
			}
		}
		if _, err = tx.ExecContext(ctx, "INSERT INTO sandbox_schema_migrations(version,applied_at) VALUES(11,?)", time.Now().UTC().Format(time.RFC3339Nano)); err != nil {
			return fmt.Errorf("record sandbox migration 11: %w", err)
		}
	}
	var suiteApplied int
	if err = tx.QueryRowContext(ctx, "SELECT COUNT(*) FROM sandbox_schema_migrations WHERE version=12").Scan(&suiteApplied); err != nil {
		return fmt.Errorf("sandbox migration 12 presence: %w", err)
	}
	if suiteApplied == 0 {
		if _, err = tx.ExecContext(ctx, `CREATE TABLE sandbox_tool_suite_bindings (
			sandbox_id TEXT NOT NULL REFERENCES sandboxes(id) ON DELETE RESTRICT,
			suite_id TEXT NOT NULL CHECK(suite_id='sandbox.workspace'),
			provider_id TEXT NOT NULL UNIQUE REFERENCES providers(id) ON DELETE RESTRICT,
			created_at TEXT NOT NULL,
			PRIMARY KEY(sandbox_id,suite_id)
		)`); err != nil {
			return fmt.Errorf("sandbox migration 12 suite bindings: %w", err)
		}
		if _, err = tx.ExecContext(ctx, "INSERT INTO sandbox_schema_migrations(version,applied_at) VALUES(12,?)", time.Now().UTC().Format(time.RFC3339Nano)); err != nil {
			return fmt.Errorf("record sandbox migration 12: %w", err)
		}
	}
	// Stage B: one non-sensitive, immutable plugin execution obligation per
	// SSH Sandbox. TaskIDs, job states and logs NEVER enter Mender SQLite.
	// A confirmed remote absence proof plus an atomic lifecycle fence is
	// required for Stop/Destroy once a Sandbox has ever submitted Jobs.
	var sshJobsApplied int
	if err = tx.QueryRowContext(ctx, "SELECT COUNT(*) FROM sandbox_schema_migrations WHERE version=13").Scan(&sshJobsApplied); err != nil {
		return fmt.Errorf("sandbox migration 13 presence: %w", err)
	}
	if sshJobsApplied == 0 {
		for _, statement := range []string{
			`CREATE TABLE sandbox_ssh_job_obligations (
				sandbox_id TEXT PRIMARY KEY REFERENCES sandboxes(id) ON DELETE RESTRICT,
				enrolled_at TEXT NOT NULL
			)`,
			`CREATE TRIGGER sandbox_ssh_job_obligation_immutable_update
			BEFORE UPDATE ON sandbox_ssh_job_obligations
			BEGIN SELECT RAISE(ABORT,'SSH Job obligation cannot be updated'); END`,
			`CREATE TRIGGER sandbox_ssh_job_obligation_immutable_delete
			BEFORE DELETE ON sandbox_ssh_job_obligations
			BEGIN SELECT RAISE(ABORT,'SSH Job obligation cannot be deleted'); END`,
		} {
			if _, err = tx.ExecContext(ctx, statement); err != nil {
				return fmt.Errorf("sandbox migration 13 SSH job obligation: %w", err)
			}
		}
		if _, err = tx.ExecContext(ctx, "INSERT INTO sandbox_schema_migrations(version,applied_at) VALUES(13,?)", time.Now().UTC().Format(time.RFC3339Nano)); err != nil {
			return fmt.Errorf("record sandbox migration 13: %w", err)
		}
	}
	// Stage C hard cut: permit an independently bound, host-reviewed SSH Jobs
	// suite and three narrowly scoped Job Profile grants. Preserve every
	// existing Suite ProviderID, Workspace grant, and immutable audit event.
	var jobsSupplyApplied int
	if err = tx.QueryRowContext(ctx, "SELECT COUNT(*) FROM sandbox_schema_migrations WHERE version=14").Scan(&jobsSupplyApplied); err != nil {
		return fmt.Errorf("sandbox migration 14 presence: %w", err)
	}
	if jobsSupplyApplied == 0 {
		for _, statement := range []string{
			// A standalone Sandbox store may precede the Provider Catalog table.
			// Keep the Workspace Suite table intact and persist the optional Jobs
			// binding independently; rebuilding the old FK table while providers
			// does not exist fails and risks corrupting cross-context startup.
			`CREATE TABLE sandbox_job_suite_bindings (
				sandbox_id TEXT PRIMARY KEY REFERENCES sandboxes(id) ON DELETE RESTRICT,
				provider_id TEXT NOT NULL UNIQUE REFERENCES providers(id) ON DELETE RESTRICT,
				created_at TEXT NOT NULL
			)`,

			`CREATE TABLE sandbox_profile_grants_v14 (
				sandbox_id TEXT NOT NULL REFERENCES sandboxes(id) ON DELETE RESTRICT,
				profile_id TEXT NOT NULL,
				capability TEXT NOT NULL CHECK(capability IN ('file.read','file.search','file.edit','process.shell','job.submit','job.observe','job.cancel')),
				granted_at TEXT NOT NULL, PRIMARY KEY(sandbox_id,profile_id,capability)
			)`,
			`INSERT INTO sandbox_profile_grants_v14 SELECT sandbox_id,profile_id,capability,granted_at FROM sandbox_profile_grants`,
			`DROP TABLE sandbox_profile_grants`,
			`ALTER TABLE sandbox_profile_grants_v14 RENAME TO sandbox_profile_grants`,
			`CREATE TABLE sandbox_profile_grant_events_v14 (
				event_id INTEGER PRIMARY KEY AUTOINCREMENT,
				sandbox_id TEXT NOT NULL REFERENCES sandboxes(id) ON DELETE RESTRICT,
				profile_id TEXT NOT NULL,
				capability TEXT NOT NULL CHECK(capability IN ('file.read','file.search','file.edit','process.shell','job.submit','job.observe','job.cancel')),
				action TEXT NOT NULL CHECK(action IN ('grant','revoke')),
				principal_kind TEXT NOT NULL CHECK(principal_kind='local_user'),
				principal_ref TEXT NOT NULL, origin TEXT NOT NULL, occurred_at TEXT NOT NULL
			)`,
			`INSERT INTO sandbox_profile_grant_events_v14 SELECT event_id,sandbox_id,profile_id,capability,action,principal_kind,principal_ref,origin,occurred_at FROM sandbox_profile_grant_events`,
			`DROP TABLE sandbox_profile_grant_events`,
			`ALTER TABLE sandbox_profile_grant_events_v14 RENAME TO sandbox_profile_grant_events`,
			`CREATE INDEX idx_sandbox_profile_grant_events ON sandbox_profile_grant_events(sandbox_id,event_id DESC)`,
		} {
			if _, err = tx.ExecContext(ctx, statement); err != nil {
				return fmt.Errorf("sandbox migration 14 Job supply: %w", err)
			}
		}
		if _, err = tx.ExecContext(ctx, "INSERT INTO sandbox_schema_migrations(version,applied_at) VALUES(14,?)", time.Now().UTC().Format(time.RFC3339Nano)); err != nil {
			return fmt.Errorf("record sandbox migration 14: %w", err)
		}
	}
	// Recovery R2: owner-controlled admission pause. A missing row means
	// unpaused. The flag is checked in the same write transaction as Job
	// submission lease reservation, independent of running remote Tasks.
	var pauseApplied int
	if err = tx.QueryRowContext(ctx, "SELECT COUNT(*) FROM sandbox_schema_migrations WHERE version=15").Scan(&pauseApplied); err != nil {
		return fmt.Errorf("sandbox migration 15 presence: %w", err)
	}
	if pauseApplied == 0 {
		for _, statement := range []string{
			`CREATE TABLE sandbox_ssh_job_admission_controls (
				sandbox_id TEXT PRIMARY KEY REFERENCES sandboxes(id) ON DELETE RESTRICT,
				paused INTEGER NOT NULL CHECK(paused IN (0,1)),
				revision INTEGER NOT NULL CHECK(revision > 0),
				updated_at TEXT NOT NULL
			)`,
			`CREATE TABLE sandbox_ssh_job_admission_events (
				event_id INTEGER PRIMARY KEY AUTOINCREMENT,
				sandbox_id TEXT NOT NULL REFERENCES sandboxes(id) ON DELETE RESTRICT,
				action TEXT NOT NULL CHECK(action IN ('pause','resume')),
				generation INTEGER NOT NULL,
				principal_kind TEXT NOT NULL,
				principal_ref TEXT NOT NULL,
				origin TEXT NOT NULL,
				occurred_at TEXT NOT NULL
			)`,
		} {
			if _, err = tx.ExecContext(ctx, statement); err != nil {
				return fmt.Errorf("sandbox migration 15 SSH Job pause: %w", err)
			}
		}
		if _, err = tx.ExecContext(ctx, "INSERT INTO sandbox_schema_migrations(version,applied_at) VALUES(15,?)", time.Now().UTC().Format(time.RFC3339Nano)); err != nil {
			return fmt.Errorf("record sandbox migration 15: %w", err)
		}
	}
	// A standalone Sandbox store may precede the Provider catalogue. Install
	// guards whenever the two contexts share their Runtime database.
	var hasProviderConfigs int
	if err = tx.QueryRowContext(ctx, "SELECT COUNT(*) FROM sqlite_master WHERE type='table' AND name='provider_configs'").Scan(&hasProviderConfigs); err != nil {
		return err
	}
	if hasProviderConfigs > 0 {
		for _, statement := range []string{
			`CREATE TRIGGER IF NOT EXISTS sandbox_suite_config_delete_guard BEFORE DELETE ON provider_configs
			WHEN EXISTS(SELECT 1 FROM sandbox_tool_suite_bindings s WHERE s.provider_id=OLD.provider_id) OR EXISTS(SELECT 1 FROM sandbox_job_suite_bindings j WHERE j.provider_id=OLD.provider_id)
			BEGIN SELECT RAISE(ABORT,'Sandbox Tool Suite is resource-owned'); END`,
			`CREATE TRIGGER IF NOT EXISTS sandbox_suite_config_update_guard BEFORE UPDATE OF plugin_id,plugin_installation_id,config_json,config_version ON provider_configs
			WHEN EXISTS(SELECT 1 FROM sandbox_tool_suite_bindings s WHERE s.provider_id=OLD.provider_id) OR EXISTS(SELECT 1 FROM sandbox_job_suite_bindings j WHERE j.provider_id=OLD.provider_id)
			BEGIN SELECT RAISE(ABORT,'Sandbox Tool Suite config immutable'); END`,
		} {
			if _, err = tx.ExecContext(ctx, statement); err != nil {
				return fmt.Errorf("sandbox suite persistence guards: %w", err)
			}
		}
	}
	if err = tx.Commit(); err != nil {
		return fmt.Errorf("commit sandbox migrations: %w", err)
	}
	return nil
}

package sqlite

import (
	"context"
	"database/sql"
	"fmt"
	"time"
)

const latestSchemaVersion = 6

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
	if err = tx.Commit(); err != nil {
		return fmt.Errorf("commit sandbox migrations: %w", err)
	}
	return nil
}

package sqlite

import (
	"context"
	"database/sql"
	"fmt"
	"time"
)

const latestSchemaVersion = 2

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
	if err = tx.Commit(); err != nil {
		return fmt.Errorf("commit sandbox migrations: %w", err)
	}
	return nil
}

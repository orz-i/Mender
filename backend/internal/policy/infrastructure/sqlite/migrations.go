package sqlite

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"time"
)

func migrate(ctx context.Context, db *sql.DB) error {
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin policy migration: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	if _, err := tx.ExecContext(ctx, `CREATE TABLE IF NOT EXISTS policy_schema_migrations (
		version INTEGER PRIMARY KEY,
		applied_at TEXT NOT NULL
	)`); err != nil {
		return fmt.Errorf("create policy migration table: %w", err)
	}

	var recordedVersion sql.NullInt64
	if err := tx.QueryRowContext(
		ctx,
		"SELECT MAX(version) FROM policy_schema_migrations",
	).Scan(&recordedVersion); err != nil {
		return fmt.Errorf("check policy schema version: %w", err)
	}
	const latestSupportedVersion = 2
	if recordedVersion.Valid && recordedVersion.Int64 > latestSupportedVersion {
		return fmt.Errorf(
			"policy schema version %d is newer than supported version %d",
			recordedVersion.Int64,
			latestSupportedVersion,
		)
	}

	var applied int
	if err := tx.QueryRowContext(
		ctx,
		"SELECT COUNT(*) FROM policy_schema_migrations WHERE version = 1",
	).Scan(&applied); err != nil {
		return fmt.Errorf("check policy migration 1: %w", err)
	}
	if applied == 0 {
		statements := []string{
			`CREATE TABLE policy_settings (
				singleton_id INTEGER PRIMARY KEY CHECK (singleton_id = 1),
				default_decision TEXT NOT NULL,
				updated_at TEXT NOT NULL
			)`,
			`CREATE TABLE policy_rules (
				rule_id TEXT PRIMARY KEY,
				display_name TEXT NOT NULL,
				enabled INTEGER NOT NULL CHECK (enabled IN (0, 1)),
				decision TEXT NOT NULL,
				match_json TEXT NOT NULL,
				created_at TEXT NOT NULL,
				updated_at TEXT NOT NULL
			)`,
		}
		for _, statement := range statements {
			if _, err := tx.ExecContext(ctx, statement); err != nil {
				return fmt.Errorf("apply policy migration 1: %w", err)
			}
		}
		now := time.Now().UTC().Format(time.RFC3339Nano)
		if _, err := tx.ExecContext(
			ctx,
			"INSERT INTO policy_settings(singleton_id, default_decision, updated_at) VALUES (1, 'allow', ?)",
			now,
		); err != nil {
			return fmt.Errorf("initialize policy settings: %w", err)
		}
		if _, err := tx.ExecContext(
			ctx,
			"INSERT INTO policy_schema_migrations(version, applied_at) VALUES (1, ?)",
			now,
		); err != nil {
			return fmt.Errorf("record policy migration 1: %w", err)
		}
	}

	applied = 0
	if err := tx.QueryRowContext(
		ctx,
		"SELECT COUNT(*) FROM policy_schema_migrations WHERE version = 2",
	).Scan(&applied); err != nil {
		return fmt.Errorf("check policy migration 2: %w", err)
	}
	if applied == 0 {
		type ruleMatchRow struct {
			id  string
			raw string
		}
		rows, err := tx.QueryContext(ctx, "SELECT rule_id, match_json FROM policy_rules")
		if err != nil {
			return fmt.Errorf("load policy rules for migration 2: %w", err)
		}
		var matches []ruleMatchRow
		for rows.Next() {
			var row ruleMatchRow
			if err := rows.Scan(&row.id, &row.raw); err != nil {
				_ = rows.Close()
				return fmt.Errorf("scan policy rule for migration 2: %w", err)
			}
			matches = append(matches, row)
		}
		if err := rows.Err(); err != nil {
			_ = rows.Close()
			return fmt.Errorf("iterate policy rules for migration 2: %w", err)
		}
		if err := rows.Close(); err != nil {
			return fmt.Errorf("close policy rules for migration 2: %w", err)
		}

		for _, row := range matches {
			var match map[string]any
			if err := json.Unmarshal([]byte(row.raw), &match); err != nil {
				return fmt.Errorf("decode policy rule %s for migration 2: %w", row.id, err)
			}
			if _, exists := match["actions"]; exists {
				return fmt.Errorf("policy rule %s unexpectedly contains actions before migration 2", row.id)
			}
			match["actions"] = []string{"tool.invoke"}
			match["principalKinds"] = []string{}
			match["principalRefs"] = []string{}
			next, err := json.Marshal(match)
			if err != nil {
				return fmt.Errorf("encode policy rule %s for migration 2: %w", row.id, err)
			}
			if _, err := tx.ExecContext(
				ctx,
				"UPDATE policy_rules SET match_json = ? WHERE rule_id = ?",
				string(next),
				row.id,
			); err != nil {
				return fmt.Errorf("update policy rule %s for migration 2: %w", row.id, err)
			}
		}
		now := time.Now().UTC().Format(time.RFC3339Nano)
		if _, err := tx.ExecContext(
			ctx,
			"INSERT INTO policy_schema_migrations(version, applied_at) VALUES (2, ?)",
			now,
		); err != nil {
			return fmt.Errorf("record policy migration 2: %w", err)
		}
	}

	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit policy migrations: %w", err)
	}
	return nil
}

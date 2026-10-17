package sqlite

import (
	"context"
	"database/sql"
	"fmt"
	"time"
)

// Policy v3 provides an exact SQLite generation fence for Agent approvals.
// Every real policy mutation bumps this singleton in the SAME transaction,
// including owner actions and direct SQL updates/deletes/inserts.
func migrateRevisionV3(ctx context.Context, tx *sql.Tx) error {
	var applied int
	if err := tx.QueryRowContext(ctx, "SELECT COUNT(*) FROM policy_schema_migrations WHERE version=3").Scan(&applied); err != nil {
		return err
	}
	if applied > 0 {
		return nil
	}
	for _, sqlText := range []string{
		`ALTER TABLE policy_settings ADD COLUMN revision INTEGER NOT NULL DEFAULT 1 CHECK(revision>=1)`,
		`CREATE TRIGGER policy_revision_default_change AFTER UPDATE OF default_decision ON policy_settings
      WHEN NEW.default_decision<>OLD.default_decision
      BEGIN UPDATE policy_settings SET revision=revision+1 WHERE singleton_id=1; END`,
		`CREATE TRIGGER policy_revision_rule_insert AFTER INSERT ON policy_rules
      BEGIN UPDATE policy_settings SET revision=revision+1 WHERE singleton_id=1; END`,
		`CREATE TRIGGER policy_revision_rule_update AFTER UPDATE ON policy_rules
      BEGIN UPDATE policy_settings SET revision=revision+1 WHERE singleton_id=1; END`,
		`CREATE TRIGGER policy_revision_rule_delete AFTER DELETE ON policy_rules
      BEGIN UPDATE policy_settings SET revision=revision+1 WHERE singleton_id=1; END`,
	} {
		if _, err := tx.ExecContext(ctx, sqlText); err != nil {
			return fmt.Errorf("policy revision v3: %w", err)
		}
	}
	if _, err := tx.ExecContext(ctx, "INSERT INTO policy_schema_migrations(version,applied_at) VALUES(3,?)", time.Now().UTC().Format(time.RFC3339Nano)); err != nil {
		return fmt.Errorf("policy revision v3: record: %w", err)
	}
	return nil
}

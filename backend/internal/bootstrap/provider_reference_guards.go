package bootstrap

import (
	"context"
	"database/sql"
	"fmt"
	"net/url"
	"path/filepath"
	"strings"

	catalogapplication "github.com/orz-i/Mender/backend/internal/catalog/application"
	catalogdomain "github.com/orz-i/Mender/backend/internal/catalog/domain"
	_ "modernc.org/sqlite"
)

// Provider/Profile/Policy share one physical SQLite database but own separate
// domain models. These guards are an explicit Runtime-level persistence adapter:
// validation before a write is advisory; the database commit decides the race.
func openProviderReferenceDB(ctx context.Context, path string) (*sql.DB, error) {
	if strings.TrimSpace(path) == "" {
		return nil, fmt.Errorf("provider reference database path is required")
	}
	absolute, err := filepath.Abs(path)
	if err != nil {
		return nil, err
	}
	slashed := filepath.ToSlash(absolute)
	if !strings.HasPrefix(slashed, "/") {
		slashed = "/" + slashed
	}
	uri := url.URL{Scheme: "file", Path: slashed}
	q := uri.Query()
	q.Add("_pragma", "foreign_keys(1)")
	q.Add("_pragma", "busy_timeout(5000)")
	uri.RawQuery = q.Encode()
	db, err := sql.Open("sqlite", uri.String())
	if err != nil {
		return nil, err
	}
	db.SetMaxOpenConns(1)
	if err := db.PingContext(ctx); err != nil {
		_ = db.Close()
		return nil, err
	}
	return db, nil
}

func installProviderReferenceGuards(ctx context.Context, path string) error {
	db, err := openProviderReferenceDB(ctx, path)
	if err != nil {
		return fmt.Errorf("provider reference guards: open: %w", err)
	}
	defer func() { _ = db.Close() }()
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()
	for _, table := range []string{"providers", "tools", "tools_fts", "provider_configs", "profiles", "profile_tools", "policy_rules"} {
		var present int
		if err := tx.QueryRowContext(ctx, "SELECT COUNT(*) FROM sqlite_master WHERE name=? AND type IN ('table','view')", table).Scan(&present); err != nil {
			return err
		}
		if present != 1 {
			return fmt.Errorf("provider reference guards: missing required %s table", table)
		}
	}
	// Keep all three directions of the reference contract in SQLite. INSERT
	// and DELETE operations serialize even when different Runtime processes
	// use independent connection pools.
	for _, statement := range []string{
		// Provider config is a dependent resource, never an independent identity.
		`CREATE TRIGGER IF NOT EXISTS provider_config_reference_insert_guard
          BEFORE INSERT ON provider_configs
          WHEN NOT EXISTS (SELECT 1 FROM providers WHERE id=NEW.provider_id)
          BEGIN SELECT RAISE(ABORT,'Provider config reference unavailable'); END`,
		`CREATE TRIGGER IF NOT EXISTS provider_config_reference_update_guard
          BEFORE UPDATE OF provider_id ON provider_configs
          WHEN NOT EXISTS (SELECT 1 FROM providers WHERE id=NEW.provider_id)
          BEGIN SELECT RAISE(ABORT,'Provider config reference unavailable'); END`,
		`CREATE TRIGGER IF NOT EXISTS profile_tool_reference_insert_guard
          BEFORE INSERT ON profile_tools
          WHEN NOT EXISTS (SELECT 1 FROM tools t JOIN providers p ON p.id=t.provider_id WHERE t.id=NEW.tool_id)
          BEGIN SELECT RAISE(ABORT,'Profile Tool reference unavailable'); END`,
		`CREATE TRIGGER IF NOT EXISTS profile_tool_reference_update_guard
          BEFORE UPDATE OF tool_id ON profile_tools
          WHEN NOT EXISTS (SELECT 1 FROM tools t JOIN providers p ON p.id=t.provider_id WHERE t.id=NEW.tool_id)
          BEGIN SELECT RAISE(ABORT,'Profile Tool reference unavailable'); END`,
		`CREATE TRIGGER IF NOT EXISTS policy_reference_insert_guard
          BEFORE INSERT ON policy_rules
          WHEN NOT json_valid(NEW.match_json)
          OR EXISTS (
             SELECT 1 FROM json_each(CASE WHEN json_valid(NEW.match_json) THEN NEW.match_json ELSE '{}' END,'$.providerIds') ref
             WHERE ref.type!='null' AND NOT EXISTS (SELECT 1 FROM providers p WHERE p.id=ref.value)
          )
          OR EXISTS (
             SELECT 1 FROM json_each(CASE WHEN json_valid(NEW.match_json) THEN NEW.match_json ELSE '{}' END,'$.toolIds') ref
             WHERE ref.type!='null' AND NOT EXISTS (SELECT 1 FROM tools t JOIN providers p ON p.id=t.provider_id WHERE t.id=ref.value)
          )
          BEGIN SELECT RAISE(ABORT,'Policy reference unavailable'); END`,
		`CREATE TRIGGER IF NOT EXISTS policy_reference_update_guard
          BEFORE UPDATE OF match_json ON policy_rules
          WHEN NOT json_valid(NEW.match_json)
          OR EXISTS (
             SELECT 1 FROM json_each(CASE WHEN json_valid(NEW.match_json) THEN NEW.match_json ELSE '{}' END,'$.providerIds') ref
             WHERE ref.type!='null' AND NOT EXISTS (SELECT 1 FROM providers p WHERE p.id=ref.value)
          )
          OR EXISTS (
             SELECT 1 FROM json_each(CASE WHEN json_valid(NEW.match_json) THEN NEW.match_json ELSE '{}' END,'$.toolIds') ref
             WHERE ref.type!='null' AND NOT EXISTS (SELECT 1 FROM tools t JOIN providers p ON p.id=t.provider_id WHERE t.id=ref.value)
          )
          BEGIN SELECT RAISE(ABORT,'Policy reference unavailable'); END`,
		`CREATE TRIGGER IF NOT EXISTS provider_reference_delete_guard
          BEFORE DELETE ON providers
          WHEN EXISTS (
              SELECT 1 FROM profile_tools b JOIN tools t ON t.id=b.tool_id WHERE t.provider_id=OLD.id
          )
          OR EXISTS (SELECT 1 FROM policy_rules WHERE NOT json_valid(match_json))
          OR EXISTS (
              SELECT 1 FROM policy_rules r, json_each(CASE WHEN json_valid(r.match_json) THEN r.match_json ELSE '{}' END,'$.providerIds') ref
              WHERE ref.value=OLD.id
          )
          OR EXISTS (
              SELECT 1 FROM policy_rules r, json_each(CASE WHEN json_valid(r.match_json) THEN r.match_json ELSE '{}' END,'$.toolIds') ref
              JOIN tools t ON t.id=ref.value WHERE t.provider_id=OLD.id
          )
          BEGIN SELECT RAISE(ABORT,'Provider has active references'); END`,
	} {
		if _, err := tx.ExecContext(ctx, statement); err != nil {
			return fmt.Errorf("provider reference guards: install: %w", err)
		}
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("provider reference guards: commit: %w", err)
	}
	return nil
}

// deleteConfiguredProviderRows commits config, FTS and Catalog retirement as
// one atomic unit. The Provider deletion trigger is the last-word reference
// check: late Profile/Policy writers either commit first and block this delete,
// or see the missing Tool/Provider afterwards and fail their own insert guard.
func deleteConfiguredProviderRows(ctx context.Context, databasePath string, providerID catalogdomain.ProviderID) error {
	db, err := openProviderReferenceDB(ctx, databasePath)
	if err != nil {
		return fmt.Errorf("delete provider: open reference database: %w", err)
	}
	defer func() { _ = db.Close() }()
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("delete provider: begin: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	// First statement acquires the SQLite writer gate; no read snapshot is
	// taken before the write. All following changes roll back together.
	if _, err := tx.ExecContext(ctx, "DELETE FROM provider_configs WHERE provider_id=?", providerID.String()); err != nil {
		return referenceDeletionError("config", err)
	}
	if _, err := tx.ExecContext(ctx, "DELETE FROM tools_fts WHERE provider_id=?", providerID.String()); err != nil {
		return referenceDeletionError("search index", err)
	}
	result, err := tx.ExecContext(ctx, "DELETE FROM providers WHERE id=?", providerID.String())
	if err != nil {
		return referenceDeletionError("catalogue", err)
	}
	count, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("delete provider: affected rows: %w", err)
	}
	if count != 1 {
		return catalogapplication.ErrProviderNotFound
	}
	if err := tx.Commit(); err != nil {
		return referenceDeletionError("commit", err)
	}
	return nil
}

func referenceDeletionError(phase string, err error) error {
	msg := strings.ToLower(err.Error())
	if strings.Contains(msg, "provider has active references") ||
		strings.Contains(msg, "foreign key constraint failed") ||
		strings.Contains(msg, "resource-owned") ||
		strings.Contains(msg, "managed agent facade") {
		return fmt.Errorf("%w: Provider references changed during %s", ErrProviderInUse, phase)
	}
	return fmt.Errorf("delete provider: %s: %w", phase, err)
}

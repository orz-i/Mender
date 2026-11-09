// Package sqlite implements the reviewed shared-database Provider retirement
// seam. Resource references and retirement still serialize at one writer gate.
package sqlite

import (
	"context"
	"database/sql"
	"fmt"
	"net/url"
	"path/filepath"
	"strings"

	catalogapplication "github.com/orz-i/Mender/backend/internal/catalog/application"
	catalogdomain "github.com/orz-i/Mender/backend/internal/catalog/domain"
	providerapplication "github.com/orz-i/Mender/backend/internal/providerplugin/application"
	_ "modernc.org/sqlite"
)

type Retirement struct{ databasePath string }

func NewRetirement(databasePath string) Retirement { return Retirement{databasePath: databasePath} }

func openReferenceDB(ctx context.Context, path string) (*sql.DB, error) {
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

func (r Retirement) Delete(ctx context.Context, providerID catalogdomain.ProviderID) error {
	db, err := openReferenceDB(ctx, r.databasePath)
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
	// The canonical v3 Catalog stores ownership as SourceRef, not tools.provider_id.
	// Cross-context Profile/Policy/Agent references are checked by the v3
	// SQLite triggers at the same commit boundary. No legacy Facade tables
	// are consulted or recreated. ToolID retirement and FTS removal are one
	// write transaction; a concurrent grant blocks rather than broadens access.
	if _, err := tx.ExecContext(ctx, `DELETE FROM tools_fts WHERE tool_id IN (
		SELECT id FROM tools WHERE source_kind='provider' AND source_id=?)`, providerID.String()); err != nil {
		return referenceDeletionError("search index", err)
	}
	if _, err := tx.ExecContext(ctx, "DELETE FROM tools WHERE source_kind='provider' AND source_id=?", providerID.String()); err != nil {
		return referenceDeletionError("owned ToolIDs", err)
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
		strings.Contains(msg, "managed agent facade") ||
		strings.Contains(msg, "tool is native or referenced by profile/policy") ||
		strings.Contains(msg, "provider has owned tool references") {
		return fmt.Errorf("%w: Provider references changed during %s", providerapplication.ErrProviderInUse, phase)
	}
	return fmt.Errorf("delete provider: %s: %w", phase, err)
}

// Package querysqlite contains read-only management projections. Cross-context
// reads do not confer ownership, permission, or resource mutation authority.
package querysqlite

import (
	"context"
	"database/sql"
	"fmt"
	"net/url"
	"path/filepath"
	"strings"

	catalog "github.com/orz-i/Mender/backend/internal/catalog/domain"
	query "github.com/orz-i/Mender/backend/internal/management/application/query"
	_ "modernc.org/sqlite"
)

// Agent and Sandbox tools are now directly owned by typed SourceRef. There
// are no resource-to-Provider façade claims in a v3 Catalog. Independently
// configured Providers retain their own identity and are projected by the
// existing Provider query application, not by a fabricated relation.
func ListToolSourceOwners(ctx context.Context, databasePath string) (map[catalog.ProviderID]query.ToolSourceOwner, error) {
	db, err := openReadOnly(ctx, databasePath)
	if err != nil {
		return nil, fmt.Errorf("tool source owners: open: %w", err)
	}
	defer func() { _ = db.Close() }()
	if _, err = db.ExecContext(ctx, "SELECT source_kind,source_id FROM tools LIMIT 0"); err != nil {
		return nil, fmt.Errorf("tool source owners: typed Catalog required: %w", err)
	}
	return map[catalog.ProviderID]query.ToolSourceOwner{}, nil
}

func openReadOnly(ctx context.Context, databasePath string) (*sql.DB, error) {
	if strings.TrimSpace(databasePath) == "" {
		return nil, fmt.Errorf("management query database path is required")
	}
	absolute, err := filepath.Abs(databasePath)
	if err != nil {
		return nil, err
	}
	slashed := filepath.ToSlash(absolute)
	if !strings.HasPrefix(slashed, "/") {
		slashed = "/" + slashed
	}
	uri := url.URL{Scheme: "file", Path: slashed}
	parameters := uri.Query()
	parameters.Set("mode", "ro")
	parameters.Add("_pragma", "query_only(1)")
	parameters.Add("_pragma", "busy_timeout(5000)")
	uri.RawQuery = parameters.Encode()
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

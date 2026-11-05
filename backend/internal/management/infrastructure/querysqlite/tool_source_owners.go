// Package querysqlite contains explicitly read-only management projections.
// Cross-context joins here confer no resource ownership or mutation authority.
package querysqlite

import (
	"context"
	"database/sql"
	"fmt"
	"net/url"
	"path/filepath"
	"strings"

	catalogdomain "github.com/orz-i/Mender/backend/internal/catalog/domain"
	managementquery "github.com/orz-i/Mender/backend/internal/management/application/query"
	_ "modernc.org/sqlite"
)

// ListToolSourceOwners reads exact resource-to-facade relationships. Unlike
// reference/lifecycle guards, this projection has no SQLite write capability.
func ListToolSourceOwners(ctx context.Context, databasePath string) (map[catalogdomain.ProviderID]managementquery.ToolSourceOwner, error) {
	db, err := openReadOnly(ctx, databasePath)
	if err != nil {
		return nil, fmt.Errorf("tool source owners: open: %w", err)
	}
	defer func() { _ = db.Close() }()
	rows, err := db.QueryContext(ctx, `
 SELECT f.provider_id, 'agent', a.agent_id, a.name
 FROM agent_tool_facades f JOIN agents a ON a.agent_id=f.agent_id
 UNION ALL
 SELECT f.provider_id, 'sandbox', s.id, s.display_name
 FROM sandbox_tool_suite_bindings f JOIN sandboxes s ON s.id=f.sandbox_id
 UNION ALL
 SELECT f.provider_id, 'sandbox', s.id, s.display_name
 FROM sandbox_job_suite_bindings f JOIN sandboxes s ON s.id=f.sandbox_id
 `)
	if err != nil {
		return nil, fmt.Errorf("tool source owners: query: %w", err)
	}
	defer func() { _ = rows.Close() }()
	owners := map[catalogdomain.ProviderID]managementquery.ToolSourceOwner{}
	for rows.Next() {
		var rawID, kind, resourceID, name string
		if err = rows.Scan(&rawID, &kind, &resourceID, &name); err != nil {
			return nil, fmt.Errorf("tool source owners: scan: %w", err)
		}
		id, parseErr := catalogdomain.ParseProviderID(rawID)
		if parseErr != nil {
			return nil, fmt.Errorf("tool source owners: provider id: %w", parseErr)
		}
		if previous, exists := owners[id]; exists {
			return nil, fmt.Errorf("tool source owners: conflicting ownership for Provider %s (%s and %s)", id, previous.ResourceID, resourceID)
		}
		owners[id] = managementquery.ToolSourceOwner{Kind: kind, ResourceID: resourceID, Name: name}
	}
	if err = rows.Err(); err != nil {
		return nil, fmt.Errorf("tool source owners: rows: %w", err)
	}
	return owners, nil
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

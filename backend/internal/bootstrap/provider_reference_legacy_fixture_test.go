package bootstrap

import (
	"context"
	"database/sql"
	"fmt"
	"net/url"
	"path/filepath"
	"strings"

	catalogdomain "github.com/orz-i/Mender/backend/internal/catalog/domain"
	providerretirement "github.com/orz-i/Mender/backend/internal/providerplugin/infrastructure/sqlite"
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

// deleteConfiguredProviderRows commits config, FTS and Catalog retirement as
// one atomic unit. The Provider deletion trigger is the last-word reference
// check: late Profile/Policy writers either commit first and block this delete,
// or see the missing Tool/Provider afterwards and fail their own insert guard.
func deleteConfiguredProviderRows(ctx context.Context, databasePath string, providerID catalogdomain.ProviderID) error {
	return providerretirement.NewRetirement(databasePath).Delete(ctx, providerID)
}

package querysqlite

import (
	"context"
	"database/sql"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestToolSourceProjectionConnectionCannotWriteOrCreateDatabase(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	path := filepath.Join(t.TempDir(), "owners.db")
	fixture := sourceOwnerFixture(t, path)
	defer func() { _ = fixture.Close() }()
	db, err := openReadOnly(ctx, path)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = db.Close() }()
	var queryOnly int
	if err := db.QueryRowContext(ctx, "PRAGMA query_only").Scan(&queryOnly); err != nil || queryOnly != 1 {
		t.Fatalf("query_only = %d, error %v", queryOnly, err)
	}
	if _, err := db.ExecContext(ctx, "INSERT INTO agents(agent_id,name) VALUES('forbidden','Forbidden')"); err == nil {
		t.Fatal("management projection connection accepted a write")
	}
	missing := filepath.Join(t.TempDir(), "missing.db")
	if db, err := openReadOnly(ctx, missing); err == nil {
		_ = db.Close()
		t.Fatal("read projection opened a missing database")
	}
	if _, err := os.Stat(missing); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("read projection created its database: %v", err)
	}
}

func TestToolSourceProjectionRejectsConflictingOwnership(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	path := filepath.Join(t.TempDir(), "owners.db")
	db := sourceOwnerFixture(t, path)
	defer func() { _ = db.Close() }()
	const providerID = "prov_00000000000000000000000000000001"
	for _, statement := range []string{
		`INSERT INTO agents(agent_id,name) VALUES('agent-one','Agent One')`,
		`INSERT INTO agent_tool_facades(provider_id,agent_id) VALUES('` + providerID + `','agent-one')`,
		`INSERT INTO sandboxes(id,display_name) VALUES('sandbox-one','Sandbox One')`,
		`INSERT INTO sandbox_tool_suite_bindings(provider_id,sandbox_id) VALUES('` + providerID + `','sandbox-one')`,
	} {
		if _, err := db.ExecContext(ctx, statement); err != nil {
			t.Fatal(err)
		}
	}
	owners, err := ListToolSourceOwners(ctx, path)
	if err == nil || !strings.Contains(err.Error(), "conflicting ownership") || owners != nil {
		t.Fatalf("ambiguous ownership returned partial/guessed sources: %#v, %v", owners, err)
	}
}

func sourceOwnerFixture(t *testing.T, path string) *sql.DB {
	t.Helper()
	db, err := sql.Open("sqlite", path)
	if err != nil {
		t.Fatal(err)
	}
	for _, statement := range []string{
		`CREATE TABLE agents(agent_id TEXT,name TEXT)`,
		`CREATE TABLE agent_tool_facades(provider_id TEXT,agent_id TEXT)`,
		`CREATE TABLE sandboxes(id TEXT,display_name TEXT)`,
		`CREATE TABLE sandbox_tool_suite_bindings(provider_id TEXT,sandbox_id TEXT)`,
		`CREATE TABLE sandbox_job_suite_bindings(provider_id TEXT,sandbox_id TEXT)`,
	} {
		if _, err := db.ExecContext(context.Background(), statement); err != nil {
			_ = db.Close()
			t.Fatal(err)
		}
	}
	return db
}

package sqlite

import (
	"context"
	"database/sql"
	"path/filepath"
	"testing"
	"time"

	agentdomain "github.com/orz-i/Mender/backend/internal/agent/domain"
	_ "modernc.org/sqlite"
)

// A stale Sandbox reference is not converted into an unchecked host path.
func TestAgentWorkspaceV9MigrationRefusesOrphanedLegacyBinding(t *testing.T) {
	ctx := context.Background()
	path := filepath.Join(t.TempDir(), "broken-legacy.db")
	s, err := Open(ctx, path)
	if err != nil {
		t.Fatal(err)
	}
	id, err := agentdomain.NewID()
	if err != nil {
		t.Fatal(err)
	}
	a, err := agentdomain.NewAgent(agentdomain.Spec{ID: id, Name: "Fixture", Backend: agentdomain.BackendGrok,
		PluginID: agentdomain.BackendGrok.PluginID(), ExtensionID: "main", WorkspaceRoot: t.TempDir(),
		Isolation: "host", OwnerKind: "local_user", OwnerRef: "os:501", CreatedAt: time.Now()})
	if err != nil {
		t.Fatal(err)
	}
	if err := s.CreateAgent(ctx, a); err != nil {
		t.Fatal(err)
	}
	if err := s.BindWorkspace(ctx, id, "legacy", t.TempDir()); err != nil {
		t.Fatal(err)
	}
	if err := s.GrantWorkspace(ctx, id, "legacy", "prof_fixture"); err != nil {
		t.Fatal(err)
	}
	if err := s.Close(); err != nil {
		t.Fatal(err)
	}
	raw, err := sql.Open("sqlite", path)
	if err != nil {
		t.Fatal(err)
	}
	for _, statement := range []string{
		"ALTER TABLE agent_workspaces RENAME COLUMN workspace_root TO sandbox_id",
		"UPDATE agent_workspaces SET sandbox_id='sbx_00000000000000000000000000000000'",
		"DELETE FROM agent_schema_migrations WHERE version=9",
	} {
		if _, err := raw.ExecContext(ctx, statement); err != nil {
			t.Fatalf("construct v8 fixture %q: %v", statement, err)
		}
	}
	if err := raw.Close(); err != nil {
		t.Fatal(err)
	}
	reopened, err := Open(ctx, path)
	if err == nil {
		_ = reopened.Close()
		t.Fatal("stale Sandbox binding was silently converted")
	}
	raw, err = sql.Open("sqlite", path)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = raw.Close() }()
	var version int
	if err := raw.QueryRowContext(ctx, "SELECT MAX(version) FROM agent_schema_migrations").Scan(&version); err != nil || version != 8 {
		t.Fatalf("failed migration partially advanced schema: version=%d error=%v", version, err)
	}
	var preservedKey, preservedID string
	if err := raw.QueryRowContext(ctx, "SELECT workspace_key,sandbox_id FROM agent_workspaces WHERE agent_id=?", id).Scan(&preservedKey, &preservedID); err != nil {
		t.Fatal(err)
	}
	if preservedKey != "legacy" || preservedID != "sbx_00000000000000000000000000000000" {
		t.Fatalf("migration destroyed owner data: %s / %s", preservedKey, preservedID)
	}
	var grantCount int
	if err := raw.QueryRowContext(ctx, "SELECT COUNT(*) FROM agent_workspace_grants WHERE agent_id=? AND workspace_key='legacy'", id).Scan(&grantCount); err != nil || grantCount != 1 {
		t.Fatalf("migration erased Profile grant: count=%d err=%v", grantCount, err)
	}
}

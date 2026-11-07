package toolhardcut_test

import (
	"context"
	"database/sql"
	"path/filepath"
	"testing"

	"github.com/orz-i/Mender/backend/internal/migration/toolhardcut"
	_ "modernc.org/sqlite"
)

func TestStagedWorkspaceRootAndPlacementCannotBeRetargetedInPlace(t *testing.T) {
	f := localSuiteFixture(t, 1)
	ctx := context.Background()
	reviewed, err := toolhardcut.AssessPath(ctx, f.path)
	if err != nil {
		t.Fatal(err)
	}
	if reviewed.Status != "ready" {
		t.Fatal(reviewed.Conflicts)
	}
	candidate, err := toolhardcut.StageCopy(ctx, f.path, filepath.Join(t.TempDir(), "staged.sqlite"), reviewed.ManifestDigest)
	if err != nil {
		t.Fatal(err)
	}
	db, err := sql.Open("sqlite", candidate.CandidatePath)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = db.Close() }()
	var wsID string
	if err := db.QueryRow("SELECT workspace_id FROM workspace_placements").Scan(&wsID); err != nil {
		t.Fatal(err)
	}
	for _, sqlText := range []string{
		"UPDATE workspaces SET verified_root='/arbitrary' WHERE workspace_id=?",
		"UPDATE workspaces SET owner_ref='attacker' WHERE workspace_id=?",
		"UPDATE workspace_placements SET backend_root='/attacker' WHERE workspace_id=?",
	} {
		if _, err := db.Exec(sqlText, wsID); err == nil {
			t.Fatalf("unapproved Workspace mutation accepted: %s", sqlText)
		}
	}
}

func TestReadOnlyFingerprintTracksAgentGrantHistoryOutsideToolTables(t *testing.T) {
	path := emptyRuntimeFixture(t)
	ctx := context.Background()
	before, err := toolhardcut.AssessPath(ctx, path)
	if err != nil {
		t.Fatal(err)
	}
	db, err := sql.Open("sqlite", path)
	if err != nil {
		t.Fatal(err)
	}
	_, err = db.Exec(`INSERT INTO agent_grant_events(agent_id,profile_id,action,occurred_at)
  VALUES('agent_11111111111111111111111111111111',
   'prof_aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa','grant','now')`)
	if err != nil {
		_ = db.Close()
		t.Fatal(err)
	}
	if err := db.Close(); err != nil {
		t.Fatal(err)
	}
	after, err := toolhardcut.AssessPath(ctx, path)
	if err != nil {
		t.Fatal(err)
	}
	if before.Snapshot.DatabaseFingerprint == after.Snapshot.DatabaseFingerprint ||
		before.ManifestDigest == after.ManifestDigest {
		t.Fatal("concurrent Agent grant history change did not invalidate migration manifest")
	}
	candidate := filepath.Join(t.TempDir(), "candidate.sqlite")
	if _, err := toolhardcut.StageCopy(ctx, path, candidate, before.ManifestDigest); err == nil {
		t.Fatal("stale manifest accepted after unrelated Context write")
	}
}

package toolhardcut_test

import (
	"context"
	"database/sql"
	"os"
	"path/filepath"
	"testing"

	"github.com/orz-i/Mender/backend/internal/migration/toolhardcut"
	_ "modernc.org/sqlite"
)

func TestOnlineBackupIncludesCommittedWALAndProducesSingleFile(t *testing.T) {
	path := emptyRuntimeFixture(t)
	db, err := sql.Open("sqlite", path)
	if err != nil {
		t.Fatal(err)
	}
	db.SetMaxOpenConns(1)
	defer func() { _ = db.Close() }()
	var mode string
	if err := db.QueryRow("PRAGMA journal_mode=WAL").Scan(&mode); err != nil || mode != "wal" {
		t.Fatalf("WAL mode fixture unavailable: %q %v", mode, err)
	}
	_, err = db.Exec(`INSERT INTO agent_grant_events(agent_id,profile_id,action,occurred_at)
  VALUES('agent_11111111111111111111111111111111',
   'prof_aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa','grant','now')`)
	if err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	expected, err := toolhardcut.AssessPath(ctx, path)
	if err != nil {
		t.Fatal(err)
	}
	dest := filepath.Join(t.TempDir(), "backup.sqlite")
	got, err := toolhardcut.BackupSnapshot(ctx, path, dest)
	if err != nil {
		t.Fatal(err)
	}
	if got.ManifestDigest != expected.ManifestDigest {
		t.Fatal("online backup missed a committed WAL write")
	}
	for _, ext := range []string{"-wal", "-shm", "-journal"} {
		if _, err := os.Stat(dest + ext); !os.IsNotExist(err) {
			t.Fatalf("backup is not a standalone SQLite file: %s", ext)
		}
	}
	restored, err := sql.Open("sqlite", dest)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = restored.Close() }()
	var count int
	if err := restored.QueryRow("SELECT COUNT(*) FROM agent_grant_events").Scan(&count); err != nil || count != 1 {
		t.Fatalf("committed Agent grant event missing from backup: %d %v", count, err)
	}
	if err := restored.QueryRow("PRAGMA journal_mode").Scan(&mode); err != nil || mode != "delete" {
		t.Fatalf("backup has WAL dependency: %s %v", mode, err)
	}
}

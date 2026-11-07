package toolhardcut_test

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/orz-i/Mender/backend/internal/migration/toolhardcut"
)

func TestSQLiteOnlineBackupConsistentAndNeverOverwrites(t *testing.T) {
	path := emptyRuntimeFixture(t)
	dest := filepath.Join(t.TempDir(), "snapshot.sqlite")
	got, err := toolhardcut.BackupSnapshot(context.Background(), path, dest)
	if err != nil {
		t.Fatal(err)
	}
	fromSource, err := toolhardcut.AssessPath(context.Background(), path)
	if err != nil {
		t.Fatal(err)
	}
	if got.ManifestDigest != fromSource.ManifestDigest ||
		got.Snapshot.DatabaseFingerprint != fromSource.Snapshot.DatabaseFingerprint {
		t.Fatal("SQLite backup changed canonical assessment")
	}
	st, err := os.Stat(dest)
	if err != nil {
		t.Fatal(err)
	}
	if st.Mode().Perm() != 0600 {
		t.Fatalf("SQLite backup permissions: %v", st.Mode().Perm())
	}
	if _, err := toolhardcut.BackupSnapshot(context.Background(), path, dest); err == nil {
		t.Fatal("existing backup was overwritten")
	}
	if _, err := toolhardcut.BackupSnapshot(context.Background(), path, path); err == nil {
		t.Fatal("backup onto source was permitted")
	}
}

func TestSQLiteOnlineBackupRequiresActualSourceAndDestination(t *testing.T) {
	valid := emptyRuntimeFixture(t)
	if _, err := toolhardcut.BackupSnapshot(context.Background(), "/missing-mender-backup-source", filepath.Join(t.TempDir(), "copy")); err == nil {
		t.Fatal("nonexistent source accepted")
	}
	absentDir := filepath.Join(t.TempDir(), "missing", "copy.sqlite")
	if _, err := toolhardcut.BackupSnapshot(context.Background(), valid, absentDir); err == nil {
		t.Fatal("backup silently created parent directory")
	}
	if _, err := os.Stat(absentDir); !os.IsNotExist(err) {
		t.Fatal("unsafe destination created")
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := toolhardcut.BackupSnapshot(ctx, valid, filepath.Join(t.TempDir(), "copy.sqlite")); err == nil {
		t.Fatal("cancelled backup accepted")
	}
}

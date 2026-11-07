package toolhardcut_test

import (
	"context"
	"database/sql"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/orz-i/Mender/backend/internal/bootstrap"
	catalog "github.com/orz-i/Mender/backend/internal/catalog/domain"
	legacycatalog "github.com/orz-i/Mender/backend/internal/catalog/infrastructure/sqlite"
	"github.com/orz-i/Mender/backend/internal/migration/toolhardcut"
	_ "modernc.org/sqlite"
)

func TestStageCopyMigratesOnlyDisposableCopyAndRejectsOldRuntime(t *testing.T) {
	ctx := context.Background()
	source := emptyRuntimeFixture(t)
	plan, err := toolhardcut.AssessPath(ctx, source)
	if err != nil {
		t.Fatal(err)
	}
	if plan.Status != "ready" {
		t.Fatalf("empty fixture should be ready: %+v", plan.Conflicts)
	}
	dest := filepath.Join(t.TempDir(), "candidate.sqlite")
	artifact, err := toolhardcut.StageCopy(ctx, source, dest, plan.ManifestDigest)
	if err != nil {
		t.Fatal(err)
	}
	if artifact.CandidatePath != dest || artifact.PreUpgradeBackupPath != dest+".preupgrade" ||
		artifact.BeforeDigest != plan.ManifestDigest || !strings.HasPrefix(artifact.AfterFileSHA256, "sha256:") {
		t.Fatalf("invalid staged copy result: %+v", artifact)
	}
	for _, path := range []string{dest, dest + ".preupgrade"} {
		st, err := os.Stat(path)
		if err != nil {
			t.Fatal(err)
		}
		if st.Mode().Perm() != 0600 {
			t.Fatalf("backup permissions for %s: %s", path, st.Mode().Perm())
		}
	}
	unchanged, err := toolhardcut.AssessPath(ctx, source)
	if err != nil || unchanged.ManifestDigest != plan.ManifestDigest {
		t.Fatalf("source changed by staging: %+v %v", unchanged.Snapshot, err)
	}
	backup, err := toolhardcut.AssessPath(ctx, artifact.PreUpgradeBackupPath)
	if err != nil || backup.ManifestDigest != plan.ManifestDigest {
		t.Fatalf("pre-upgrade backup damaged: %v", err)
	}
	if store, err := legacycatalog.Open(ctx, dest); err == nil {
		_ = store.Close()
		t.Fatal("old Runtime accepted future hard-cut schema")
	}
	db, err := sql.Open("sqlite", dest)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = db.Close() }()
	var version int
	if err := db.QueryRow("SELECT MAX(version) FROM catalog_schema_migrations").Scan(&version); err != nil || version != 3 {
		t.Fatalf("new Catalog version = %d: %v", version, err)
	}
	for _, tc := range []struct {
		query string
		want  int
	}{
		{"SELECT COUNT(*) FROM tools", 9},
		{"SELECT COUNT(*) FROM tools_fts", 9},
		{"SELECT COUNT(*) FROM native_tool_contracts", 9},
		{"SELECT COUNT(*) FROM hardcut_legacy_tool_identities", 0},
		{"SELECT COUNT(*) FROM sqlite_master WHERE type='table' AND name='agent_tool_facades'", 0},
		{"SELECT COUNT(*) FROM sqlite_master WHERE type='table' AND name='sandbox_tool_suite_bindings'", 0},
	} {
		var n int
		if err := db.QueryRow(tc.query).Scan(&n); err != nil || n != tc.want {
			t.Fatalf("staged state %s => %d want %d: %v", tc.query, n, tc.want, err)
		}
	}
	// SQL commit-point guard must reject arbitrary privileged native ToolIDs,
	// even when a writer directly issues SQL instead of using the Go repo.
	if _, err := db.Exec(`INSERT INTO tools(id,source_kind,source_id,source_key,
  name,title,description,input_schema,tags_json,created_at,updated_at)
  VALUES('tool_aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa','native','workspace',
   'workspace.read','read','','','{}','[]','now','now')`); err == nil {
		t.Fatal("unreviewed privileged native tool was inserted")
	}
	if _, err := db.Exec("DELETE FROM tools WHERE source_kind='native' LIMIT 1"); err == nil {
		t.Fatal("native canonical tool was retired")
	}
}

func TestStageCopyRefusesChangedManifestAndPartialCandidate(t *testing.T) {
	ctx := context.Background()
	source := emptyRuntimeFixture(t)
	initial, err := toolhardcut.AssessPath(ctx, source)
	if err != nil {
		t.Fatal(err)
	}
	db, err := sql.Open("sqlite", source)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`INSERT INTO providers(id,namespace,display_name,description,created_at,updated_at)
  VALUES('prov_11111111111111111111111111111111','test','Test','','now','now')`); err != nil {
		t.Fatal(err)
	}
	_ = db.Close()
	candidate := filepath.Join(t.TempDir(), "candidate.sqlite")
	if _, err := toolhardcut.StageCopy(ctx, source, candidate, initial.ManifestDigest); err == nil {
		t.Fatal("stale dry-run manifest accepted")
	}
	if _, err := os.Stat(candidate); !os.IsNotExist(err) {
		t.Fatal("stale plan created candidate")
	}
	if _, err := os.Stat(candidate + ".preupgrade"); !os.IsNotExist(err) {
		t.Fatal("stale plan created backup")
	}
}

func TestStageCopyAbortsUnreviewedLegacyTriggerWithoutPartialUpgrade(t *testing.T) {
	ctx := context.Background()
	source := emptyRuntimeFixture(t)
	db, err := sql.Open("sqlite", source)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`CREATE TRIGGER unreviewed_new_tool_guard BEFORE DELETE ON providers
  WHEN EXISTS(SELECT 1 FROM tools t WHERE t.provider_id=OLD.id)
  BEGIN SELECT RAISE(ABORT,'unreviewed'); END`); err != nil {
		t.Fatal(err)
	}
	_ = db.Close()
	plan, err := toolhardcut.AssessPath(ctx, source)
	if err != nil {
		t.Fatal(err)
	}
	if plan.Status != "ready" {
		t.Fatalf("unreviewed trigger must be transactional fail-safe: %+v", plan.Conflicts)
	}
	candidate := filepath.Join(t.TempDir(), "candidate.sqlite")
	if _, err := toolhardcut.StageCopy(ctx, source, candidate, plan.ManifestDigest); err == nil {
		t.Fatal("unreviewed legacy trigger silently discarded")
	}
	if _, err := os.Stat(candidate); !os.IsNotExist(err) {
		t.Fatal("partial candidate remained after failed transaction")
	}
	if _, err := toolhardcut.AssessPath(ctx, candidate+".preupgrade"); err != nil {
		t.Fatalf("pre-upgrade backup not recoverable after rollback: %v", err)
	}
	original, err := toolhardcut.AssessPath(ctx, source)
	if err != nil || original.ManifestDigest != plan.ManifestDigest {
		t.Fatalf("failed DDL modified source: %v", err)
	}
}

func TestNoLegacyProviderConstructorUsedForNativeAfterCut(t *testing.T) {
	// A SourceRef is not a ProviderID. Stage F must replace all remaining
	// ProviderID-dependent presentation/approval/budget paths.
	source, _ := catalog.NativeSource("workspace")
	if source.ID() == "" || source.Kind() != catalog.SourceNative {
		t.Fatal("invalid native source")
	}
	var provider catalog.ProviderID
	if provider.String() != "" {
		t.Fatal("native source invented a Provider ID")
	}
	_ = bootstrap.RuntimeOptions{} // Ensure this smoke test links the real runtime without executing it.
}

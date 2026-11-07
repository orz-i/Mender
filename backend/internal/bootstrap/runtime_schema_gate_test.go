package bootstrap

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	migration "github.com/orz-i/Mender/backend/internal/migration/toolhardcut"
	_ "modernc.org/sqlite"
)

func fileDigest(t *testing.T, path string) [sha256.Size]byte {
	t.Helper()
	// #nosec G304 -- path is a disposable test fixture created under t.TempDir().
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return sha256.Sum256(data)
}

// StageCopy is the actual Stage E cross-context transaction, not a mock v3
// schema. Old Runtime startup must not mutate it or launch any Provider/Agent.
func TestLegacyRuntimeRejectsStagedF4DatabaseBeforeAnyWrites(t *testing.T) {
	ctx := context.Background()
	dir := t.TempDir()
	legacy := filepath.Join(dir, "legacy.db")
	runtime, err := NewRuntime(ctx, RuntimeOptions{DatabasePath: legacy})
	if err != nil {
		t.Fatal(err)
	}
	if err := runtime.Close(); err != nil {
		t.Fatal(err)
	}
	report, err := migration.AssessPath(ctx, legacy)
	if err != nil || report.Status != "ready" {
		t.Fatalf("blank fixture has unresolved migration blockers: %+v %v", report.Conflicts, err)
	}
	candidate, err := migration.StageCopy(ctx, legacy, filepath.Join(dir, "future.db"), report.ManifestDigest)
	if err != nil {
		t.Fatal(err)
	}
	before := fileDigest(t, candidate.CandidatePath)
	reopened, err := NewRuntime(ctx, RuntimeOptions{DatabasePath: candidate.CandidatePath})
	if err == nil {
		_ = reopened.Close()
		t.Fatal("Provider-v2 Runtime accepted F4 candidate schema")
	}
	if !strings.Contains(err.Error(), "typed Tool hard-cut database") {
		t.Fatalf("did not reject future schema before opening Stores: %v", err)
	}
	if after := fileDigest(t, candidate.CandidatePath); after != before {
		t.Fatal("rejected old Runtime modified converted database")
	}
	db, err := sql.Open("sqlite", candidate.CandidatePath)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = db.Close() }()
	var v int
	if err := db.QueryRow("SELECT MAX(version) FROM catalog_schema_migrations").Scan(&v); err != nil || v != 3 {
		t.Fatalf("old runtime damaged migrated Catalog version %d: %v", v, err)
	}
}

func TestLegacyRuntimeRejectsFutureOrPartiallyTypedSchemaWithoutRepair(t *testing.T) {
	ctx := context.Background()
	for _, tc := range []struct {
		name   string
		update string
		reason string
	}{
		{"catalog-future", "INSERT INTO catalog_schema_migrations(version,applied_at) VALUES(3,'test')", "catalog_schema_migrations"},
		{"agent-future", "INSERT INTO agent_schema_migrations(version,applied_at) VALUES(20,'test')", "agent_schema_migrations"},
		{"typed-tool-column-without-version", "ALTER TABLE tools ADD COLUMN source_kind TEXT", "typed Tool source columns"},
		{"corrupt-hardcut-marker", "CREATE TABLE tool_hardcut_schema_migrations(version INTEGER PRIMARY KEY)", "typed Tool hard-cut database"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "legacy.sqlite")
			runtime, err := NewRuntime(ctx, RuntimeOptions{DatabasePath: path})
			if err != nil {
				t.Fatal(err)
			}
			if err := runtime.Close(); err != nil {
				t.Fatal(err)
			}
			db, err := sql.Open("sqlite", path)
			if err != nil {
				t.Fatal(err)
			}
			if _, err := db.Exec(tc.update); err != nil {
				_ = db.Close()
				t.Fatal(err)
			}
			if err := db.Close(); err != nil {
				t.Fatal(err)
			}
			before := fileDigest(t, path)
			opened, err := NewRuntime(ctx, RuntimeOptions{DatabasePath: path})
			if err == nil {
				_ = opened.Close()
				t.Fatal("failed to reject unsafe schema")
			}
			if !strings.Contains(err.Error(), tc.reason) {
				t.Fatalf("wrong startup refusal: %v", err)
			}
			if got := fileDigest(t, path); got != before {
				t.Fatal("startup refusal performed database writes")
			}
		})
	}
}

func TestLegacyRuntimeSchemaPreflightRejectsMissingDatabaseSourceSafely(t *testing.T) {
	ctx := context.Background()
	missing := filepath.Join(t.TempDir(), "never-created.sqlite")
	if err := requireLegacyRuntimeSchema(ctx, missing); err != nil {
		t.Fatalf("new database denied by old runtime preflight: %v", err)
	}
	if _, err := os.Stat(missing); !errors.Is(err, os.ErrNotExist) {
		t.Fatal("inert schema preflight created a database")
	}
}

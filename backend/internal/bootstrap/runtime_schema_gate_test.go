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

// A v3 runtime accepts only a complete, attested typed schema. A valid v3
// database opens without repair or writes beyond normal runtime lifecycle.
func TestV3RuntimeAcceptsFreshTypedDatabase(t *testing.T) {
	ctx := context.Background()
	path := filepath.Join(t.TempDir(), "fresh-v3.db")
	runtime, err := NewRuntime(ctx, RuntimeOptions{DatabasePath: path})
	if err != nil {
		t.Fatalf("fresh v3 runtime: %v", err)
	}
	if err := runtime.Close(); err != nil {
		t.Fatal(err)
	}
	reopened, err := NewRuntime(ctx, RuntimeOptions{DatabasePath: path})
	if err != nil {
		t.Fatalf("reopen valid v3 runtime: %v", err)
	}
	if err := reopened.Close(); err != nil {
		t.Fatal(err)
	}
}

func TestV3RuntimeRejectsFutureOrPartiallyTypedSchemaWithoutRepair(t *testing.T) {
	ctx := context.Background()
	for _, tc := range []struct {
		name   string
		update string
		reason string
	}{
		{"catalog-future", "INSERT INTO catalog_schema_migrations(version,applied_at) VALUES(99,'test')", "catalog_schema_migrations"},
		{"agent-future", "INSERT INTO agent_schema_migrations(version,applied_at) VALUES(99,'test')", "agent_schema_migrations"},
		{"missing-hardcut-marker", "DELETE FROM tool_hardcut_schema_migrations WHERE version=1", "tool_hardcut_schema_migrations"},
		{"legacy-facade-resurrected", "CREATE TABLE agent_tool_facades(agent_id TEXT PRIMARY KEY)", "legacy facade table"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "v3.sqlite")
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

func TestV3RuntimeMissingDatabaseGetsFreshSchema(t *testing.T) {
	ctx := context.Background()
	missing := filepath.Join(t.TempDir(), "new.sqlite")
	if _, err := os.Stat(missing); !errors.Is(err, os.ErrNotExist) {
		t.Fatal("test setup created database")
	}
	runtime, err := NewRuntime(ctx, RuntimeOptions{DatabasePath: missing})
	if err != nil {
		t.Fatalf("new database should get fresh v3 schema: %v", err)
	}
	if err := runtime.Close(); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(missing); err != nil {
		t.Fatal("fresh v3 database was not created")
	}
}

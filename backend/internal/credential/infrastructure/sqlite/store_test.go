package sqlite

import (
	"context"
	"database/sql"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"

	credentialapplication "github.com/orz-i/Mender/backend/internal/credential/application"
	credentialdomain "github.com/orz-i/Mender/backend/internal/credential/domain"
	_ "modernc.org/sqlite"
)

func TestCredentialMetadataRoundTrip(t *testing.T) {
	t.Parallel()

	store := openTestStore(t)
	now := time.Date(2026, 9, 27, 12, 0, 0, 0, time.UTC)
	native, err := credentialdomain.NewCredential(
		mustReference(t, "github", "work"),
		"GitHub Work",
		credentialdomain.SourceNative,
		"",
		now,
	)
	if err != nil {
		t.Fatal(err)
	}
	environment, err := credentialdomain.NewCredential(
		mustReference(t, "search", "prod"),
		"Search Prod",
		credentialdomain.SourceEnvironment,
		"SEARCH_TOKEN",
		now,
	)
	if err != nil {
		t.Fatal(err)
	}

	for _, item := range []credentialdomain.Credential{native, environment} {
		if err := store.Create(context.Background(), item); err != nil {
			t.Fatalf("Create(%s) error = %v", item.Reference(), err)
		}
	}

	loaded, err := store.Get(context.Background(), environment.Reference())
	if err != nil {
		t.Fatal(err)
	}
	if loaded.DisplayName() != "Search Prod" ||
		loaded.Source() != credentialdomain.SourceEnvironment ||
		loaded.EnvironmentVariable() != "SEARCH_TOKEN" {
		t.Fatalf("loaded = %#v", loaded)
	}

	listed, err := store.List(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(listed) != 2 {
		t.Fatalf("List() len = %d", len(listed))
	}

	if err := store.Delete(context.Background(), native.Reference()); err != nil {
		t.Fatal(err)
	}
	if _, err := store.Get(context.Background(), native.Reference()); err == nil {
		t.Fatal("Get(deleted) expected error")
	}
}

func TestCredentialSchemaContainsNoSecretColumns(t *testing.T) {
	t.Parallel()

	store := openTestStore(t)
	rows, err := store.db.QueryContext(
		context.Background(),
		"PRAGMA table_info(credentials)",
	)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = rows.Close() }()

	var columns []string
	for rows.Next() {
		var (
			cid          int
			name         string
			columnType   string
			notNull      int
			defaultValue sql.NullString
			primaryKey   int
		)
		if err := rows.Scan(
			&cid,
			&name,
			&columnType,
			&notNull,
			&defaultValue,
			&primaryKey,
		); err != nil {
			t.Fatal(err)
		}
		columns = append(columns, name)
	}
	if err := rows.Err(); err != nil {
		t.Fatal(err)
	}

	for _, forbidden := range []string{
		"secret",
		"token",
		"password",
		"api_key",
		"value",
		"payload",
	} {
		for _, column := range columns {
			if strings.Contains(strings.ToLower(column), forbidden) {
				t.Fatalf("credentials schema contains forbidden column %q", column)
			}
		}
	}
	for _, required := range []string{
		"reference",
		"display_name",
		"source",
		"environment_variable",
		"created_at",
		"updated_at",
	} {
		if !slices.Contains(columns, required) {
			t.Fatalf("credentials columns = %v, missing %q", columns, required)
		}
	}
}

func TestOpenRejectsFutureCredentialSchemaVersion(t *testing.T) {
	t.Parallel()

	path := filepath.Join(t.TempDir(), "mender.db")
	db, err := sql.Open("sqlite", path)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`CREATE TABLE credential_schema_migrations (
		version INTEGER PRIMARY KEY,
		applied_at TEXT NOT NULL
	)`); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(
		"INSERT INTO credential_schema_migrations(version, applied_at) VALUES (99, 'future')",
	); err != nil {
		t.Fatal(err)
	}
	if err := db.Close(); err != nil {
		t.Fatal(err)
	}

	store, err := Open(context.Background(), path)
	if store != nil {
		_ = store.Close()
	}
	if err == nil || !strings.Contains(err.Error(), "newer than supported version") {
		t.Fatalf("Open() error = %v", err)
	}
}

func TestCredentialRestoreFailsClosedOnMalformedSource(t *testing.T) {
	t.Parallel()

	store := openTestStore(t)
	now := time.Date(2026, 9, 27, 12, 0, 0, 0, time.UTC)
	item, err := credentialdomain.NewCredential(
		mustReference(t, "github", "work"),
		"GitHub Work",
		credentialdomain.SourceNative,
		"",
		now,
	)
	if err != nil {
		t.Fatal(err)
	}
	if err := store.Create(context.Background(), item); err != nil {
		t.Fatal(err)
	}
	if _, err := store.db.ExecContext(
		context.Background(),
		"UPDATE credentials SET source = 'plaintext' WHERE reference = ?",
		item.Reference().String(),
	); err != nil {
		t.Fatal(err)
	}
	if _, err := store.Get(context.Background(), item.Reference()); err == nil {
		t.Fatal("Get() accepted malformed persisted source")
	}
}

func openTestStore(t *testing.T) *Store {
	t.Helper()
	store, err := Open(
		context.Background(),
		filepath.Join(t.TempDir(), "mender.db"),
	)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := store.Close(); err != nil {
			t.Errorf("Close() error = %v", err)
		}
	})
	return store
}

func mustReference(
	t *testing.T,
	namespace string,
	name string,
) credentialdomain.Reference {
	t.Helper()
	ref, err := credentialdomain.NewReference(namespace, name)
	if err != nil {
		t.Fatal(err)
	}
	return ref
}

var _ credentialapplication.Repository = (*Store)(nil)

package sqlite

import (
	"context"
	"database/sql"
	"path/filepath"
	"strings"
	"testing"
	"time"

	credentialdomain "github.com/orz-i/Mender/backend/internal/credential/domain"
)

func TestOAuthCredentialMetadataRoundTripContainsNoSecrets(t *testing.T) {
	store := openTestStore(t)
	now := time.Date(2026, 10, 19, 0, 0, 0, 0, time.UTC)
	ref := mustReference(t, "oauth", "work")
	credential, err := credentialdomain.NewCredential(ref, "OAuth account", credentialdomain.SourceOAuth, "", now)
	if err != nil {
		t.Fatal(err)
	}
	//nolint:gosec // Public OAuth client ID is a nonsecret metadata fixture.
	metadata := credentialdomain.OAuthMetadata{Resource: "https://mcp.example.test/mcp", Issuer: "https://id.example.test", ClientID: "public-desktop", TokenEndpoint: "https://id.example.test/token", Scopes: []string{"read"}, Status: credentialdomain.OAuthConnected}
	credential, err = credential.WithOAuthMetadata(metadata)
	if err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	if err := store.Create(ctx, credential); err != nil {
		t.Fatal(err)
	}
	loaded, err := store.Get(ctx, ref)
	if err != nil {
		t.Fatal(err)
	}
	if loaded.Source() != credentialdomain.SourceOAuth || loaded.OAuthMetadata() == nil || loaded.OAuthMetadata().ClientID != "public-desktop" {
		t.Fatalf("OAuth credential roundtrip failed: %+v", loaded.OAuthMetadata())
	}
	_, err = store.db.ExecContext(ctx, "UPDATE credentials SET oauth_metadata='not json' WHERE reference=?", ref.String())
	if err != nil {
		t.Fatal(err)
	}
	if _, err := store.Get(ctx, ref); err == nil {
		t.Fatal("malformed OAuth state accepted")
	}
}

func TestCredentialV1SchemaMigrationPreservesExistingNativeCredential(t *testing.T) {
	ctx := context.Background()
	path := filepath.Join(t.TempDir(), "legacy-mender.db")
	db, err := sql.Open("sqlite", path)
	if err != nil {
		t.Fatal(err)
	}
	stmts := []string{
		"CREATE TABLE credential_schema_migrations (version INTEGER PRIMARY KEY, applied_at TEXT NOT NULL)",
		"INSERT INTO credential_schema_migrations(version,applied_at) VALUES (1,'2026-01-01')",
		"CREATE TABLE credentials (reference TEXT PRIMARY KEY,display_name TEXT NOT NULL,source TEXT NOT NULL,environment_variable TEXT NOT NULL,created_at TEXT NOT NULL,updated_at TEXT NOT NULL)",
		"INSERT INTO credentials(reference,display_name,source,environment_variable,created_at,updated_at) VALUES ('credential://demo/key','Existing key','native','','2026-01-01T00:00:00Z','2026-01-01T00:00:00Z')",
	}
	for _, sqlStmt := range stmts {
		if _, err := db.ExecContext(ctx, sqlStmt); err != nil {
			t.Fatal(err)
		}
	}
	if err := db.Close(); err != nil {
		t.Fatal(err)
	}
	store, err := Open(ctx, path)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = store.Close() }()
	loaded, err := store.Get(ctx, mustReference(t, "demo", "key"))
	if err != nil || loaded.DisplayName() != "Existing key" || loaded.Source() != credentialdomain.SourceNative {
		t.Fatalf("migration lost existing credential: %+v %v", loaded, err)
	}
	var metadata string
	if err := store.db.QueryRowContext(ctx, "SELECT oauth_metadata FROM credentials WHERE reference='credential://demo/key'").Scan(&metadata); err != nil || strings.TrimSpace(metadata) != "" {
		t.Fatalf("migration changed native credential OAuth metadata: %q %v", metadata, err)
	}
	var version int
	if err := store.db.QueryRowContext(ctx, "SELECT MAX(version) FROM credential_schema_migrations").Scan(&version); err != nil || version != 2 {
		t.Fatalf("version = %d, %v", version, err)
	}
}

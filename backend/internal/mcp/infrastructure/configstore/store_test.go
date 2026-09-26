package configstore

import (
	"context"
	"database/sql"
	"errors"
	"path/filepath"
	"slices"
	"testing"

	catalogdomain "github.com/orz-i/Mender/backend/internal/catalog/domain"
)

func TestConfigRoundTripHTTPAndStdio(t *testing.T) {
	t.Parallel()

	store := openTestStore(t)
	httpID := mustProviderID(t, "prov_00000000000000000000000000000001")
	stdioID := mustProviderID(t, "prov_00000000000000000000000000000002")

	httpConfig := Config{
		ProviderID: httpID,
		Transport:  TransportHTTP,
		Endpoint:   "https://example.test/mcp",
		Headers: map[string]string{
			"Authorization": "Bearer ${EXAMPLE_API_KEY}",
			"X-Client":      "mender",
		},
	}
	stdioConfig := Config{
		ProviderID: stdioID,
		Transport:  TransportStdio,
		Executable: "/usr/local/bin/tool-server",
		Args:       []string{"--mode", "stdio"},
		Directory:  "/tmp/work",
	}

	for _, config := range []Config{httpConfig, stdioConfig} {
		if err := store.Save(context.Background(), config); err != nil {
			t.Fatalf("Save(%s) error = %v", config.ProviderID, err)
		}
	}

	loadedHTTP, err := store.Get(context.Background(), httpID)
	if err != nil {
		t.Fatal(err)
	}
	if loadedHTTP.Endpoint != httpConfig.Endpoint ||
		loadedHTTP.Transport != TransportHTTP ||
		loadedHTTP.Headers["Authorization"] != "Bearer ${EXAMPLE_API_KEY}" ||
		loadedHTTP.Headers["X-Client"] != "mender" {
		t.Fatalf("HTTP config = %+v", loadedHTTP)
	}

	loadedStdio, err := store.Get(context.Background(), stdioID)
	if err != nil {
		t.Fatal(err)
	}
	if loadedStdio.Executable != stdioConfig.Executable ||
		!slices.Equal(loadedStdio.Args, stdioConfig.Args) ||
		loadedStdio.Directory != stdioConfig.Directory {
		t.Fatalf("stdio config = %+v", loadedStdio)
	}

	listed, err := store.List(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(listed) != 2 {
		t.Fatalf("List() = %+v", listed)
	}

	if err := store.Delete(context.Background(), httpID); err != nil {
		t.Fatal(err)
	}
	if _, err := store.Get(context.Background(), httpID); !errors.Is(err, ErrNotFound) {
		t.Fatalf("Get(deleted) error = %v", err)
	}
}

func TestConfigRejectsPersistedSensitiveHeaderValue(t *testing.T) {
	t.Parallel()

	store := openTestStore(t)
	err := store.Save(context.Background(), Config{
		ProviderID: mustProviderID(t, "prov_00000000000000000000000000000003"),
		Transport:  TransportHTTP,
		Endpoint:   "https://example.test/mcp",
		Headers: map[string]string{
			"Authorization": "Bearer plaintext-secret",
		},
	})
	if err == nil {
		t.Fatal("Save() expected plaintext Authorization rejection")
	}
}

func TestConfigSchemaContainsNoSecretOrEnvironmentColumns(t *testing.T) {
	t.Parallel()

	store := openTestStore(t)
	rows, err := store.db.QueryContext(context.Background(), "PRAGMA table_info(mcp_provider_configs)")
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
		"env",
		"environment",
		"token",
		"password",
		"secret",
		"credential",
	} {
		if slices.Contains(columns, forbidden) {
			t.Fatalf("config schema contains forbidden column %q: %v", forbidden, columns)
		}
	}
}

func openTestStore(t *testing.T) *Store {
	t.Helper()

	store, err := Open(context.Background(), filepath.Join(t.TempDir(), "mender.db"))
	if err != nil {
		t.Fatalf("Open() error = %v", err)
	}
	t.Cleanup(func() {
		if err := store.Close(); err != nil {
			t.Errorf("Close() error = %v", err)
		}
	})
	return store
}

func mustProviderID(t *testing.T, raw string) catalogdomain.ProviderID {
	t.Helper()
	id, err := catalogdomain.ParseProviderID(raw)
	if err != nil {
		t.Fatal(err)
	}
	return id
}

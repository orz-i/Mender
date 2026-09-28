package configstore

import (
	"context"
	"database/sql"
	"errors"
	"path/filepath"
	"slices"
	"testing"

	catalogdomain "github.com/orz-i/Mender/backend/internal/catalog/domain"
	credentialdomain "github.com/orz-i/Mender/backend/internal/credential/domain"
	providerplugin "github.com/orz-i/Mender/backend/internal/providerplugin"
)

func TestStoreRoundTripListAndDelete(t *testing.T) {
	t.Parallel()

	store := openTestStore(t)
	first := testConfiguration(
		t,
		"prov_00000000000000000000000000000021",
		"io.mender.provider.mcp",
		`{"transport":{"kind":"stdio","executable":"server"}}`,
	)
	second := testConfiguration(
		t,
		"prov_00000000000000000000000000000022",
		"com.example.provider.openapi",
		`{"document":"https://example.test/openapi.json"}`,
	)

	for _, config := range []providerplugin.Configuration{second, first} {
		if err := store.Save(context.Background(), config); err != nil {
			t.Fatalf("Save(%s) error = %v", config.ProviderID, err)
		}
	}

	loaded, err := store.GetProviderConfig(context.Background(), first.ProviderID)
	if err != nil {
		t.Fatal(err)
	}
	if loaded.PluginID != first.PluginID ||
		loaded.ConfigVersion != first.ConfigVersion ||
		loaded.Revision != first.Revision ||
		string(loaded.Config) != string(first.Config) {
		t.Fatalf("GetProviderConfig() = %+v, want %+v", loaded, first)
	}

	listed, err := store.List(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(listed) != 2 {
		t.Fatalf("List() len = %d, want 2", len(listed))
	}
	if listed[0].ProviderID != first.ProviderID ||
		listed[1].ProviderID != second.ProviderID {
		t.Fatalf(
			"List() order = [%s %s]",
			listed[0].ProviderID,
			listed[1].ProviderID,
		)
	}

	if err := store.Delete(context.Background(), first.ProviderID); err != nil {
		t.Fatal(err)
	}
	if _, err := store.GetProviderConfig(
		context.Background(),
		first.ProviderID,
	); !errors.Is(err, ErrNotFound) {
		t.Fatalf("GetProviderConfig(deleted) error = %v", err)
	}
}

func TestStoreRoundTripsNormalizedConfigMetadata(t *testing.T) {
	t.Parallel()

	store := openTestStore(t)
	config := testConfiguration(
		t,
		"prov_00000000000000000000000000000027",
		"dev.example.echo",
		`{"credentialRef":"credential://demo/token"}`,
	)
	ref, err := credentialdomain.ParseReference("credential://demo/token")
	if err != nil {
		t.Fatal(err)
	}
	metadata, err := providerplugin.NewConfigMetadataWithSummary(
		[]credentialdomain.Reference{ref, ref},
		"Example API · production",
	)
	if err != nil {
		t.Fatal(err)
	}
	config, err = config.WithMetadata(metadata)
	if err != nil {
		t.Fatal(err)
	}
	if err := store.Save(context.Background(), config); err != nil {
		t.Fatal(err)
	}

	loaded, err := store.GetProviderConfig(context.Background(), config.ProviderID)
	if err != nil {
		t.Fatal(err)
	}
	got, ok := loaded.ConfigMetadata()
	if !ok {
		t.Fatal("ConfigMetadata() missing after round trip")
	}
	if len(got.CredentialRefs) != 1 || got.CredentialRefs[0] != ref {
		t.Fatalf("CredentialRefs = %v, want [%s]", got.CredentialRefs, ref)
	}
	if got.ConnectionSummary != metadata.ConnectionSummary {
		t.Fatalf("ConnectionSummary = %q, want %q", got.ConnectionSummary, metadata.ConnectionSummary)
	}
}

func TestStoreRoundTripsExternalInstallationPin(t *testing.T) {
	t.Parallel()

	store := openTestStore(t)
	providerID, err := catalogdomain.ParseProviderID(
		"prov_00000000000000000000000000000026",
	)
	if err != nil {
		t.Fatal(err)
	}
	pluginID, err := providerplugin.ParseID("dev.example.echo")
	if err != nil {
		t.Fatal(err)
	}
	installationID, err := providerplugin.ParseInstallationID(
		"pinst_0123456789abcdef0123456789abcdef",
	)
	if err != nil {
		t.Fatal(err)
	}
	config, err := providerplugin.NewInstalledConfiguration(
		providerID,
		pluginID,
		installationID,
		1,
		[]byte(`{"name":"demo"}`),
	)
	if err != nil {
		t.Fatal(err)
	}
	if err := store.Save(context.Background(), config); err != nil {
		t.Fatal(err)
	}

	loaded, err := store.GetProviderConfig(context.Background(), providerID)
	if err != nil {
		t.Fatal(err)
	}
	if loaded.InstallationID == nil ||
		*loaded.InstallationID != installationID ||
		loaded.PluginID != pluginID {
		t.Fatalf("GetProviderConfig() = %+v", loaded)
	}
}

func TestImportOnceDoesNotOverwriteGenericRowsOrRepeat(t *testing.T) {
	t.Parallel()

	store := openTestStore(t)
	current := testConfiguration(
		t,
		"prov_00000000000000000000000000000023",
		"io.mender.provider.mcp",
		`{"transport":{"kind":"stdio","executable":"current"}}`,
	)
	legacySameProvider := testConfiguration(
		t,
		current.ProviderID.String(),
		"io.mender.provider.mcp",
		`{"transport":{"kind":"stdio","executable":"legacy"}}`,
	)
	imported := testConfiguration(
		t,
		"prov_00000000000000000000000000000024",
		"io.mender.provider.mcp",
		`{"transport":{"kind":"stdio","executable":"imported"}}`,
	)

	if err := store.Save(context.Background(), current); err != nil {
		t.Fatal(err)
	}
	if err := store.ImportOnce(
		context.Background(),
		"legacy-mcp-v3",
		[]providerplugin.Configuration{legacySameProvider, imported},
	); err != nil {
		t.Fatal(err)
	}

	loadedCurrent, err := store.GetProviderConfig(context.Background(), current.ProviderID)
	if err != nil {
		t.Fatal(err)
	}
	if string(loadedCurrent.Config) != string(current.Config) {
		t.Fatalf(
			"ImportOnce() overwrote generic config: %s",
			loadedCurrent.Config,
		)
	}
	if _, err := store.GetProviderConfig(
		context.Background(),
		imported.ProviderID,
	); err != nil {
		t.Fatalf("imported config missing: %v", err)
	}

	later := testConfiguration(
		t,
		"prov_00000000000000000000000000000025",
		"io.mender.provider.mcp",
		`{"transport":{"kind":"stdio","executable":"later"}}`,
	)
	if err := store.ImportOnce(
		context.Background(),
		"legacy-mcp-v3",
		[]providerplugin.Configuration{later},
	); err != nil {
		t.Fatal(err)
	}
	if _, err := store.GetProviderConfig(
		context.Background(),
		later.ProviderID,
	); !errors.Is(err, ErrNotFound) {
		t.Fatalf("second ImportOnce() unexpectedly imported new row: %v", err)
	}
}

func TestProviderConfigSchemaIsProtocolAgnostic(t *testing.T) {
	t.Parallel()

	store := openTestStore(t)
	rows, err := store.db.QueryContext(
		context.Background(),
		"PRAGMA table_info(provider_configs)",
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

	for _, expected := range []string{
		"provider_id",
		"plugin_id",
		"plugin_installation_id",
		"config_version",
		"revision",
		"config_json",
		"credential_refs_json",
		"connection_summary",
	} {
		if !slices.Contains(columns, expected) {
			t.Fatalf("provider config schema missing %q: %v", expected, columns)
		}
	}
	for _, forbidden := range []string{
		"transport",
		"endpoint",
		"executable",
		"headers",
		"credential_value",
		"secret",
	} {
		if slices.Contains(columns, forbidden) {
			t.Fatalf("provider config schema contains protocol field %q", forbidden)
		}
	}
}

func openTestStore(t *testing.T) *Store {
	t.Helper()
	store, err := Open(
		context.Background(),
		filepath.Join(t.TempDir(), "mender.db"),
	)
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

func testConfiguration(
	t *testing.T,
	rawProviderID string,
	rawPluginID string,
	rawConfig string,
) providerplugin.Configuration {
	t.Helper()

	providerID, err := catalogdomain.ParseProviderID(rawProviderID)
	if err != nil {
		t.Fatal(err)
	}
	pluginID, err := providerplugin.ParseID(rawPluginID)
	if err != nil {
		t.Fatal(err)
	}
	config, err := providerplugin.NewConfiguration(
		providerID,
		pluginID,
		1,
		[]byte(rawConfig),
	)
	if err != nil {
		t.Fatal(err)
	}
	return config
}

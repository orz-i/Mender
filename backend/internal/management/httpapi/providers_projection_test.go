package httpapi

import (
	"encoding/json"
	"testing"
	"time"

	catalogdomain "github.com/orz-i/Mender/backend/internal/catalog/domain"
	providerplugin "github.com/orz-i/Mender/backend/internal/providerplugin"
)

func TestProviderProjectionRedactsHistoricalSensitiveConfig(t *testing.T) {
	t.Parallel()

	providerID, err := catalogdomain.ParseProviderID(
		"prov_00000000000000000000000000000096",
	)
	if err != nil {
		t.Fatal(err)
	}
	namespace, err := catalogdomain.NewNamespace("projection")
	if err != nil {
		t.Fatal(err)
	}
	provider, err := catalogdomain.NewProvider(
		providerID,
		namespace,
		"Projection",
		"",
		time.Date(2026, 9, 28, 12, 0, 0, 0, time.UTC),
	)
	if err != nil {
		t.Fatal(err)
	}
	pluginID, err := providerplugin.ParseID("dev.example.projection")
	if err != nil {
		t.Fatal(err)
	}
	installationID, err := providerplugin.ParseInstallationID(
		"pinst_00000000000000000000000000000096",
	)
	if err != nil {
		t.Fatal(err)
	}
	config := providerplugin.Configuration{
		ProviderID:     providerID,
		PluginID:       pluginID,
		InstallationID: &installationID,
		ConfigVersion:  1,
		Revision:       "legacy",
		Config: json.RawMessage(
			`{"endpoint":"https://example.test","apiKey":"plaintext-secret","credentialRef":"credential://demo/token"}`,
		),
	}
	descriptor := providerplugin.Descriptor{
		ID:            pluginID,
		Version:       "2.0.0",
		DisplayName:   "Projection Plugin",
		ConfigVersion: 1,
		ConfigSchema: json.RawMessage(
			`{"type":"object","properties":{"credentialRef":{"type":"string","x-mender-control":"credential"}}}`,
		),
	}

	result := providerProjection(provider, &config, &descriptor, 0)
	projected, ok := result["config"].(map[string]any)
	if !ok {
		t.Fatalf("projected config = %#v", result["config"])
	}
	if _, exists := projected["apiKey"]; exists {
		t.Fatalf("sensitive apiKey leaked: %#v", projected)
	}
	if projected["credentialRef"] != "credential://demo/token" {
		t.Fatalf("credentialRef = %#v", projected["credentialRef"])
	}
	if result["pluginVersion"] != "2.0.0" ||
		result["pluginDisplayName"] != "Projection Plugin" {
		t.Fatalf("plugin metadata = %#v", result)
	}
}

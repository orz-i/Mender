package binding

import (
	"encoding/json"
	"testing"
	"time"

	catalogdomain "github.com/orz-i/Mender/backend/internal/catalog/domain"
	providerplugin "github.com/orz-i/Mender/backend/internal/providerplugin"
)

func TestProviderProjectionUsesExternalDescriptorAndSafeConfig(t *testing.T) {
	t.Parallel()

	providerID, err := catalogdomain.ParseProviderID(
		"prov_00000000000000000000000000000095",
	)
	if err != nil {
		t.Fatal(err)
	}
	namespace, err := catalogdomain.NewNamespace("desktop-projection")
	if err != nil {
		t.Fatal(err)
	}
	provider, err := catalogdomain.NewProvider(
		providerID,
		namespace,
		"Desktop Projection",
		"",
		time.Date(2026, 9, 28, 12, 0, 0, 0, time.UTC),
	)
	if err != nil {
		t.Fatal(err)
	}
	pluginID, err := providerplugin.ParseID("dev.example.desktop")
	if err != nil {
		t.Fatal(err)
	}
	installationID, err := providerplugin.ParseInstallationID(
		"pinst_00000000000000000000000000000095",
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
			`{"apiKey":"plaintext-secret","credentialRef":"credential://demo/token"}`,
		),
	}
	descriptor := providerplugin.Descriptor{
		ID:            pluginID,
		Version:       "3.1.0",
		DisplayName:   "Desktop External",
		ConfigVersion: 1,
		ConfigSchema: json.RawMessage(
			`{"type":"object","properties":{"credentialRef":{"type":"string","x-mender-control":"credential"}}}`,
		),
	}

	result := providerProjection(provider, &config, &descriptor, 0)
	if _, exists := result.Config["apiKey"]; exists {
		t.Fatalf("sensitive apiKey leaked: %#v", result.Config)
	}
	if result.Config["credentialRef"] != "credential://demo/token" {
		t.Fatalf("credentialRef = %#v", result.Config["credentialRef"])
	}
	if result.PluginVersion != "3.1.0" ||
		result.PluginDisplayName != "Desktop External" {
		t.Fatalf("plugin metadata = %+v", result)
	}
}

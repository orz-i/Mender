package providerplugin

import (
	"testing"

	catalogdomain "github.com/orz-i/Mender/backend/internal/catalog/domain"
)

func TestNewConfigurationBuildsDeterministicRevision(t *testing.T) {
	t.Parallel()

	providerID, err := catalogdomain.ParseProviderID("prov_00000000000000000000000000000010")
	if err != nil {
		t.Fatal(err)
	}
	pluginID, err := ParseID("io.mender.provider.mcp")
	if err != nil {
		t.Fatal(err)
	}
	raw := []byte(`{"transport":{"kind":"stdio","executable":"server"}}`)

	first, err := NewConfiguration(providerID, pluginID, 1, raw)
	if err != nil {
		t.Fatal(err)
	}
	second, err := NewConfiguration(providerID, pluginID, 1, raw)
	if err != nil {
		t.Fatal(err)
	}
	if first.Revision == "" || first.Revision != second.Revision {
		t.Fatalf("revisions = %q and %q", first.Revision, second.Revision)
	}

	raw[0] = '['
	if string(first.Config) == string(raw) {
		t.Fatal("NewConfiguration() retained caller-owned config bytes")
	}
}

func TestConfigurationValidate(t *testing.T) {
	t.Parallel()

	providerID, err := catalogdomain.ParseProviderID("prov_00000000000000000000000000000001")
	if err != nil {
		t.Fatal(err)
	}
	pluginID, err := ParseID("io.mender.provider.mcp")
	if err != nil {
		t.Fatal(err)
	}

	valid := Configuration{
		ProviderID:    providerID,
		PluginID:      pluginID,
		ConfigVersion: 1,
		Revision:      "sha256:abc",
		Config:        []byte(`{"transport":{"kind":"stdio"}}`),
	}
	if err := valid.Validate(); err != nil {
		t.Fatalf("Validate() error = %v", err)
	}

	tests := []struct {
		name   string
		mutate func(*Configuration)
	}{
		{
			name: "invalid provider",
			mutate: func(config *Configuration) {
				config.ProviderID = catalogdomain.ProviderID("bad")
			},
		},
		{
			name: "invalid plugin",
			mutate: func(config *Configuration) {
				config.PluginID = ID("mcp")
			},
		},
		{
			name: "invalid config version",
			mutate: func(config *Configuration) {
				config.ConfigVersion = 0
			},
		},
		{
			name: "empty revision",
			mutate: func(config *Configuration) {
				config.Revision = ""
			},
		},
		{
			name: "invalid json",
			mutate: func(config *Configuration) {
				config.Config = []byte("{")
			},
		},
	}

	for _, test := range tests {
		test := test
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			config := valid
			test.mutate(&config)
			if err := config.Validate(); err == nil {
				t.Fatal("Validate() error = nil, want validation error")
			}
		})
	}
}

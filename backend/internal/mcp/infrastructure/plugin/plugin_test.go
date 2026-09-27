package plugin

import (
	"context"
	"encoding/json"
	"testing"

	providerplugin "github.com/orz-i/Mender/backend/internal/providerplugin"
)

func TestPluginDescriptor(t *testing.T) {
	t.Parallel()

	descriptor := New().Descriptor()
	if descriptor.ID != ID {
		t.Fatalf("Descriptor().ID = %s, want %s", descriptor.ID, ID)
	}
	if descriptor.ConfigVersion != ConfigVersion {
		t.Fatalf(
			"Descriptor().ConfigVersion = %d, want %d",
			descriptor.ConfigVersion,
			ConfigVersion,
		)
	}
	if err := descriptor.Validate(); err != nil {
		t.Fatalf("Descriptor().Validate() error = %v", err)
	}
}

func TestPluginOpenRejectsUnsupportedConfigVersionBeforeActivation(t *testing.T) {
	t.Parallel()

	_, err := New().Open(context.Background(), providerplugin.OpenRequest{
		ConfigVersion: ConfigVersion + 1,
		Config: json.RawMessage(
			`{"transport":{"kind":"stdio","executable":"server"}}`,
		),
	})
	if err == nil {
		t.Fatal("Open() error = nil, want unsupported version")
	}
}

package plugin

import (
	"context"
	"encoding/json"
	"testing"

	credentialdomain "github.com/orz-i/Mender/backend/internal/credential/domain"
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

func TestPluginInspectConfigReturnsCredentialMetadata(t *testing.T) {
	t.Parallel()

	ref, err := credentialdomain.NewReference("demo", "token")
	if err != nil {
		t.Fatal(err)
	}
	raw, err := json.Marshal(Config{
		Transport: TransportConfig{
			Kind:     TransportStreamableHTTP,
			Endpoint: "https://example.test/mcp",
			CredentialHeaders: []CredentialHeader{
				{Name: "Authorization", Scheme: "Bearer", Reference: ref},
			},
		},
	})
	if err != nil {
		t.Fatal(err)
	}

	metadata, err := New().InspectConfig(context.Background(), raw)
	if err != nil {
		t.Fatalf("InspectConfig() error = %v", err)
	}
	if len(metadata.CredentialRefs) != 1 || metadata.CredentialRefs[0] != ref {
		t.Fatalf("CredentialRefs = %v, want [%s]", metadata.CredentialRefs, ref)
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

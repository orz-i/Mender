package plugin

import (
	"encoding/json"
	"testing"

	credentialdomain "github.com/orz-i/Mender/backend/internal/credential/domain"
)

func TestDecodeConfigTransportVariants(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		raw  string
		kind TransportKind
	}{
		{
			name: "streamable http",
			raw:  `{"transport":{"kind":"streamable-http","endpoint":"https://example.test/mcp"}}`,
			kind: TransportStreamableHTTP,
		},
		{
			name: "stdio",
			raw:  `{"transport":{"kind":"stdio","executable":"mcp-server","args":["--stdio"]}}`,
			kind: TransportStdio,
		},
	}

	for _, test := range tests {
		test := test
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			config, err := DecodeConfig(json.RawMessage(test.raw))
			if err != nil {
				t.Fatalf("DecodeConfig() error = %v", err)
			}
			if config.Transport.Kind != test.kind {
				t.Fatalf(
					"transport kind = %q, want %q",
					config.Transport.Kind,
					test.kind,
				)
			}
		})
	}
}

func TestDecodeConfigRejectsUnknownAndMixedFields(t *testing.T) {
	t.Parallel()

	tests := []string{
		`{"transport":{"kind":"stdio","executable":"server","unknown":true}}`,
		`{"transport":{"kind":"stdio","executable":"server","endpoint":"https://example.test/mcp"}}`,
		`{"transport":{"kind":"streamable-http","endpoint":"https://example.test/mcp","executable":"server"}}`,
	}
	for _, raw := range tests {
		raw := raw
		t.Run(raw, func(t *testing.T) {
			t.Parallel()
			if _, err := DecodeConfig(json.RawMessage(raw)); err == nil {
				t.Fatal("DecodeConfig() error = nil, want validation error")
			}
		})
	}
}

func TestConfigRequiredCredentialRefsAreUniqueAndSorted(t *testing.T) {
	t.Parallel()

	first, err := credentialdomain.NewReference("demo", "alpha")
	if err != nil {
		t.Fatal(err)
	}
	second, err := credentialdomain.NewReference("demo", "beta")
	if err != nil {
		t.Fatal(err)
	}

	config := Config{
		Transport: TransportConfig{
			Kind: TransportStreamableHTTP,
			CredentialHeaders: []CredentialHeader{
				{Name: "X-Beta", Reference: second},
				{Name: "X-Alpha", Reference: first},
				{Name: "X-Alpha-Again", Reference: first},
			},
		},
	}

	refs := config.RequiredCredentialRefs()
	if len(refs) != 2 {
		t.Fatalf("RequiredCredentialRefs() len = %d, want 2", len(refs))
	}
	if refs[0] != first || refs[1] != second {
		t.Fatalf("RequiredCredentialRefs() = %v, want [%s %s]", refs, first, second)
	}
}

func TestConfigRejectsDuplicateHeaderPlacement(t *testing.T) {
	t.Parallel()

	ref, err := credentialdomain.NewReference("demo", "token")
	if err != nil {
		t.Fatal(err)
	}
	config := Config{
		Transport: TransportConfig{
			Kind:     TransportStreamableHTTP,
			Endpoint: "https://example.test/mcp",
			Headers: map[string]string{
				"Authorization": "legacy-template",
			},
			CredentialHeaders: []CredentialHeader{
				{Name: "authorization", Scheme: "Bearer", Reference: ref},
			},
		},
	}
	if err := config.Validate(); err == nil {
		t.Fatal("Validate() error = nil, want duplicate header error")
	}
}

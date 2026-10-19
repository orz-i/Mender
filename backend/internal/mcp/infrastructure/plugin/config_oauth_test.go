package plugin

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	credentialdomain "github.com/orz-i/Mender/backend/internal/credential/domain"
	providerplugin "github.com/orz-i/Mender/backend/internal/providerplugin"
)

func TestOAuthBindingRequiresValidResourceCredential(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name      string
		raw       string
		wantError bool
	}{
		{"oauth HTTP", `{"transport":{"kind":"streamable-http","endpoint":"https://example.test/mcp","oauthCredentialRef":"credential://oauth/work"}}`, false},
		{"bad ref", `{"transport":{"kind":"streamable-http","endpoint":"https://example.test/mcp","oauthCredentialRef":"invalid"}}`, true},
		{"oauth on stdio", `{"transport":{"kind":"stdio","executable":"mcp-server","oauthCredentialRef":"credential://oauth/work"}}`, true},
		{"static authorization collision", `{"transport":{"kind":"streamable-http","endpoint":"https://example.test/mcp","headers":{"Authorization":"Bearer \${TOKEN}"},"oauthCredentialRef":"credential://oauth/work"}}`, true},
		{"bound authorization collision", `{"transport":{"kind":"streamable-http","endpoint":"https://example.test/mcp","credentialHeaders":[{"name":"authorization","scheme":"Bearer","credentialRef":"credential://demo/token"}],"oauthCredentialRef":"credential://oauth/work"}}`, true},
		{"unrelated header permitted", `{"transport":{"kind":"streamable-http","endpoint":"https://example.test/mcp","credentialHeaders":[{"name":"X-API-Key","credentialRef":"credential://demo/key"}],"oauthCredentialRef":"credential://oauth/work"}}`, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg, err := DecodeConfig(json.RawMessage(tt.raw))
			if (err != nil) != tt.wantError {
				t.Fatalf("DecodeConfig() = %+v, %v; wantError %t", cfg, err, tt.wantError)
			}
		})
	}
}

func TestOAuthBindingParticipatesInRequiredCredentialProof(t *testing.T) {
	t.Parallel()
	ref, err := credentialdomain.NewReference("oauth", "work")
	if err != nil {
		t.Fatal(err)
	}
	cfg := Config{Transport: TransportConfig{Kind: TransportStreamableHTTP,
		Endpoint: "https://example.test/mcp", OAuthCredentialRef: ref}}
	refs := cfg.RequiredCredentialRefs()
	if len(refs) != 1 || refs[0] != ref {
		t.Fatalf("RequiredCredentialRefs = %v", refs)
	}
	metadata, err := New().InspectConfig(context.Background(), mustOAuthJSON(t, cfg))
	if err != nil || len(metadata.CredentialRefs) != 1 || metadata.CredentialRefs[0] != ref {
		t.Fatalf("InspectConfig = %+v, %v", metadata, err)
	}
}

func TestOAuthProviderFailsClosedWithoutTokenSource(t *testing.T) {
	t.Parallel()
	//nolint:gosec // Synthetic Credential Reference is a nonsecret fixture.
	cfg := Config{Transport: TransportConfig{Kind: TransportStreamableHTTP,
		Endpoint: "https://example.test/mcp", OAuthCredentialRef: "credential://oauth/work"}}
	_, err := New().Open(context.Background(), providerplugin.OpenRequest{
		ConfigVersion: ConfigVersion, Config: mustOAuthJSON(t, cfg),
	})
	if err == nil || !strings.Contains(err.Error(), "OAuth token source") {
		t.Fatalf("Open() error = %v, want explicit fail-closed auth requirement", err)
	}
}

func mustOAuthJSON(t *testing.T, cfg Config) json.RawMessage {
	t.Helper()
	raw, err := json.Marshal(cfg)
	if err != nil {
		t.Fatal(err)
	}
	return raw
}

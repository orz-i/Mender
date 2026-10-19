package httpapi

import (
	"context"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"

	"github.com/orz-i/Mender/backend/internal/bootstrap"
)

// A rejected OAuth Provider configuration is an operator-fixable 400, not an
// internal server fault. Neither a transient Provider nor its config may be
// left behind after validation failure.
func TestOAuthProviderConfigValidationReturnsBadRequestWithoutWrites(t *testing.T) {
	t.Parallel()
	runtime, err := bootstrap.NewRuntime(context.Background(), bootstrap.RuntimeOptions{
		DatabasePath: filepath.Join(t.TempDir(), "validation.db"),
	})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = runtime.Close() })
	server := httptest.NewServer(New(runtime))
	t.Cleanup(server.Close)

	cases := []struct {
		name      string
		transport map[string]any
	}{
		{
			name: "plaintext authorization header",
			transport: map[string]any{
				"kind": "streamable-http", "endpoint": "https://example.test/mcp",
				"oauthCredentialRef": "credential://qaoauth/work",
				// #nosec G101 -- synthetic sentinel string to exercise rejection.
				"headers": map[string]any{"Authorization": "Bearer qa-test-value"},
			},
		},
		{
			name: "oauth conflicts with inventory-backed authorization header",
			//nolint:gosec // Credential References are non-secret, synthetic test identifiers.
			transport: map[string]any{
				"kind": "streamable-http", "endpoint": "https://example.test/mcp",
				"oauthCredentialRef": "credential://qaoauth/work",
				"credentialHeaders": []map[string]any{
					//nolint:gosec // Synthetic Credential Reference, not a secret value.
					{"name": "Authorization", "scheme": "Bearer", "credentialRef": "credential://qaoauth/static"},
				},
			},
		},
		{
			name: "stdio oauth attachment",
			//nolint:gosec // Synthetic Credential Reference, not a secret value.
			transport: map[string]any{
				"kind": "stdio", "executable": "/usr/bin/env",
				"oauthCredentialRef": "credential://qaoauth/work",
			},
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			response := requestJSON(t, server.Client(), http.MethodPost, server.URL+"/api/v1/providers", map[string]any{
				"namespace":   "qa-provider-validation",
				"displayName": "Rejected Provider",
				"pluginId":    "io.mender.provider.mcp",
				"config":      map[string]any{"transport": tc.transport},
			}, http.StatusBadRequest)
			if response["code"] != "validation_error" {
				t.Fatalf("wrong error for invalid Provider config: %#v", response)
			}
			providers, err := runtime.Catalog.ListProviders.Execute(context.Background())
			if err != nil {
				t.Fatal(err)
			}
			if len(providers) != 0 {
				t.Fatalf("invalid Provider config left %d persisted Providers", len(providers))
			}
		})
	}
}

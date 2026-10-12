package httpapi

import (
	"context"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"

	"github.com/orz-i/Mender/backend/internal/bootstrap"
)

// GET is purely discovery. It cannot activate AnySearch or change local state.
func TestReviewedIntegrationOfferingsHTTPIsInert(t *testing.T) {
	r, err := bootstrap.NewRuntime(context.Background(), bootstrap.RuntimeOptions{DatabasePath: filepath.Join(t.TempDir(), "integrations.db")})
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = r.Close() }()
	server := httptest.NewServer(New(r))
	defer server.Close()
	endpoint := server.URL + "/api/v1"
	for i := 0; i < 2; i++ {
		item := requestJSON(t, server.Client(), http.MethodGet, endpoint+"/integrations", nil, http.StatusOK)
		offerings := item["integrations"].([]any)
		if len(offerings) != 1 {
			t.Fatalf("unexpected offerings: %v", item)
		}
		anysearch := offerings[0].(map[string]any)
		if anysearch["offeringId"] != "io.mender.integration.anysearch" ||
			anysearch["driverPluginId"] != "io.mender.provider.mcp" ||
			anysearch["endpoint"] != "https://api.anysearch.com/mcp" ||
			anysearch["authentication"] != "optional-bearer" {
			t.Fatalf("wrong reviewed preset: %v", anysearch)
		}
		if _, exists := anysearch["credentialRef"]; exists {
			t.Fatal("offering leaked a credential")
		}
		providers := requestJSON(t, server.Client(), http.MethodGet, endpoint+"/providers", nil, http.StatusOK)
		if len(providers["providers"].([]any)) != 0 {
			t.Fatal("offering listing activated a Provider")
		}
	}
}

package binding

import (
	"context"
	"path/filepath"
	"testing"

	"github.com/orz-i/Mender/backend/internal/bootstrap"
)

func TestListIntegrationOfferingsIsInert(t *testing.T) {
	r, err := bootstrap.NewRuntime(context.Background(), bootstrap.RuntimeOptions{DatabasePath: filepath.Join(t.TempDir(), "bindings.db")})
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = r.Close() }()
	service := NewService(context.Background(), r)
	result, err := service.ListIntegrationOfferings()
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Integrations) != 1 || result.Integrations[0].Endpoint != "https://api.anysearch.com/mcp" {
		t.Fatalf("invalid desktop offerings: %+v", result)
	}
}

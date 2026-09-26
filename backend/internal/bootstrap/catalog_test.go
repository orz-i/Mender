package bootstrap

import (
	"context"
	"path/filepath"
	"testing"

	"github.com/orz-i/Mender/backend/internal/catalog/application"
)

func TestCatalogCompositionRegistersAndLoadsProvider(t *testing.T) {
	t.Parallel()

	catalog, err := NewCatalog(context.Background(), CatalogOptions{
		DatabasePath: filepath.Join(t.TempDir(), "mender.db"),
	})
	if err != nil {
		t.Fatalf("NewCatalog() error = %v", err)
	}
	t.Cleanup(func() {
		if err := catalog.Close(); err != nil {
			t.Errorf("Catalog.Close() error = %v", err)
		}
	})

	provider, err := catalog.RegisterProvider.Execute(
		context.Background(),
		application.RegisterProviderRequest{
			Namespace:   "local",
			DisplayName: "Local tools",
		},
	)
	if err != nil {
		t.Fatalf("RegisterProvider.Execute() error = %v", err)
	}

	loaded, err := catalog.GetProvider.Execute(context.Background(), provider.ID())
	if err != nil {
		t.Fatalf("GetProvider.Execute() error = %v", err)
	}
	if loaded.ID() != provider.ID() {
		t.Fatalf("loaded Provider ID = %q, want %q", loaded.ID(), provider.ID())
	}
}

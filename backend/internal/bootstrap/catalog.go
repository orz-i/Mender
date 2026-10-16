package bootstrap

import (
	"context"
	"fmt"
	"time"

	"github.com/orz-i/Mender/backend/internal/catalog/application"
	"github.com/orz-i/Mender/backend/internal/catalog/infrastructure/identity"
	catalogsqlite "github.com/orz-i/Mender/backend/internal/catalog/infrastructure/sqlite"
)

// CatalogOptions configures the Catalog composition root.
type CatalogOptions struct {
	DatabasePath string
}

// Catalog contains the currently composed Catalog use cases.
//
// Tool execution and protocol adapters intentionally do not belong in this Catalog composition root.
type Catalog struct {
	RegisterProvider        application.RegisterProvider
	UpdateProvider          application.UpdateProvider
	GetProvider             application.GetProvider
	ListProviders           application.ListProviders
	FindProviderByNamespace application.FindProviderByNamespace
	RemoveProvider          application.RemoveProvider
	DescribeTool            application.DescribeTool
	BrowseTools             application.BrowseTools
	PageTools               application.PageTools
	SearchTools             application.SearchTools
	ListProviderToolCounts  application.ListProviderToolCounts

	store *catalogsqlite.Store
	ids   *identity.Generator
	clock systemClock
}

// NewCatalog composes Catalog application services with local infrastructure.
func NewCatalog(ctx context.Context, options CatalogOptions) (*Catalog, error) {
	store, err := catalogsqlite.Open(ctx, options.DatabasePath)
	if err != nil {
		return nil, fmt.Errorf("compose catalog: %w", err)
	}

	ids := identity.NewGenerator()
	clock := systemClock{}

	return &Catalog{
		RegisterProvider:        application.NewRegisterProvider(store, ids, clock),
		UpdateProvider:          application.NewUpdateProvider(store, store, clock),
		GetProvider:             application.NewGetProvider(store),
		ListProviders:           application.NewListProviders(store),
		FindProviderByNamespace: application.NewFindProviderByNamespace(store),
		RemoveProvider:          application.NewRemoveProvider(store),
		DescribeTool:            application.NewDescribeTool(store),
		BrowseTools:             application.NewBrowseTools(store),
		PageTools:               application.NewPageTools(store),
		SearchTools:             application.NewSearchTools(store),
		ListProviderToolCounts:  application.NewListProviderToolCounts(store),
		store:                   store,
		ids:                     ids,
		clock:                   clock,
	}, nil
}

// SyncProviderTools creates the synchronization use case for a concrete ToolSource.
//
// ToolSource is supplied by an adapter, so the composition root does not know
// about MCP or any other provider protocol.
func (c *Catalog) SyncProviderTools(source application.ToolSource) application.SyncProviderTools {
	return application.NewSyncProviderTools(c.store, c.store, source, c.ids, c.clock)
}

// Close releases Catalog infrastructure resources.
func (c *Catalog) Close() error {
	return c.store.Close()
}

type systemClock struct{}

func (systemClock) Now() time.Time {
	return time.Now().UTC()
}

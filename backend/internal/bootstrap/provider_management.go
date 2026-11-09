package bootstrap

import (
	"context"

	catalogapplication "github.com/orz-i/Mender/backend/internal/catalog/application"
	catalogdomain "github.com/orz-i/Mender/backend/internal/catalog/domain"
	providerapplication "github.com/orz-i/Mender/backend/internal/providerplugin/application"
	providerretirement "github.com/orz-i/Mender/backend/internal/providerplugin/infrastructure/sqlite"
)

// providerManagement only wires existing owners into the configured Provider
// application. It owns no state transitions or compensation decisions.
func (r *Runtime) providerManagement() providerapplication.Service {
	deps := providerapplication.Dependencies{
		Catalog:         providerCatalogAdapter{catalog: r.Catalog},
		Configurations:  r.providerConfigs,
		Builtins:        r.providerPlugins,
		Resolver:        r.providerResolver,
		Installations:   r.externalResolver,
		Instances:       r.providerInstances,
		Credentials:     r.Credentials.Resolver,
		Activation:      r.providerActivation,
		Profiles:        r.Profiles.List,
		Policy:          r.Policy.Get,
		Retirement:      providerretirement.NewRetirement(r.providerDatabasePath),
		Clock:           r.Catalog.clock,
		IsManagedFacade: isManagedProviderFacade,
	}
	// Agent and Sandbox are directly owned SourceRefs in v3, not Provider
	// children; their retired facade tables are absent from the database.
	return providerapplication.NewService(deps)
}

type providerCatalogAdapter struct{ catalog *Catalog }

func (a providerCatalogAdapter) Register(ctx context.Context, request catalogapplication.RegisterProviderRequest) (catalogdomain.Provider, error) {
	return a.catalog.RegisterProvider.Execute(ctx, request)
}

func (a providerCatalogAdapter) Update(ctx context.Context, request catalogapplication.UpdateProviderRequest) (catalogdomain.Provider, error) {
	return a.catalog.UpdateProvider.Execute(ctx, request)
}

func (a providerCatalogAdapter) Get(ctx context.Context, id catalogdomain.ProviderID) (catalogdomain.Provider, error) {
	return a.catalog.GetProvider.Execute(ctx, id)
}

func (a providerCatalogAdapter) Remove(ctx context.Context, id catalogdomain.ProviderID) error {
	return a.catalog.RemoveProvider.Execute(ctx, id)
}

func (a providerCatalogAdapter) DescribeTool(ctx context.Context, id catalogdomain.ToolID) (catalogdomain.Tool, error) {
	return a.catalog.DescribeTool.Execute(ctx, id)
}

func (a providerCatalogAdapter) Sync(ctx context.Context, id catalogdomain.ProviderID, source catalogapplication.ToolSource) (catalogapplication.SyncProviderToolsResult, error) {
	return a.catalog.SyncProviderTools(source).Execute(ctx, id)
}

func (a providerCatalogAdapter) Snapshot(ctx context.Context, id catalogdomain.ProviderID) ([]catalogdomain.Tool, error) {
	return a.catalog.ProviderToolSnapshots.Snapshot(ctx, id)
}

func (a providerCatalogAdapter) Restore(ctx context.Context, id catalogdomain.ProviderID, tools []catalogdomain.Tool) error {
	return a.catalog.ProviderToolSnapshots.Restore(ctx, id, tools)
}

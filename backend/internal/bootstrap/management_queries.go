package bootstrap

import (
	"context"
	"fmt"

	catalogdomain "github.com/orz-i/Mender/backend/internal/catalog/domain"
	managementquery "github.com/orz-i/Mender/backend/internal/management/application/query"
	"github.com/orz-i/Mender/backend/internal/management/infrastructure/providerprojection"
	"github.com/orz-i/Mender/backend/internal/providerplugin"
)

// ListManagementProviders shares one inert inventory query across transports.
func (r *Runtime) ListManagementProviders(ctx context.Context) ([]managementquery.ProviderView, error) {
	if r == nil {
		return nil, fmt.Errorf("list management providers: runtime is nil")
	}
	return managementquery.NewListProviderViews(managementProviderSource{r}, providerprojection.Provider).Execute(ctx)
}

// ManagementProviderView is also used for existing mutation responses, so
// configuration redaction and plugin-specific projection have a single owner.
func ManagementProviderView(provider catalogdomain.Provider, config *providerplugin.Configuration,
	descriptor *providerplugin.Descriptor, count int) managementquery.ProviderView {
	return providerprojection.Provider(provider, config, descriptor, count)
}

type managementProviderSource struct{ *Runtime }

func (s managementProviderSource) ListProviders(ctx context.Context) ([]catalogdomain.Provider, error) {
	return s.Catalog.ListProviders.Execute(ctx)
}

func (s managementProviderSource) ListProviderToolCounts(ctx context.Context) (map[catalogdomain.ProviderID]int, error) {
	return s.Catalog.ListProviderToolCounts.Execute(ctx)
}

func (s managementProviderSource) ProviderToolSource(ctx context.Context, provider catalogdomain.Provider,
	config *providerplugin.Configuration) (managementquery.ToolSourceView, error) {
	owner := managementquery.SourceOwnerForProvider(provider, config != nil)
	view := managementquery.ToolSourceView{ToolSourceOwner: owner}
	if !owner.Discoverable(false) {
		return view, nil
	}
	view.Selectable = owner.Discoverable(false)
	return view, nil
}

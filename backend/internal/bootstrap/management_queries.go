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
	config *providerplugin.Configuration, owners map[catalogdomain.ProviderID]managementquery.ToolSourceOwner) (managementquery.ToolSourceView, error) {
	owner := ToolSourceForProvider(provider, config, owners)
	view := managementquery.ToolSourceView{ToolSourceOwner: owner}
	if !owner.Discoverable(false) {
		return view, nil
	}
	var unavailable bool
	if owner.Kind == "sandbox" && config != nil {
		// Keep existing host-owned inert capability/lifecycle observations at
		// the composition boundary. This does not probe or activate a backend.
		var err error
		unavailable, err = s.mcpSandboxSourceUnavailable(ctx, owner.ResourceID, config.PluginID)
		if err != nil {
			return managementquery.ToolSourceView{}, fmt.Errorf("sandbox readiness: %w", err)
		}
	}
	view.Selectable = owner.Discoverable(unavailable)
	return view, nil
}

package bootstrap

import (
	"context"
	"fmt"

	catalogdomain "github.com/orz-i/Mender/backend/internal/catalog/domain"
	managementquery "github.com/orz-i/Mender/backend/internal/management/application/query"
	"github.com/orz-i/Mender/backend/internal/management/infrastructure/querysqlite"
	providerplugin "github.com/orz-i/Mender/backend/internal/providerplugin"
)

// ToolSourceOwner is inert, derived display attribution for an exact Provider.
// It is not a second resource identity, a grant, or a health/availability probe.
type ToolSourceOwner = managementquery.ToolSourceOwner

// ListToolSourceOwners joins existing exact resource -> facade relations;
// it never guesses an owner from PluginID, namespace or config arguments.
func (r *Runtime) ListToolSourceOwners(ctx context.Context) (map[catalogdomain.ProviderID]ToolSourceOwner, error) {
	if r == nil {
		return nil, fmt.Errorf("tool source owners: runtime is nil")
	}
	return querysqlite.ListToolSourceOwners(ctx, r.providerDatabasePath)
}

// ToolSourceForProvider gives a standalone Provider its own display identity;
// an unbound managed facade remains explicitly unclaimed, not assigned by
// matching the internal adapter name or inspecting unverified config.
func ToolSourceForProvider(
	provider catalogdomain.Provider,
	config *providerplugin.Configuration,
	owners map[catalogdomain.ProviderID]ToolSourceOwner,
) ToolSourceOwner {
	return managementquery.SourceOwnerForProvider(provider, config != nil,
		config != nil && isManagedProviderFacade(config.PluginID), owners)
}

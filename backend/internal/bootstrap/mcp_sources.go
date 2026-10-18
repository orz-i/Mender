package bootstrap

import (
	"context"
	"fmt"
	"strings"

	catalogdomain "github.com/orz-i/Mender/backend/internal/catalog/domain"
	mcpserver "github.com/orz-i/Mender/backend/internal/mcp/infrastructure/server"
	providerplugin "github.com/orz-i/Mender/backend/internal/providerplugin"
)

// resolveMCPToolSources projects verified, current source attribution only for
// Providers explicitly requested by the Profile-scoped MCP adapter. It shares
// the same host-owned identity rules as the Tools workspace; no source registry,
// dynamic plugin inspection, or execution is involved.
func (r *Runtime) resolveMCPToolSources(
	ctx context.Context,
	providerIDs []catalogdomain.ProviderID,
) (map[catalogdomain.ProviderID]mcpserver.ToolSource, error) {
	sources := make(map[catalogdomain.ProviderID]mcpserver.ToolSource, len(providerIDs))
	if len(providerIDs) == 0 {
		return sources, nil
	}

	owners, err := r.ListToolSourceOwners(ctx)
	if err != nil {
		// Ambiguous managed ownership cannot safely be attributed.
		return nil, fmt.Errorf("resolve MCP tool sources: ownership: %w", err)
	}
	configs, err := r.ListProviderConfigs(ctx)
	if err != nil {
		return nil, fmt.Errorf("resolve MCP tool sources: provider configs: %w", err)
	}
	configByProvider := make(map[catalogdomain.ProviderID]providerplugin.Configuration, len(configs))
	for _, config := range configs {
		configByProvider[config.ProviderID] = config
	}

	for _, id := range providerIDs {
		if _, seen := sources[id]; seen {
			continue
		}
		provider, err := r.Catalog.GetProvider.Execute(ctx, id)
		if err != nil {
			// Fail closed on a stale/deleted Provider rather than assigning its
			// tools to another Provider or inferring a resource owner.
			return nil, fmt.Errorf("resolve MCP tool source %s: %w", id, err)
		}
		var configPtr *providerplugin.Configuration
		if config, found := configByProvider[id]; found {
			value := config
			configPtr = &value
		}
		owner := ToolSourceForProvider(provider, configPtr, owners)
		source := mcpserver.ToolSource{Kind: owner.Kind}
		if owner.Kind != "unclaimed" && strings.TrimSpace(owner.Name) != "" {
			source.Name = owner.Name
			source.ResourceID = owner.ResourceID
			if owner.Kind != "provider" && provider.DisplayName() != owner.Name {
				source.ProviderName = provider.DisplayName()
			}
		} else {
			source.Kind = "unclaimed"
			source.Name = "Unclaimed source"
		}
		sources[id] = source
	}
	return sources, nil
}

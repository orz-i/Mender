package bootstrap

import (
	"context"
	"errors"
	"fmt"
	"strings"

	catalogdomain "github.com/orz-i/Mender/backend/internal/catalog/domain"
	mcpserver "github.com/orz-i/Mender/backend/internal/mcp/infrastructure/server"
	providerplugin "github.com/orz-i/Mender/backend/internal/providerplugin"
	sandboxdomain "github.com/orz-i/Mender/backend/internal/sandbox/domain"
	sshjobsplugin "github.com/orz-i/Mender/backend/internal/sshjobs/infrastructure/plugin"
	workspaceplugin "github.com/orz-i/Mender/backend/internal/workspace/infrastructure/plugin"
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
			if owner.Kind == "sandbox" && configPtr != nil {
				source.Unavailable, err = r.mcpSandboxSourceUnavailable(ctx, owner.ResourceID, configPtr.PluginID)
				if err != nil {
					return nil, fmt.Errorf("resolve MCP tool source %s: sandbox readiness: %w", id, err)
				}
			}
		} else {
			source.Kind = "unclaimed"
			source.Name = "Unclaimed source"
		}
		sources[id] = source
	}
	return sources, nil
}

// mcpSandboxSourceUnavailable reuses inert, Host-reviewed capability and
// lifecycle observations. Workspace operations require a Ready Sandbox;
// Durable Jobs retain read-only recovery while Stopped. No health probes,
// remote login, resource activation, or Profile mutations occur here.
func (r *Runtime) mcpSandboxSourceUnavailable(
	ctx context.Context, resourceID string, pluginID providerplugin.ID,
) (bool, error) {
	id, err := sandboxdomain.ParseID(resourceID)
	if err != nil {
		return true, nil
	}
	switch pluginID {
	case workspaceplugin.ID:
		available, _, err := r.sandboxWorkspaceCapabilitySnapshot(ctx, id)
		if errors.Is(err, sandboxdomain.ErrNotFound) {
			return true, nil
		}
		return !available, err
	case sshjobsplugin.ID:
		run, err := r.Sandboxes.GetRun(ctx, id)
		if errors.Is(err, sandboxdomain.ErrNotFound) {
			return true, nil
		}
		if err != nil {
			return false, err
		}
		if run.State != sandboxdomain.StateReady && run.State != sandboxdomain.StateStopped {
			return true, nil
		}
		if _, err := r.Sandboxes.VerifiedSSHJobContracts(ctx, id); err != nil {
			if errors.Is(err, sandboxdomain.ErrNotFound) ||
				errors.Is(err, sandboxdomain.ErrCapabilityUnavailable) ||
				errors.Is(err, sandboxdomain.ErrBackendUnavailable) {
				return true, nil
			}
			return false, err
		}
		return false, nil
	default:
		// A future Sandbox facade needs an explicit verified contract rather
		// than silently borrowing Workspace or Jobs readiness semantics.
		return true, nil
	}
}

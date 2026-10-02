package bootstrap

import (
	"context"
	"errors"
	"fmt"

	catalogapplication "github.com/orz-i/Mender/backend/internal/catalog/application"
	catalogdomain "github.com/orz-i/Mender/backend/internal/catalog/domain"
	mcpconfigstore "github.com/orz-i/Mender/backend/internal/mcp/infrastructure/configstore"
	mcpprovider "github.com/orz-i/Mender/backend/internal/mcp/infrastructure/provider"
	mcpserver "github.com/orz-i/Mender/backend/internal/mcp/infrastructure/server"
	profiledomain "github.com/orz-i/Mender/backend/internal/profile/domain"
)

// RegisterMCPProvider persists canonical Provider metadata and adapter-owned
// connection configuration. Config failure is compensated by removing the newly
// created Provider.
func (r *Runtime) RegisterMCPProvider(
	ctx context.Context,
	request catalogapplication.RegisterProviderRequest,
	config mcpconfigstore.Config,
) (catalogdomain.Provider, error) {
	provider, err := r.Catalog.RegisterProvider.Execute(ctx, request)
	if err != nil {
		return catalogdomain.Provider{}, err
	}

	config.ProviderID = provider.ID()
	genericConfig, err := legacyMCPProviderConfiguration(config)
	if err != nil {
		removeErr := r.Catalog.RemoveProvider.Execute(ctx, provider.ID())
		if removeErr != nil {
			return catalogdomain.Provider{}, errors.Join(
				fmt.Errorf("register MCP provider: map config: %w", err),
				fmt.Errorf("register MCP provider: compensate provider removal: %w", removeErr),
			)
		}
		return catalogdomain.Provider{}, fmt.Errorf("register MCP provider: map config: %w", err)
	}
	if err := r.providerConfigs.Save(ctx, genericConfig); err != nil {
		removeErr := r.Catalog.RemoveProvider.Execute(ctx, provider.ID())
		if removeErr != nil {
			return catalogdomain.Provider{}, errors.Join(
				fmt.Errorf("register MCP provider: save provider config: %w", err),
				fmt.Errorf("register MCP provider: compensate provider removal: %w", removeErr),
			)
		}
		return catalogdomain.Provider{}, fmt.Errorf(
			"register MCP provider: save provider config: %w",
			err,
		)
	}
	if err := r.mcpConfigs.Save(ctx, config); err != nil {
		deleteErr := r.providerConfigs.Delete(ctx, provider.ID())
		removeErr := r.Catalog.RemoveProvider.Execute(ctx, provider.ID())
		return catalogdomain.Provider{}, errors.Join(
			fmt.Errorf("register MCP provider: save compatibility config: %w", err),
			wrapOptionalError(
				"register MCP provider: compensate provider config removal",
				deleteErr,
			),
			wrapOptionalError(
				"register MCP provider: compensate provider removal",
				removeErr,
			),
		)
	}

	return provider, nil
}

// MCPProviderConfig returns persisted adapter configuration for one Provider.
func (r *Runtime) MCPProviderConfig(
	ctx context.Context,
	providerID catalogdomain.ProviderID,
) (mcpconfigstore.Config, error) {
	return r.mcpConfigs.Get(ctx, providerID)
}

// ListMCPProviderConfigs returns all persisted MCP Provider configurations.
func (r *Runtime) ListMCPProviderConfigs(
	ctx context.Context,
) ([]mcpconfigstore.Config, error) {
	return r.mcpConfigs.List(ctx)
}

// attachMCPProviderMechanism is an ungated low-level test/embedding mechanism.
// Production activation paths must use Provider activation admission.
func (r *Runtime) attachMCPProviderMechanism(
	ctx context.Context,
	providerID catalogdomain.ProviderID,
	connection *mcpprovider.Connection,
) (catalogapplication.SyncProviderToolsResult, error) {
	if connection == nil {
		return catalogapplication.SyncProviderToolsResult{}, fmt.Errorf("attach MCP provider: connection must not be nil")
	}
	return r.activateMCPProvider(ctx, providerID, connection, true)
}

func (r *Runtime) activateMCPProvider(
	ctx context.Context,
	providerID catalogdomain.ProviderID,
	connection *mcpprovider.Connection,
	synchronize bool,
) (catalogapplication.SyncProviderToolsResult, error) {
	if _, err := r.Catalog.GetProvider.Execute(ctx, providerID); err != nil {
		return catalogapplication.SyncProviderToolsResult{}, fmt.Errorf("activate MCP provider: provider: %w", err)
	}

	var result catalogapplication.SyncProviderToolsResult
	if synchronize {
		syncTools := r.Catalog.SyncProviderTools(connection)
		var err error
		result, err = syncTools.Execute(ctx, providerID)
		if err != nil {
			return catalogapplication.SyncProviderToolsResult{}, fmt.Errorf("activate MCP provider: synchronize: %w", err)
		}
	}

	if err := r.RegisterExecutor(providerID, connection); err != nil {
		return catalogapplication.SyncProviderToolsResult{}, fmt.Errorf("activate MCP provider: register executor: %w", err)
	}

	r.addCloser(connection)
	return result, nil
}

// NewMCPServer builds a Profile-scoped downstream MCP server backed by the
// existing Catalog queries and shared Invocation pipeline.
func (r *Runtime) NewMCPServer(
	ctx context.Context,
	profile profiledomain.Profile,
) (*mcpserver.Adapter, error) {
	return mcpserver.New(ctx, profile, mcpserver.Dependencies{
		DescribeTool: r.Catalog.DescribeTool.Execute,
		GetProvider:  r.Catalog.GetProvider.Execute,
		SearchTools:  r.Catalog.SearchTools.Execute,
		InvokeTool:   r.InvokeTool.Execute,
	})
}

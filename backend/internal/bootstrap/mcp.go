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
	if err := r.mcpConfigs.Save(ctx, config); err != nil {
		removeErr := r.Catalog.RemoveProvider.Execute(ctx, provider.ID())
		if removeErr != nil {
			return catalogdomain.Provider{}, errors.Join(
				fmt.Errorf("register MCP provider: save config: %w", err),
				fmt.Errorf("register MCP provider: compensate provider removal: %w", removeErr),
			)
		}
		return catalogdomain.Provider{}, fmt.Errorf("register MCP provider: save config: %w", err)
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

// ConnectConfiguredMCPProvider connects a persisted Provider configuration.
//
// synchronize=false registers an executor without refreshing Catalog metadata.
// This is used by tool call and serve startup paths.
func (r *Runtime) ConnectConfiguredMCPProvider(
	ctx context.Context,
	providerID catalogdomain.ProviderID,
	synchronize bool,
) (catalogapplication.SyncProviderToolsResult, error) {
	if _, err := r.Catalog.GetProvider.Execute(ctx, providerID); err != nil {
		return catalogapplication.SyncProviderToolsResult{}, fmt.Errorf(
			"connect configured MCP provider: provider: %w",
			err,
		)
	}

	if synchronize {
		instance, err := r.providerInstances.Reload(ctx, providerID)
		if err != nil {
			return catalogapplication.SyncProviderToolsResult{}, fmt.Errorf(
				"connect configured MCP provider: %w",
				err,
			)
		}
		syncTools := r.Catalog.SyncProviderTools(instance)
		result, err := syncTools.Execute(ctx, providerID)
		if err != nil {
			return catalogapplication.SyncProviderToolsResult{}, fmt.Errorf(
				"connect configured MCP provider: synchronize: %w",
				err,
			)
		}
		return result, nil
	}

	if _, err := r.providerInstances.Ensure(ctx, providerID); err != nil {
		return catalogapplication.SyncProviderToolsResult{}, fmt.Errorf(
			"connect configured MCP provider: %w",
			err,
		)
	}
	return catalogapplication.SyncProviderToolsResult{}, nil
}

// SyncConfiguredMCPProvider connects and refreshes one Provider's Catalog snapshot.
func (r *Runtime) SyncConfiguredMCPProvider(
	ctx context.Context,
	providerID catalogdomain.ProviderID,
) (catalogapplication.SyncProviderToolsResult, error) {
	return r.ConnectConfiguredMCPProvider(ctx, providerID, true)
}

// ActivateConfiguredMCPProvider connects one Provider for execution without resynchronizing.
func (r *Runtime) ActivateConfiguredMCPProvider(
	ctx context.Context,
	providerID catalogdomain.ProviderID,
) error {
	_, err := r.ConnectConfiguredMCPProvider(ctx, providerID, false)
	return err
}

// ConnectMCPHTTP connects, synchronizes, and registers a remote MCP Provider.
// On success the Runtime owns the connection and closes it with Runtime.Close.
func (r *Runtime) ConnectMCPHTTP(
	ctx context.Context,
	providerID catalogdomain.ProviderID,
	config mcpprovider.HTTPConfig,
) (catalogapplication.SyncProviderToolsResult, error) {
	connection, err := mcpprovider.ConnectHTTP(ctx, config)
	if err != nil {
		return catalogapplication.SyncProviderToolsResult{}, err
	}

	result, err := r.activateMCPProvider(ctx, providerID, connection, true)
	if err != nil {
		_ = connection.Close()
		return catalogapplication.SyncProviderToolsResult{}, err
	}
	return result, nil
}

// ConnectMCPCommand starts, synchronizes, and registers a local stdio MCP Provider.
// On success the Runtime owns the process connection and closes it with Runtime.Close.
func (r *Runtime) ConnectMCPCommand(
	ctx context.Context,
	providerID catalogdomain.ProviderID,
	config mcpprovider.CommandConfig,
) (catalogapplication.SyncProviderToolsResult, error) {
	connection, err := mcpprovider.ConnectCommand(ctx, config)
	if err != nil {
		return catalogapplication.SyncProviderToolsResult{}, err
	}

	result, err := r.activateMCPProvider(ctx, providerID, connection, true)
	if err != nil {
		_ = connection.Close()
		return catalogapplication.SyncProviderToolsResult{}, err
	}
	return result, nil
}

// AttachMCPProvider synchronizes and registers an already-connected MCP Provider.
//
// This is primarily useful for embedded/in-memory transports. Ownership transfers
// to Runtime only after successful synchronization and executor registration.
func (r *Runtime) AttachMCPProvider(
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

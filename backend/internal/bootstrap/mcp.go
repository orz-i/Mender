package bootstrap

import (
	"context"
	"fmt"

	catalogapplication "github.com/orz-i/Mender/backend/internal/catalog/application"
	catalogdomain "github.com/orz-i/Mender/backend/internal/catalog/domain"
	mcpprovider "github.com/orz-i/Mender/backend/internal/mcp/infrastructure/provider"
	mcpserver "github.com/orz-i/Mender/backend/internal/mcp/infrastructure/server"
	profiledomain "github.com/orz-i/Mender/backend/internal/profile/domain"
)

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

	result, err := r.attachMCPProvider(ctx, providerID, connection)
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

	result, err := r.attachMCPProvider(ctx, providerID, connection)
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
	return r.attachMCPProvider(ctx, providerID, connection)
}

func (r *Runtime) attachMCPProvider(
	ctx context.Context,
	providerID catalogdomain.ProviderID,
	connection *mcpprovider.Connection,
) (catalogapplication.SyncProviderToolsResult, error) {
	if _, err := r.Catalog.GetProvider.Execute(ctx, providerID); err != nil {
		return catalogapplication.SyncProviderToolsResult{}, fmt.Errorf("attach MCP provider: provider: %w", err)
	}

	syncTools := r.Catalog.SyncProviderTools(connection)
	result, err := syncTools.Execute(ctx, providerID)
	if err != nil {
		return catalogapplication.SyncProviderToolsResult{}, fmt.Errorf("attach MCP provider: synchronize: %w", err)
	}
	if err := r.RegisterExecutor(providerID, connection); err != nil {
		return catalogapplication.SyncProviderToolsResult{}, fmt.Errorf("attach MCP provider: register executor: %w", err)
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

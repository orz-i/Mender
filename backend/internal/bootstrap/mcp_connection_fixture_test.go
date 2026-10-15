package bootstrap

import (
	"context"
	"fmt"

	catalogapplication "github.com/orz-i/Mender/backend/internal/catalog/application"
	catalogdomain "github.com/orz-i/Mender/backend/internal/catalog/domain"
	mcpprovider "github.com/orz-i/Mender/backend/internal/mcp/infrastructure/provider"
)

// Test-only direct MCP connection fixture, not a Runtime production path.
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

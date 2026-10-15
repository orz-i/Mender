package bootstrap

import (
	"context"

	mcpserver "github.com/orz-i/Mender/backend/internal/mcp/infrastructure/server"
	profiledomain "github.com/orz-i/Mender/backend/internal/profile/domain"
)

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
		InvokeTool:   r.invokeToolWithSSHTimeout,
	})
}

package bootstrap

import (
	"context"
	"fmt"
	"sort"

	catalogdomain "github.com/orz-i/Mender/backend/internal/catalog/domain"
	governancedomain "github.com/orz-i/Mender/backend/internal/governance/domain"
	mcpmanaged "github.com/orz-i/Mender/backend/internal/mcp/infrastructure/managed"
	profiledomain "github.com/orz-i/Mender/backend/internal/profile/domain"
	tunneldomain "github.com/orz-i/Mender/backend/internal/tunnel/domain"
)

var (
	ErrInvalidMCPServerAddress = mcpmanaged.ErrInvalidMCPServerAddress
	ErrMCPServerNotFound       = mcpmanaged.ErrMCPServerNotFound
)

type MCPServerInstance = mcpmanaged.Instance

func (r *Runtime) ListMCPServers() []MCPServerInstance { return r.managedMCP.List() }
func (r *Runtime) StartMCPHTTPServer(ctx context.Context, slug profiledomain.Slug, address string, control governancedomain.Context) (MCPServerInstance, error) {
	return r.managedMCP.Start(ctx, slug, address, control)
}
func (r *Runtime) StopMCPServer(ctx context.Context, id string) error {
	return r.managedMCP.Stop(ctx, id)
}
func (r *Runtime) RestartMCPServer(ctx context.Context, id string, control governancedomain.Context) (MCPServerInstance, error) {
	return r.managedMCP.Restart(ctx, id, control)
}
func (r *Runtime) isTunnelActive(id tunneldomain.TunnelID) bool {
	return r.managedMCP != nil && r.managedMCP.IsTunnelActive(id)
}
func (r *Runtime) closeManagedMCPServers() error {
	if r.managedMCP == nil {
		return nil
	}
	return r.managedMCP.Close()
}
func (r *Runtime) ActivateProfileProviders(
	ctx context.Context,
	profile profiledomain.Profile,
	governanceContext governancedomain.Context,
) error {
	if err := governanceContext.Validate(); err != nil {
		return fmt.Errorf("activate profile providers: governance context: %w", err)
	}
	if governanceContext.Scope().Kind() != governancedomain.ScopeProfile ||
		governanceContext.Scope().Reference() != profile.ID().String() {
		return fmt.Errorf("activate profile providers: governance scope must match Profile %s", profile.ID())
	}
	providerSet := make(map[catalogdomain.ProviderID]struct{})
	for _, toolID := range profile.ToolIDs() {
		tool, err := r.Catalog.DescribeTool.Execute(ctx, toolID)
		if err != nil {
			return fmt.Errorf(
				"activate profile provider: describe %s: %w",
				toolID,
				err,
			)
		}
		providerSet[tool.ProviderID()] = struct{}{}
	}

	providerIDs := make([]catalogdomain.ProviderID, 0, len(providerSet))
	for providerID := range providerSet {
		providerIDs = append(providerIDs, providerID)
	}
	sort.Slice(providerIDs, func(i, j int) bool {
		return providerIDs[i].String() < providerIDs[j].String()
	})

	for _, providerID := range providerIDs {
		if err := r.activateConfiguredProvider(ctx, providerID, governanceContext); err != nil {
			return fmt.Errorf(
				"activate profile provider %s: %w",
				providerID,
				err,
			)
		}
	}
	return nil
}

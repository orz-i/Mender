package bootstrap

import (
	"context"
	"fmt"

	mcpdelivery "github.com/orz-i/Mender/backend/internal/mcp/application/delivery"
	mcpdomain "github.com/orz-i/Mender/backend/internal/mcp/domain"
	mcpsqlite "github.com/orz-i/Mender/backend/internal/mcp/infrastructure/sqlite"
	profileapplication "github.com/orz-i/Mender/backend/internal/profile/application"
	profiledomain "github.com/orz-i/Mender/backend/internal/profile/domain"
	tunnelapplication "github.com/orz-i/Mender/backend/internal/tunnel/application"
)

type MCPDelivery struct {
	Get         mcpdelivery.Get
	GetRevision mcpdelivery.GetRevision
	Configure   mcpdelivery.Configure
	store       *mcpsqlite.Store
}

// newTypedMCPDelivery uses the SAME existing MCP Delivery application use
// cases; the v3 schema must be verified before opening the Store. This does
// not start a listener or activate a Provider/Tunnel.
func newTypedMCPDelivery(ctx context.Context, path string, profiles profileapplication.GetProfileByID, tunnels tunnelapplication.GetTunnel) (*MCPDelivery, error) {
	store, err := mcpsqlite.OpenTyped(ctx, path)
	if err != nil {
		return nil, err
	}
	return composeMCPDelivery(store, profiles, tunnels), nil
}

func composeMCPDelivery(store *mcpsqlite.Store, profiles profileapplication.GetProfileByID, tunnels tunnelapplication.GetTunnel) *MCPDelivery {
	return &MCPDelivery{Get: mcpdelivery.NewGet(store), GetRevision: mcpdelivery.NewGetRevision(store), Configure: mcpdelivery.NewConfigure(store, profiles, tunnels), store: store}
}

func (d *MCPDelivery) Close() error {
	if d == nil || d.store == nil {
		return nil
	}
	return d.store.Close()
}

// ProfileMCPDelivery combines two read snapshots, without putting transport
// configuration back into Profile or making a running resource authoritative.
type ProfileMCPDelivery struct {
	Profile profiledomain.Profile
	Config  mcpdomain.Config
}

func (r *Runtime) GetProfileMCPDelivery(ctx context.Context, slug profiledomain.Slug) (ProfileMCPDelivery, error) {
	profile, err := r.Profiles.Get.Execute(ctx, slug)
	if err != nil {
		return ProfileMCPDelivery{}, err
	}
	config, err := r.MCPDelivery.Get.Execute(ctx, profile.ID())
	if err != nil {
		return ProfileMCPDelivery{}, err
	}
	profileRevision, err := r.Profiles.GetRevision.Execute(ctx, profile.ID())
	if err != nil {
		return ProfileMCPDelivery{}, err
	}
	deliveryRevision, err := r.MCPDelivery.GetRevision.Execute(ctx, profile.ID())
	if err != nil {
		return ProfileMCPDelivery{}, err
	}
	if profileRevision != profile.Revision() {
		return ProfileMCPDelivery{}, mcpdelivery.ErrProfileRevisionConflict
	}
	if deliveryRevision != config.Revision() {
		return ProfileMCPDelivery{}, mcpdelivery.ErrRevisionConflict
	}
	return ProfileMCPDelivery{Profile: profile, Config: config}, nil
}

func (r *Runtime) ConfigureProfileMCPDelivery(ctx context.Context, slug profiledomain.Slug, request mcpdelivery.ConfigureRequest) (ProfileMCPDelivery, error) {
	profile, err := r.Profiles.Get.Execute(ctx, slug)
	if err != nil {
		return ProfileMCPDelivery{}, err
	}
	if profile.Revision() != request.ExpectedProfileRevision {
		return ProfileMCPDelivery{}, mcpdelivery.ErrProfileRevisionConflict
	}
	// The route's selected Profile owns the identity. No request may substitute
	// another resource, and configuration does not start a listener or Tunnel.
	request.ProfileID = profile.ID()
	config, err := r.MCPDelivery.Configure.Execute(ctx, request)
	if err != nil {
		return ProfileMCPDelivery{}, fmt.Errorf("configure Profile MCP delivery: %w", err)
	}
	return ProfileMCPDelivery{Profile: profile, Config: config}, nil
}

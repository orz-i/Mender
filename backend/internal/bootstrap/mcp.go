package bootstrap

import (
	"context"
	"errors"
	"net/http"

	governancedomain "github.com/orz-i/Mender/backend/internal/governance/domain"
	mcpdomain "github.com/orz-i/Mender/backend/internal/mcp/domain"
	mcpmanaged "github.com/orz-i/Mender/backend/internal/mcp/infrastructure/managed"
	mcpserver "github.com/orz-i/Mender/backend/internal/mcp/infrastructure/server"
	profiledomain "github.com/orz-i/Mender/backend/internal/profile/domain"
)

// NewMCPServer builds a Profile-scoped downstream MCP server backed by the
// existing Catalog queries and shared Invocation pipeline.
func (r *Runtime) NewMCPServer(
	ctx context.Context,
	profile profiledomain.Profile,
) (*mcpserver.Adapter, error) {
	snapshot, err := r.managedMCP.LoadSnapshot(ctx, profile.ID())
	if err != nil {
		return nil, err
	}
	if snapshot.Profile.Revision() != profile.Revision() || snapshot.Profile.Slug() != profile.Slug() {
		return nil, errors.New("build MCP server: Profile changed while loading delivery configuration")
	}
	adapter, err := r.newMCPProjection(ctx, profile, snapshot.Config)
	if err != nil {
		return nil, err
	}
	if err := r.managedMCP.ConfirmSnapshot(ctx, snapshot); err != nil {
		return nil, err
	}
	return adapter, nil
}

func (r *Runtime) newMCPProjection(ctx context.Context, profile profiledomain.Profile, _ mcpdomain.Config) (*mcpserver.Adapter, error) {
	// Both direct CLI MCP and managed HTTP MCP use the identical typed,
	// Profile-restricted source projection and single governed Invocation.
	// There is no ProviderID fallback or second MCP execution engine.
	return mcpserver.NewTyped(ctx, profile.ID().String(), mcpserver.TypedDependencies{
		Profiles: r.typedSupply, DescribeTool: r.Catalog.DescribeTool.Execute,
		InvokeTool: r.invokeToolWithSSHTimeout,
	})
}

func (r *Runtime) configureManagedMCP(
	getConfig func(context.Context, profiledomain.ProfileID) (mcpdomain.Config, error),
	getRevision func(context.Context, profiledomain.ProfileID) (uint64, error),
) error {
	if getConfig == nil || getRevision == nil {
		return errors.New("configure managed MCP: delivery reads unavailable")
	}
	load := func(ctx context.Context, profile profiledomain.Profile) (mcpmanaged.Snapshot, error) {
		config, err := getConfig(ctx, profile.ID())
		return mcpmanaged.Snapshot{Profile: profile, Config: config}, err
	}
	manager, err := mcpmanaged.New(mcpmanaged.Dependencies{
		LoadBySlug: func(ctx context.Context, slug profiledomain.Slug) (mcpmanaged.Snapshot, error) {
			profile, err := r.Profiles.Get.Execute(ctx, slug)
			if err != nil {
				return mcpmanaged.Snapshot{}, err
			}
			return load(ctx, profile)
		},
		LoadByID: func(ctx context.Context, id profiledomain.ProfileID) (mcpmanaged.Snapshot, error) {
			profile, err := r.Profiles.GetByID.Execute(ctx, id)
			if err != nil {
				return mcpmanaged.Snapshot{}, err
			}
			return load(ctx, profile)
		},
		Revisions: func(ctx context.Context, id profiledomain.ProfileID) (mcpmanaged.Versions, error) {
			profileRevision, err := r.Profiles.GetRevision.Execute(ctx, id)
			if err != nil {
				return mcpmanaged.Versions{}, err
			}
			deliveryRevision, err := getRevision(ctx, id)
			return mcpmanaged.Versions{Profile: profileRevision, Delivery: deliveryRevision}, err
		},
		BuildHTTP: func(ctx context.Context, snapshot mcpmanaged.Snapshot, control governancedomain.Context) (http.Handler, error) {
			activation, err := profileActivationContext(control, snapshot.Profile)
			if err != nil {
				return nil, err
			}
			if err := r.ActivateProfileProviders(ctx, snapshot.Profile, activation); err != nil {
				return nil, err
			}
			adapter, err := r.newMCPProjection(ctx, snapshot.Profile, snapshot.Config)
			if err != nil {
				return nil, err
			}
			return adapter.StreamableHTTPHandler(), nil
		},
		DescribeTool:    r.Catalog.DescribeTool.Execute,
		CatalogRevision: r.Catalog.store.CatalogRevision,
		StartTunnel:     r.Tunnels.Start.Execute,
	})
	if err != nil {
		return err
	}
	r.managedMCP = manager
	return nil
}

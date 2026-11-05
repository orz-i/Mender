package delivery

import (
	"context"
	"errors"
	"fmt"

	catalog "github.com/orz-i/Mender/backend/internal/catalog/domain"
	mcp "github.com/orz-i/Mender/backend/internal/mcp/domain"
	profile "github.com/orz-i/Mender/backend/internal/profile/domain"
	tunnel "github.com/orz-i/Mender/backend/internal/tunnel/domain"
)

var (
	ErrNotFound                = errors.New("MCP delivery: not configured")
	ErrRevisionConflict        = errors.New("MCP delivery: stale configuration revision")
	ErrProfileRevisionConflict = errors.New("MCP delivery: stale Profile selection revision")
	ErrToolOutsideProfile      = errors.New("MCP delivery: direct Tool is outside Profile")
	ErrInvalidConfiguration    = errors.New("MCP delivery: invalid configuration")
)

type Repository interface {
	GetConfig(context.Context, profile.ProfileID) (mcp.Config, error)
	GetConfigRevision(context.Context, profile.ProfileID) (uint64, error)
	PutConfig(context.Context, mcp.Config, uint64, *uint64) (mcp.Config, error)
}

type ProfileLookup interface {
	Execute(context.Context, profile.ProfileID) (profile.Profile, error)
}
type TunnelLookup interface {
	Execute(context.Context, tunnel.TunnelID) (tunnel.Tunnel, error)
}

type Get struct{ repository Repository }

func NewGet(repository Repository) Get { return Get{repository: repository} }
func (uc Get) Execute(ctx context.Context, id profile.ProfileID) (mcp.Config, error) {
	return uc.repository.GetConfig(ctx, id)
}

type GetRevision struct{ repository Repository }

func NewGetRevision(repository Repository) GetRevision { return GetRevision{repository: repository} }
func (uc GetRevision) Execute(ctx context.Context, id profile.ProfileID) (uint64, error) {
	return uc.repository.GetConfigRevision(ctx, id)
}

type ConfigureRequest struct {
	ProfileID               profile.ProfileID
	Mode                    mcp.Mode
	TunnelID                tunnel.TunnelID
	DirectToolIDs           []catalog.ToolID
	ExpectedProfileRevision uint64
	// Nil means create only, including when the first persisted revision is 0.
	ExpectedRevision *uint64
}

type Configure struct {
	repository Repository
	profiles   ProfileLookup
	tunnels    TunnelLookup
}

func NewConfigure(repository Repository, profiles ProfileLookup, tunnels TunnelLookup) Configure {
	return Configure{repository: repository, profiles: profiles, tunnels: tunnels}
}
func (uc Configure) Execute(ctx context.Context, request ConfigureRequest) (mcp.Config, error) {
	config, err := mcp.NewConfig(request.ProfileID, request.Mode, request.TunnelID, request.DirectToolIDs)
	if err != nil {
		return mcp.Config{}, fmt.Errorf("%w: %w", ErrInvalidConfiguration, err)
	}
	selected, err := uc.profiles.Execute(ctx, request.ProfileID)
	if err != nil {
		return mcp.Config{}, fmt.Errorf("configure MCP delivery: Profile: %w", err)
	}
	if selected.Revision() != request.ExpectedProfileRevision {
		return mcp.Config{}, ErrProfileRevisionConflict
	}
	for _, id := range request.DirectToolIDs {
		if !selected.Contains(id) {
			return mcp.Config{}, fmt.Errorf("%w: %s", ErrToolOutsideProfile, id)
		}
	}
	if request.TunnelID != "" {
		if uc.tunnels == nil {
			return mcp.Config{}, fmt.Errorf("configure MCP delivery: Tunnel lookup unavailable")
		}
		if _, err := uc.tunnels.Execute(ctx, request.TunnelID); err != nil {
			return mcp.Config{}, fmt.Errorf("configure MCP delivery: Tunnel: %w", err)
		}
	}
	// Both revision fences and the subset are rechecked by the committing
	// repository, after the SQLite writer gate is acquired.
	return uc.repository.PutConfig(ctx, config, request.ExpectedProfileRevision, request.ExpectedRevision)
}

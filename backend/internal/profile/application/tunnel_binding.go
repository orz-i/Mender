package application

import (
	"context"
	"fmt"

	profiledomain "github.com/orz-i/Mender/backend/internal/profile/domain"
	tunneldomain "github.com/orz-i/Mender/backend/internal/tunnel/domain"
)

type SetTunnelRequest struct {
	ProfileSlug profiledomain.Slug
	TunnelID    tunneldomain.TunnelID
}

type SetTunnel struct {
	repository Repository
	tunnels    TunnelLookup
}

func NewSetTunnel(repository Repository, tunnels TunnelLookup) SetTunnel {
	return SetTunnel{repository: repository, tunnels: tunnels}
}

func (uc SetTunnel) Execute(
	ctx context.Context,
	request SetTunnelRequest,
) (profiledomain.Profile, error) {
	profile, err := uc.repository.GetProfileBySlug(ctx, request.ProfileSlug)
	if err != nil {
		return profiledomain.Profile{}, fmt.Errorf("set profile tunnel: %w", err)
	}
	if request.TunnelID == "" {
		updated, err := profile.ClearTunnel()
		if err != nil {
			return profiledomain.Profile{}, fmt.Errorf("clear profile tunnel: %w", err)
		}
		if err := uc.repository.SaveProfile(ctx, updated); err != nil {
			return profiledomain.Profile{}, fmt.Errorf("clear profile tunnel: save: %w", err)
		}
		return updated.WithRevision(updated.Revision() + 1), nil
	}
	if uc.tunnels == nil {
		return profiledomain.Profile{}, fmt.Errorf("set profile tunnel: tunnel lookup is unavailable")
	}
	if _, err := uc.tunnels.GetTunnel(ctx, request.TunnelID); err != nil {
		return profiledomain.Profile{}, fmt.Errorf(
			"set profile tunnel: tunnel %s: %w",
			request.TunnelID,
			err,
		)
	}
	updated, err := profile.SetTunnel(request.TunnelID)
	if err != nil {
		return profiledomain.Profile{}, fmt.Errorf("set profile tunnel: %w", err)
	}
	if err := uc.repository.SaveProfile(ctx, updated); err != nil {
		return profiledomain.Profile{}, fmt.Errorf("set profile tunnel: save: %w", err)
	}
	return updated.WithRevision(updated.Revision() + 1), nil
}

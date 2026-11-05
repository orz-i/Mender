package binding

import (
	"errors"

	"github.com/orz-i/Mender/backend/internal/bootstrap"
	catalog "github.com/orz-i/Mender/backend/internal/catalog/domain"
	mcpdelivery "github.com/orz-i/Mender/backend/internal/mcp/application/delivery"
	mcp "github.com/orz-i/Mender/backend/internal/mcp/domain"
	profile "github.com/orz-i/Mender/backend/internal/profile/domain"
	tunnel "github.com/orz-i/Mender/backend/internal/tunnel/domain"
)

func (s *Service) GetProfileMCPDelivery(slug string) (*ProfileMCPDelivery, error) {
	selected, err := profile.NewSlug(slug)
	if err != nil {
		return nil, err
	}
	snapshot, err := s.runtime.GetProfileMCPDelivery(s.ctx, selected)
	if errors.Is(err, mcpdelivery.ErrNotFound) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	result := profileMCPDeliveryProjection(snapshot)
	return &result, nil
}

// ConfigureProfileMCPDelivery changes desired configuration only. The slug is
// resolved by Runtime; callers cannot supply another Profile's identity.
func (s *Service) ConfigureProfileMCPDelivery(slug string, input ProfileMCPDeliveryInput) (ProfileMCPDelivery, error) {
	selected, err := profile.NewSlug(slug)
	if err != nil {
		return ProfileMCPDelivery{}, err
	}
	var tunnelID tunnel.TunnelID
	if input.TunnelID != "" {
		tunnelID, err = tunnel.ParseTunnelID(input.TunnelID)
		if err != nil {
			return ProfileMCPDelivery{}, err
		}
	}
	direct := make([]catalog.ToolID, 0, len(input.DirectToolIDs))
	for _, raw := range input.DirectToolIDs {
		id, err := catalog.ParseToolID(raw)
		if err != nil {
			return ProfileMCPDelivery{}, err
		}
		direct = append(direct, id)
	}
	snapshot, err := s.runtime.ConfigureProfileMCPDelivery(s.ctx, selected, mcpdelivery.ConfigureRequest{
		Mode: mcp.Mode(input.Mode), TunnelID: tunnelID, DirectToolIDs: direct,
		ExpectedProfileRevision: input.ExpectedProfileRevision, ExpectedRevision: input.ExpectedRevision,
	})
	if err != nil {
		return ProfileMCPDelivery{}, err
	}
	return profileMCPDeliveryProjection(snapshot), nil
}

func profileMCPDeliveryProjection(snapshot bootstrap.ProfileMCPDelivery) ProfileMCPDelivery {
	result := ProfileMCPDelivery{ProfileID: snapshot.Profile.ID().String(), ProfileRevision: snapshot.Profile.Revision(),
		Revision: snapshot.Config.Revision(), Mode: string(snapshot.Config.Mode()),
		DirectToolIDs: make([]string, 0, len(snapshot.Config.DirectToolIDs()))}
	if id, ok := snapshot.Config.TunnelID(); ok {
		result.TunnelID = id.String()
	}
	for _, id := range snapshot.Config.DirectToolIDs() {
		result.DirectToolIDs = append(result.DirectToolIDs, id.String())
	}
	return result
}

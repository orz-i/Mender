package bootstrap

import (
	"context"
	"testing"

	catalog "github.com/orz-i/Mender/backend/internal/catalog/domain"
	mcpdelivery "github.com/orz-i/Mender/backend/internal/mcp/application/delivery"
	mcp "github.com/orz-i/Mender/backend/internal/mcp/domain"
	profileapp "github.com/orz-i/Mender/backend/internal/profile/application"
	profile "github.com/orz-i/Mender/backend/internal/profile/domain"
	tunnel "github.com/orz-i/Mender/backend/internal/tunnel/domain"
)

// Existing integration fixtures now explicitly configure their MCP projection;
// production Profile creation itself has no transport side effects.
func createMCPProfileFixture(t *testing.T, r *Runtime, ctx context.Context, request profileapp.CreateProfileRequest, mode string) (profile.Profile, error) {
	t.Helper()
	p, err := r.Profiles.Create.Execute(ctx, request)
	if err != nil {
		return p, err
	}
	_, err = r.ConfigureProfileMCPDelivery(ctx, p.Slug(), mcpdelivery.ConfigureRequest{Mode: mcp.Mode(mode), ExpectedProfileRevision: p.Revision()})
	return p, err
}

func addMCPProfileToolFixture(t *testing.T, r *Runtime, ctx context.Context, request profileapp.AddToolRequest, direct bool) (profile.Profile, error) {
	t.Helper()
	p, err := r.Profiles.AddTool.Execute(ctx, request)
	if err != nil {
		return p, err
	}
	snapshot, err := r.GetProfileMCPDelivery(ctx, p.Slug())
	if err != nil {
		return p, err
	}
	if snapshot.Config.Mode() != mcp.ModeHybrid {
		return p, nil
	}
	ids := snapshot.Config.DirectToolIDs()
	included := false
	selected := make([]catalog.ToolID, 0, len(ids)+1)
	for _, id := range ids {
		if id == request.ToolID {
			included = true
			if !direct {
				continue
			}
		}
		selected = append(selected, id)
	}
	if direct && !included {
		selected = append(selected, request.ToolID)
	}
	if included == direct {
		return p, nil
	}
	revision := snapshot.Config.Revision()
	tunnelID, _ := snapshot.Config.TunnelID()
	_, err = r.ConfigureProfileMCPDelivery(ctx, p.Slug(), mcpdelivery.ConfigureRequest{Mode: snapshot.Config.Mode(), TunnelID: tunnelID, DirectToolIDs: selected, ExpectedProfileRevision: p.Revision(), ExpectedRevision: &revision})
	return p, err
}

func setMCPProfileTunnelFixture(t *testing.T, r *Runtime, ctx context.Context, slug profile.Slug, id tunnel.TunnelID) (profile.Profile, error) {
	t.Helper()
	snapshot, err := r.GetProfileMCPDelivery(ctx, slug)
	if err != nil {
		return profile.Profile{}, err
	}
	revision := snapshot.Config.Revision()
	updated, err := r.ConfigureProfileMCPDelivery(ctx, slug, mcpdelivery.ConfigureRequest{Mode: snapshot.Config.Mode(), DirectToolIDs: snapshot.Config.DirectToolIDs(), TunnelID: id, ExpectedProfileRevision: snapshot.Profile.Revision(), ExpectedRevision: &revision})
	return updated.Profile, err
}

func profileMCPFixtureTunnel(t *testing.T, r *Runtime, ctx context.Context, p profile.Profile) (tunnel.TunnelID, bool) {
	t.Helper()
	snapshot, err := r.GetProfileMCPDelivery(ctx, p.Slug())
	if err != nil {
		t.Fatal(err)
	}
	return snapshot.Config.TunnelID()
}

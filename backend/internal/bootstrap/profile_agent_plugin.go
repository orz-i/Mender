package bootstrap

import (
	"context"

	profiledomain "github.com/orz-i/Mender/backend/internal/profile/domain"
	skillapplication "github.com/orz-i/Mender/backend/internal/skill/application"
)

// ExportProfileAgentPlugin exports the Profile-owned Skill snapshot and
// returns that snapshot revision, without claiming continuing freshness.
func (r *Runtime) ExportProfileAgentPlugin(ctx context.Context, slug profiledomain.Slug,
	request skillapplication.ExportAgentPluginRequest,
) (uint64, error) {
	return r.profileDelivery.ExportProfileAgentPlugin(ctx, slug, request)
}

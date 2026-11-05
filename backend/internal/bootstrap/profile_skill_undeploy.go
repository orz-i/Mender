package bootstrap

import (
	"context"

	profiledelivery "github.com/orz-i/Mender/backend/internal/profile/application/delivery"
	profiledomain "github.com/orz-i/Mender/backend/internal/profile/domain"
	skilldomain "github.com/orz-i/Mender/backend/internal/skill/domain"
)

// ErrSkillDeliveryOwned rejects unscoped operations on Profile-managed files.
var ErrSkillDeliveryOwned = profiledelivery.ErrSkillDeliveryOwned

func (r *Runtime) UndeployUnassignedSkill(ctx context.Context, id skilldomain.DeploymentID) error {
	return r.profileDelivery.UndeployUnassignedSkill(ctx, id)
}

func (r *Runtime) UndeployProfileSkill(ctx context.Context, slug profiledomain.Slug, id skilldomain.DeploymentID) error {
	return r.profileDelivery.UndeployProfileSkill(ctx, slug, id)
}

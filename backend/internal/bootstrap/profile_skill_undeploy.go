package bootstrap

import (
	"context"
	"errors"
	"fmt"

	profiledomain "github.com/orz-i/Mender/backend/internal/profile/domain"
	skillapplication "github.com/orz-i/Mender/backend/internal/skill/application"
	skilldomain "github.com/orz-i/Mender/backend/internal/skill/domain"
)

// ErrSkillDeliveryOwned rejects unscoped operations on Profile-managed files.
var ErrSkillDeliveryOwned = errors.New("skill deployment belongs to a Profile")

// UndeployUnassignedSkill is reserved for historical deliveries that predate
// Profile ownership. It cannot remove any explicitly Profile-owned content.
func (r *Runtime) UndeployUnassignedSkill(ctx context.Context, id skilldomain.DeploymentID) error {
	existing, err := r.Skills.store.GetDeployment(ctx, id)
	if err != nil {
		return err
	}
	if existing.OwnerProfileID() != "" {
		return ErrSkillDeliveryOwned
	}
	return r.Skills.Undeploy(ctx, id)
}

// UndeployProfileSkill checks ownership at the service boundary. Profile
// membership may have since changed, but a historical owned deployment
// can still be cleaned up through its original Profile identity.
func (r *Runtime) UndeployProfileSkill(ctx context.Context, slug profiledomain.Slug, id skilldomain.DeploymentID) error {
	profile, err := r.Profiles.Get.Execute(ctx, slug)
	if err != nil {
		return fmt.Errorf("undeploy Profile Skill: %w", err)
	}
	existing, err := r.Skills.store.GetDeployment(ctx, id)
	if err != nil {
		return fmt.Errorf("undeploy Profile Skill: %w", err)
	}
	if existing.OwnerProfileID() != profile.ID().String() {
		return skillapplication.ErrDeploymentNotFound
	}
	return r.Skills.Undeploy(ctx, id)
}

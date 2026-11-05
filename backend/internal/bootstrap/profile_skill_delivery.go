package bootstrap

import (
	"context"

	profiledomain "github.com/orz-i/Mender/backend/internal/profile/domain"
	skillapplication "github.com/orz-i/Mender/backend/internal/skill/application"
	skilldomain "github.com/orz-i/Mender/backend/internal/skill/domain"
)

// DeployProfileSkills delivers only this Profile's explicit Skill selection.
func (r *Runtime) DeployProfileSkills(ctx context.Context, slug profiledomain.Slug,
	target skilldomain.DeploymentTarget, root string,
) ([]skillapplication.DeploymentInspection, error) {
	return r.profileDelivery.DeployProfileSkills(ctx, slug, target, root)
}

// ListProfileSkillDeployments does not adopt historical unowned deliveries.
func (r *Runtime) ListProfileSkillDeployments(ctx context.Context, slug profiledomain.Slug) ([]skillapplication.DeploymentInspection, error) {
	return r.profileDelivery.ListProfileSkillDeployments(ctx, slug)
}

package delivery

import (
	"context"
	"errors"
	"fmt"

	profiledomain "github.com/orz-i/Mender/backend/internal/profile/domain"
	skillapplication "github.com/orz-i/Mender/backend/internal/skill/application"
	skilldomain "github.com/orz-i/Mender/backend/internal/skill/domain"
)

// DeployProfileSkills materializes the Profile's exact Skill selection into one
// explicitly selected host target. It does not publish Tools or grant Agent rights.
// Failure attempts to compensate only the deployments created by this call.
func (s *Service) DeployProfileSkills(
	ctx context.Context,
	slug profiledomain.Slug,
	target skilldomain.DeploymentTarget,
	root string,
) ([]skillapplication.DeploymentInspection, error) {
	if err := target.Validate(); err != nil {
		return nil, err
	}
	profile, err := s.profiles.Snapshot(ctx, slug)
	if err != nil {
		return nil, fmt.Errorf("deploy Profile Skills: %w", err)
	}
	selection := profile.Skills
	if len(selection) == 0 {
		return nil, fmt.Errorf("deploy Profile Skills: Profile has no selected Skills")
	}

	// Preflight all installed/trusted/selected snapshots and name collisions
	// before creating a single host directory or deployment row.
	source := s.source
	names := make(map[string]struct{}, len(selection))
	for _, id := range selection {
		item, err := source.GetDeployable(ctx, id)
		if err != nil {
			return nil, fmt.Errorf("deploy Profile Skills: resolve %s: %w", id, err)
		}
		if !item.Trusted || !item.Enabled {
			return nil, fmt.Errorf("deploy Profile Skills: %s is not trusted and enabled", id)
		}
		if _, exists := names[item.Name]; exists {
			return nil, fmt.Errorf("deploy Profile Skills: duplicate target Skill name %q", item.Name)
		}
		names[item.Name] = struct{}{}
	}
	ensureRevision := func() error {
		current, err := s.profiles.Revision(ctx, profile.ID)
		if err != nil {
			return fmt.Errorf("deploy Profile Skills: check revision: %w", err)
		}
		if current != profile.Revision {
			return fmt.Errorf("deploy Profile Skills: Profile changed while delivering")
		}
		return nil
	}

	created := make([]skillapplication.DeploymentInspection, 0, len(selection))
	rollback := func(cause error) ([]skillapplication.DeploymentInspection, error) {
		for index := len(created) - 1; index >= 0; index-- {
			if cleanupErr := s.deployments.Undeploy(context.WithoutCancel(ctx), created[index].Deployment.ID()); cleanupErr != nil {
				cause = errors.Join(cause, fmt.Errorf("compensate deployment %s: %w",
					created[index].Deployment.ID(), cleanupErr))
			}
		}
		return nil, cause
	}

	for _, id := range selection {
		if err := ctx.Err(); err != nil {
			return rollback(err)
		}
		if err := ensureRevision(); err != nil {
			return rollback(err)
		}
		deployment, err := s.deployments.DeployForProfile(ctx, id, target, root,
			profile.ID.String(), profile.Revision)
		if err != nil {
			return rollback(fmt.Errorf("deploy Profile Skills: %s: %w", id, err))
		}
		created = append(created, skillapplication.DeploymentInspection{
			Deployment: deployment, Drift: skilldomain.DeploymentCurrent,
			CurrentDigest: deployment.Digest(),
		})
	}
	if err := ensureRevision(); err != nil {
		return rollback(err)
	}
	return created, nil
}

// ListProfileSkillDeployments returns only records already explicitly owned by
// the Profile; legacy unowned materializations remain visible in the Skill
// Library and are never adopted merely by viewing this collection.
func (s *Service) ListProfileSkillDeployments(
	ctx context.Context, slug profiledomain.Slug,
) ([]skillapplication.DeploymentInspection, error) {
	profile, err := s.profiles.Snapshot(ctx, slug)
	if err != nil {
		return nil, fmt.Errorf("list Profile Skill deployments: %w", err)
	}
	return s.records.ListOwnedDeployments(ctx, profile.ID.String())
}

package application

import (
	"context"
	"fmt"

	profiledomain "github.com/orz-i/Mender/backend/internal/profile/domain"
	skilldomain "github.com/orz-i/Mender/backend/internal/skill/domain"
)

// SkillStatusLookup resolves canonical installation state. Profile owns only
// exact references; the Skill Library remains the content/trust authority.
type SkillStatusLookup interface {
	GetSkillState(context.Context, skilldomain.InstallationID) (trusted, enabled bool, err error)
}

type ReplaceSkillsRequest struct {
	ProfileSlug     profiledomain.Slug
	InstallationIDs []skilldomain.InstallationID
}

type ReplaceSkills struct {
	repository Repository
	skills     SkillStatusLookup
}

func NewReplaceSkills(repository Repository, skills SkillStatusLookup) ReplaceSkills {
	return ReplaceSkills{repository: repository, skills: skills}
}

func (uc ReplaceSkills) Execute(ctx context.Context, request ReplaceSkillsRequest) (profiledomain.Profile, error) {
	profile, err := uc.repository.GetProfileBySlug(ctx, request.ProfileSlug)
	if err != nil {
		return profiledomain.Profile{}, fmt.Errorf("replace profile skills: %w", err)
	}
	updated, err := profile.ReplaceSkills(request.InstallationIDs)
	if err != nil {
		return profiledomain.Profile{}, fmt.Errorf("replace profile skills: %w: %w", ErrInvalidSkillMembership, err)
	}
	for _, id := range updated.Skills() {
		if uc.skills == nil {
			return profiledomain.Profile{}, fmt.Errorf("replace profile skills: skill lookup unavailable")
		}
		trusted, enabled, err := uc.skills.GetSkillState(ctx, id)
		if err != nil {
			return profiledomain.Profile{}, fmt.Errorf("replace profile skills: installation %s: %w: %w", id, ErrInvalidSkillMembership, err)
		}
		if !trusted || !enabled {
			return profiledomain.Profile{}, fmt.Errorf("replace profile skills: installation %s: %w", id, ErrSkillNotDeployable)
		}
	}
	if err := uc.repository.SaveProfile(ctx, updated); err != nil {
		return profiledomain.Profile{}, fmt.Errorf("replace profile skills: save: %w", err)
	}
	return updated.WithRevision(updated.Revision() + 1), nil
}

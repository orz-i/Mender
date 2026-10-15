package application

import (
	"context"
	"fmt"

	profiledomain "github.com/orz-i/Mender/backend/internal/profile/domain"
)

type CreateProfileRequest struct {
	Slug        string
	DisplayName string
	Mode        profiledomain.ExposureMode
}

type CreateProfile struct {
	repository Repository
	ids        ProfileIDGenerator
}

func NewCreateProfile(repository Repository, ids ProfileIDGenerator) CreateProfile {
	return CreateProfile{repository: repository, ids: ids}
}

func (uc CreateProfile) Execute(
	ctx context.Context,
	request CreateProfileRequest,
) (profiledomain.Profile, error) {
	slug, err := profiledomain.NewSlug(request.Slug)
	if err != nil {
		return profiledomain.Profile{}, fmt.Errorf("create profile: %w", err)
	}
	id, err := uc.ids.NewProfileID()
	if err != nil {
		return profiledomain.Profile{}, fmt.Errorf("create profile: allocate id: %w", err)
	}
	profile, err := profiledomain.NewProfileWithBindings(
		id,
		slug,
		request.DisplayName,
		request.Mode,
		nil,
	)
	if err != nil {
		return profiledomain.Profile{}, fmt.Errorf("create profile: %w", err)
	}
	if err := uc.repository.CreateProfile(ctx, profile); err != nil {
		return profiledomain.Profile{}, fmt.Errorf("create profile: %w", err)
	}
	return profile, nil
}

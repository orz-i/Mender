package application

import (
	"context"
	"fmt"

	profiledomain "github.com/orz-i/Mender/backend/internal/profile/domain"
)

type GetProfileByID struct {
	repository Repository
}

func NewGetProfileByID(repository Repository) GetProfileByID {
	return GetProfileByID{repository: repository}
}

func (uc GetProfileByID) Execute(
	ctx context.Context,
	id profiledomain.ProfileID,
) (profiledomain.Profile, error) {
	profile, err := uc.repository.GetProfileByID(ctx, id)
	if err != nil {
		return profiledomain.Profile{}, fmt.Errorf("get profile by id: %w", err)
	}
	return profile, nil
}

type GetProfile struct {
	repository Repository
}

func NewGetProfile(repository Repository) GetProfile {
	return GetProfile{repository: repository}
}

func (uc GetProfile) Execute(
	ctx context.Context,
	slug profiledomain.Slug,
) (profiledomain.Profile, error) {
	profile, err := uc.repository.GetProfileBySlug(ctx, slug)
	if err != nil {
		return profiledomain.Profile{}, fmt.Errorf("get profile: %w", err)
	}
	return profile, nil
}

type ListProfiles struct {
	repository Repository
}

func NewListProfiles(repository Repository) ListProfiles {
	return ListProfiles{repository: repository}
}

func (uc ListProfiles) Execute(ctx context.Context) ([]profiledomain.Profile, error) {
	profiles, err := uc.repository.ListProfiles(ctx)
	if err != nil {
		return nil, fmt.Errorf("list profiles: %w", err)
	}
	return profiles, nil
}

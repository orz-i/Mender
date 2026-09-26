package application

import (
	"context"
	"fmt"

	catalogdomain "github.com/orz-i/Mender/backend/internal/catalog/domain"
	profiledomain "github.com/orz-i/Mender/backend/internal/profile/domain"
)

type AddToolRequest struct {
	ProfileSlug profiledomain.Slug
	ToolID      catalogdomain.ToolID
	Direct      bool
}

type AddTool struct {
	repository Repository
	tools      ToolLookup
}

func NewAddTool(repository Repository, tools ToolLookup) AddTool {
	return AddTool{repository: repository, tools: tools}
}

func (uc AddTool) Execute(
	ctx context.Context,
	request AddToolRequest,
) (profiledomain.Profile, error) {
	if _, err := uc.tools.GetTool(ctx, request.ToolID); err != nil {
		return profiledomain.Profile{}, fmt.Errorf("add profile tool: resolve tool: %w", err)
	}
	profile, err := uc.repository.GetProfileBySlug(ctx, request.ProfileSlug)
	if err != nil {
		return profiledomain.Profile{}, fmt.Errorf("add profile tool: %w", err)
	}
	updated, err := profile.AddTool(request.ToolID, request.Direct)
	if err != nil {
		return profiledomain.Profile{}, fmt.Errorf("add profile tool: %w", err)
	}
	if err := uc.repository.SaveProfile(ctx, updated); err != nil {
		return profiledomain.Profile{}, fmt.Errorf("add profile tool: save: %w", err)
	}
	return updated, nil
}

type RemoveToolRequest struct {
	ProfileSlug profiledomain.Slug
	ToolID      catalogdomain.ToolID
}

type RemoveTool struct {
	repository Repository
}

func NewRemoveTool(repository Repository) RemoveTool {
	return RemoveTool{repository: repository}
}

func (uc RemoveTool) Execute(
	ctx context.Context,
	request RemoveToolRequest,
) (profiledomain.Profile, error) {
	profile, err := uc.repository.GetProfileBySlug(ctx, request.ProfileSlug)
	if err != nil {
		return profiledomain.Profile{}, fmt.Errorf("remove profile tool: %w", err)
	}
	updated, err := profile.RemoveTool(request.ToolID)
	if err != nil {
		return profiledomain.Profile{}, fmt.Errorf("remove profile tool: %w", err)
	}
	if err := uc.repository.SaveProfile(ctx, updated); err != nil {
		return profiledomain.Profile{}, fmt.Errorf("remove profile tool: save: %w", err)
	}
	return updated, nil
}

package application

import (
	"context"
	"fmt"

	profiledomain "github.com/orz-i/Mender/backend/internal/profile/domain"
)

type ReplaceToolsRequest struct {
	ProfileSlug profiledomain.Slug
	Tools       []ToolBindingInput
}

type ReplaceTools struct {
	repository Repository
	tools      ToolLookup
}

func NewReplaceTools(repository Repository, tools ToolLookup) ReplaceTools {
	return ReplaceTools{
		repository: repository,
		tools:      tools,
	}
}

func (uc ReplaceTools) Execute(
	ctx context.Context,
	request ReplaceToolsRequest,
) (profiledomain.Profile, error) {
	profile, err := uc.repository.GetProfileBySlug(ctx, request.ProfileSlug)
	if err != nil {
		return profiledomain.Profile{}, fmt.Errorf("replace profile tools: %w", err)
	}

	bindings := make([]profiledomain.ToolBinding, 0, len(request.Tools))
	for _, item := range request.Tools {
		binding, err := newToolBinding(item)
		if err != nil {
			return profiledomain.Profile{}, fmt.Errorf(
				"replace profile tools: binding %s: %w: %w",
				item.ToolID,
				ErrInvalidBinding,
				err,
			)
		}
		bindings = append(bindings, binding)
	}

	updated, err := profile.ReplaceBindings(bindings)
	if err != nil {
		return profiledomain.Profile{}, fmt.Errorf(
			"replace profile tools: %w: %w",
			ErrInvalidBinding,
			err,
		)
	}
	if err := validateBindingToolsAndNames(ctx, updated, uc.tools); err != nil {
		return profiledomain.Profile{}, fmt.Errorf("replace profile tools: %w", err)
	}
	if err := uc.repository.SaveProfile(ctx, updated); err != nil {
		return profiledomain.Profile{}, fmt.Errorf(
			"replace profile tools: save: %w",
			err,
		)
	}
	return updated, nil
}

package application

import (
	"context"
	"fmt"

	catalogdomain "github.com/orz-i/Mender/backend/internal/catalog/domain"
	profiledomain "github.com/orz-i/Mender/backend/internal/profile/domain"
)

type ReplaceToolSelection struct {
	ToolID catalogdomain.ToolID
	Direct bool
}

type ReplaceToolsRequest struct {
	ProfileSlug profiledomain.Slug
	Tools       []ReplaceToolSelection
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

	selections := make([]profiledomain.ToolSelection, 0, len(request.Tools))
	for _, item := range request.Tools {
		if _, err := uc.tools.GetTool(ctx, item.ToolID); err != nil {
			return profiledomain.Profile{}, fmt.Errorf(
				"replace profile tools: resolve %s: %w",
				item.ToolID,
				err,
			)
		}
		selections = append(selections, profiledomain.ToolSelection{
			ToolID: item.ToolID,
			Direct: item.Direct,
		})
	}

	updated, err := profile.ReplaceTools(selections)
	if err != nil {
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

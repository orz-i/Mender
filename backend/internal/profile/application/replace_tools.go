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
	repository  Repository
	tools       ToolLookup
	credentials CredentialLookup
}

func NewReplaceTools(
	repository Repository,
	tools ToolLookup,
	credentials ...CredentialLookup,
) ReplaceTools {
	var credentialLookup CredentialLookup
	if len(credentials) != 0 {
		credentialLookup = credentials[0]
	}
	return ReplaceTools{
		repository:  repository,
		tools:       tools,
		credentials: credentialLookup,
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
	if err := validateBindings(ctx, updated, uc.tools, uc.credentials); err != nil {
		return profiledomain.Profile{}, fmt.Errorf("replace profile tools: %w", err)
	}
	if err := uc.repository.SaveProfile(ctx, updated); err != nil {
		return profiledomain.Profile{}, fmt.Errorf(
			"replace profile tools: save: %w",
			err,
		)
	}
	return updated.WithRevision(updated.Revision() + 1), nil
}

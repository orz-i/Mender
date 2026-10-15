package application

import (
	"context"
	"fmt"

	catalogdomain "github.com/orz-i/Mender/backend/internal/catalog/domain"
	credentialdomain "github.com/orz-i/Mender/backend/internal/credential/domain"
	profiledomain "github.com/orz-i/Mender/backend/internal/profile/domain"
)

type AddToolRequest struct {
	ProfileSlug           profiledomain.Slug
	ToolID                catalogdomain.ToolID
	Direct                bool
	Alias                 string
	TitleOverride         string
	DescriptionOverride   string
	PolicyConstraint      profiledomain.PolicyConstraint
	ConcurrencyConstraint profiledomain.ConcurrencyConstraint
	CredentialRefs        []credentialdomain.Reference
}

type AddTool struct {
	repository  Repository
	tools       ToolLookup
	credentials CredentialLookup
}

func NewAddTool(
	repository Repository,
	tools ToolLookup,
	credentials ...CredentialLookup,
) AddTool {
	var credentialLookup CredentialLookup
	if len(credentials) != 0 {
		credentialLookup = credentials[0]
	}
	return AddTool{
		repository:  repository,
		tools:       tools,
		credentials: credentialLookup,
	}
}

func (uc AddTool) Execute(
	ctx context.Context,
	request AddToolRequest,
) (profiledomain.Profile, error) {
	profile, err := uc.repository.GetProfileBySlug(ctx, request.ProfileSlug)
	if err != nil {
		return profiledomain.Profile{}, fmt.Errorf("add profile tool: %w", err)
	}
	binding, err := newToolBinding(ToolBindingInput{
		ToolID:                request.ToolID,
		Direct:                request.Direct,
		Alias:                 request.Alias,
		TitleOverride:         request.TitleOverride,
		DescriptionOverride:   request.DescriptionOverride,
		PolicyConstraint:      request.PolicyConstraint,
		ConcurrencyConstraint: request.ConcurrencyConstraint,
		CredentialRefs:        append([]credentialdomain.Reference(nil), request.CredentialRefs...),
	})
	if err != nil {
		return profiledomain.Profile{}, fmt.Errorf(
			"add profile tool: %w: %w",
			ErrInvalidBinding,
			err,
		)
	}
	updated, err := profile.AddBinding(binding)
	if err != nil {
		return profiledomain.Profile{}, fmt.Errorf(
			"add profile tool: %w: %w",
			ErrInvalidBinding,
			err,
		)
	}
	if err := validateBindings(ctx, updated, uc.tools, uc.credentials); err != nil {
		return profiledomain.Profile{}, fmt.Errorf("add profile tool: %w", err)
	}
	if err := uc.repository.SaveProfile(ctx, updated); err != nil {
		return profiledomain.Profile{}, fmt.Errorf("add profile tool: save: %w", err)
	}
	return updated.WithRevision(updated.Revision() + 1), nil
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
	return updated.WithRevision(updated.Revision() + 1), nil
}

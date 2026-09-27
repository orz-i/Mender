package bootstrap

import (
	"context"
	"errors"
	"fmt"

	catalogdomain "github.com/orz-i/Mender/backend/internal/catalog/domain"
	invocationapplication "github.com/orz-i/Mender/backend/internal/invocation/application"
	profileapplication "github.com/orz-i/Mender/backend/internal/profile/application"
	profiledomain "github.com/orz-i/Mender/backend/internal/profile/domain"
)

type invocationProfileScopeAuthorizer struct {
	get profileapplication.GetProfileByID
}

func newInvocationProfileScopeAuthorizer(
	get profileapplication.GetProfileByID,
) invocationapplication.ProfileScopeAuthorizer {
	return invocationProfileScopeAuthorizer{get: get}
}

func (a invocationProfileScopeAuthorizer) AllowsTool(
	ctx context.Context,
	profileReference string,
	toolID catalogdomain.ToolID,
) (bool, error) {
	profileID, err := profiledomain.ParseProfileID(profileReference)
	if err != nil {
		return false, nil
	}

	profile, err := a.get.Execute(ctx, profileID)
	if errors.Is(err, profileapplication.ErrProfileNotFound) {
		return false, nil
	}
	if err != nil {
		return false, fmt.Errorf("resolve profile scope: %w", err)
	}
	return profile.Contains(toolID), nil
}

package bootstrap

import (
	"fmt"

	governancedomain "github.com/orz-i/Mender/backend/internal/governance/domain"
	profiledomain "github.com/orz-i/Mender/backend/internal/profile/domain"
)

// OwnerPrincipal returns the local Runtime owner's OS-backed Principal.
func (r *Runtime) OwnerPrincipal() governancedomain.Principal {
	return r.ownerPrincipal
}

func profileActivationContext(
	controlContext governancedomain.Context,
	profile profiledomain.Profile,
) (governancedomain.Context, error) {
	if err := controlContext.Validate(); err != nil {
		return governancedomain.Context{}, err
	}
	scope, err := governancedomain.ProfileScope(profile.ID().String())
	if err != nil {
		return governancedomain.Context{}, fmt.Errorf("profile activation scope: %w", err)
	}
	return governancedomain.NewContext(
		controlContext.Origin(),
		controlContext.Principal(),
		scope,
	)
}

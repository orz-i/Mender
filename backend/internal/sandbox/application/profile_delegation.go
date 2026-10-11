package application

import (
	"context"

	governancedomain "github.com/orz-i/Mender/backend/internal/governance/domain"
	profiledomain "github.com/orz-i/Mender/backend/internal/profile/domain"
	sandboxdomain "github.com/orz-i/Mender/backend/internal/sandbox/domain"
)

// ProfileServiceID accepts only the trusted Profile MCP identity and exact scope.
// It must never infer a remote human identity or consult model-controlled input.
func ProfileServiceID(caller governancedomain.Context) (string, bool) {
	if caller.Validate() != nil || caller.Origin() != governancedomain.OriginMCP ||
		caller.Principal().Kind() != governancedomain.PrincipalService ||
		caller.Scope().Kind() != governancedomain.ScopeProfile {
		return "", false
	}
	profileID := caller.Scope().Reference()
	if _, err := profiledomain.ParseProfileID(profileID); err != nil ||
		caller.Principal().Reference() != "profile:"+profileID {
		return "", false
	}
	return profileID, true
}

func IsOwner(caller governancedomain.Context, sandbox sandboxdomain.Sandbox) bool {
	subject := caller.Principal()
	return string(subject.Kind()) == sandbox.OwnerKind() &&
		subject.Reference() == sandbox.OwnerRef()
}

// IsGrantManager identifies an authenticated local control-plane operation.
// A Profile MCP service never manages grants, even for a service-owned Sandbox.
func IsGrantManager(caller governancedomain.Context, sandbox sandboxdomain.Sandbox) bool {
	if caller.Validate() != nil || !IsOwner(caller, sandbox) ||
		caller.Principal().Kind() != governancedomain.PrincipalLocalUser ||
		caller.Scope().Kind() != governancedomain.ScopeCatalog {
		return false
	}
	switch caller.Origin() {
	case governancedomain.OriginCLI, governancedomain.OriginDesktop, governancedomain.OriginManagementHTTP:
		return true
	}
	return false
}

// ProfileDelegationLookup checks current grants before admission. The durable
// lease store must re-check the same grant atomically when reserving execution.
type ProfileDelegationLookup interface {
	HasProfileGrant(context.Context, sandboxdomain.ID, string, sandboxdomain.Capability) (bool, error)
}

type ProfileGrant struct {
	ProfileID    string                     `json:"profileId"`
	Capabilities []sandboxdomain.Capability `json:"capabilities"`
}

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

// ProfileInvocationID recognizes two deliberately distinct, host-authenticated
// entry points for a Profile-scoped Tool call. MCP requires a service Principal
// pinned to that Profile, while local owner frontends retain their actual
// local-user Principal and original CLI/Desktop/HTTP origin. This is never a
// general substitute for ProfileServiceID when authenticating an MCP session.
func ProfileInvocationID(caller governancedomain.Context) (string, bool) {
	if id, ok := ProfileServiceID(caller); ok {
		return id, true
	}
	if caller.Validate() != nil || caller.Scope().Kind() != governancedomain.ScopeProfile || caller.Principal().Kind() != governancedomain.PrincipalLocalUser {
		return "", false
	}
	switch caller.Origin() {
	case governancedomain.OriginCLI, governancedomain.OriginDesktop, governancedomain.OriginManagementHTTP:
	default:
		return "", false
	}
	id := caller.Scope().Reference()
	if _, err := profiledomain.ParseProfileID(id); err != nil {
		return "", false
	}
	return id, true
}

// LocalCatalogOwner is an explicitly authenticated management invocation,
// never a Profile service, Plugin Principal or anonymous API caller.
func LocalCatalogOwner(caller governancedomain.Context) bool {
	if caller.Validate() != nil || caller.Scope().Kind() != governancedomain.ScopeCatalog || caller.Principal().Kind() != governancedomain.PrincipalLocalUser {
		return false
	}
	switch caller.Origin() {
	case governancedomain.OriginCLI, governancedomain.OriginDesktop, governancedomain.OriginManagementHTTP:
		return true
	}
	return false
}

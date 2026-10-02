package bootstrap

import governancedomain "github.com/orz-i/Mender/backend/internal/governance/domain"

// OwnerPrincipal returns the local Runtime owner's OS-backed Principal.
func (r *Runtime) OwnerPrincipal() governancedomain.Principal {
	return r.ownerPrincipal
}

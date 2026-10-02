package localuser

import (
	"fmt"
	"os/user"
	"strings"

	governancedomain "github.com/orz-i/Mender/backend/internal/governance/domain"
)

// CurrentPrincipal returns the local Runtime owner's OS-backed Principal.
func CurrentPrincipal() (governancedomain.Principal, error) {
	current, err := user.Current()
	if err != nil {
		return governancedomain.Principal{}, fmt.Errorf("resolve local user principal: %w", err)
	}
	reference := strings.TrimSpace(current.Uid)
	if reference == "" {
		reference = strings.TrimSpace(current.Username)
	}
	if reference == "" {
		return governancedomain.Principal{}, fmt.Errorf(
			"resolve local user principal: OS identity is empty",
		)
	}
	principal, err := governancedomain.NewPrincipal(
		governancedomain.PrincipalLocalUser,
		"os:"+reference,
	)
	if err != nil {
		return governancedomain.Principal{}, fmt.Errorf(
			"resolve local user principal: %w",
			err,
		)
	}
	return principal, nil
}

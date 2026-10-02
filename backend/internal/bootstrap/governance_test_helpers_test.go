package bootstrap

import (
	"testing"

	governancedomain "github.com/orz-i/Mender/backend/internal/governance/domain"
)

func testPrincipal(t *testing.T, label string) governancedomain.Principal {
	t.Helper()
	if label == "" {
		label = "default"
	}
	principal, err := governancedomain.NewPrincipal(
		governancedomain.PrincipalService,
		"test:"+label,
	)
	if err != nil {
		t.Fatal(err)
	}
	return principal
}

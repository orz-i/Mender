package bootstrap

import (
	"context"
	"testing"

	runtimegovernancedomain "github.com/orz-i/Mender/backend/internal/runtimegovernance/domain"
)

func ensureTestingRuntimeIsolation(t *testing.T, r *Runtime) {
	t.Helper()
	if !r.RuntimeGovernance.processIsolation.Info().RestrictedHostAvailable {
		if _, err := r.RuntimeGovernance.UpdateIsolation(
			context.Background(),
			runtimegovernancedomain.IsolationSpec{
				WorkspaceShell:   runtimegovernancedomain.IsolationHost,
				ExternalProvider: runtimegovernancedomain.IsolationHost,
				TunnelClient:     runtimegovernancedomain.IsolationHost,
			},
		); err != nil {
			t.Fatal(err)
		}
	}
}

package bootstrap

import (
	"context"
	"path/filepath"
	"testing"

	runtimegovernancedomain "github.com/orz-i/Mender/backend/internal/runtimegovernance/domain"
)

func TestRuntimeGovernanceInitialIsolationFollowsBackendAvailability(t *testing.T) {
	t.Parallel()

	governance, err := NewRuntimeGovernance(
		context.Background(),
		filepath.Join(t.TempDir(), "mender.db"),
	)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = governance.Close() }()

	isolation, err := governance.GetIsolation.Execute(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	expectedWorkspace := runtimegovernancedomain.IsolationHost
	if governance.processIsolation.Info().RestrictedHostAvailable {
		expectedWorkspace = runtimegovernancedomain.IsolationRestrictedHost
	}
	if isolation.WorkspaceShell() != expectedWorkspace {
		t.Fatalf(
			"workspace isolation = %q, want %q (backend=%+v)",
			isolation.WorkspaceShell(),
			expectedWorkspace,
			governance.processIsolation.Info(),
		)
	}
	if isolation.ExternalProvider() != runtimegovernancedomain.IsolationHost {
		t.Fatalf("external Provider isolation = %q, want host", isolation.ExternalProvider())
	}
}

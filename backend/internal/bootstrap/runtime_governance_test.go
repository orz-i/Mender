package bootstrap

import (
	"context"
	"path/filepath"
	"testing"

	runtimegovernancedomain "github.com/orz-i/Mender/backend/internal/runtimegovernance/domain"
)

func TestRuntimeGovernanceRejectsRestrictedHostWhenBackendUnavailable(t *testing.T) {
	governance, err := NewRuntimeGovernance(
		context.Background(),
		filepath.Join(t.TempDir(), "mender.db"),
	)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = governance.Close() }()

	governance.processIsolation = nil
	if _, err := governance.UpdateIsolation(
		context.Background(),
		runtimegovernancedomain.IsolationSpec{
			WorkspaceShell:   runtimegovernancedomain.IsolationRestrictedHost,
			ExternalProvider: runtimegovernancedomain.IsolationHost,
			TunnelClient:     runtimegovernancedomain.IsolationHost,
		},
	); err == nil {
		t.Fatal("UpdateIsolation accepted restricted_host without backend")
	}
}

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
	if isolation.ExternalProvider() != expectedWorkspace {
		t.Fatalf("external Provider isolation = %q, want %q", isolation.ExternalProvider(), expectedWorkspace)
	}
	if isolation.TunnelClient() != expectedWorkspace {
		t.Fatalf(
			"Tunnel client isolation = %q, want %q (backend=%+v)",
			isolation.TunnelClient(),
			expectedWorkspace,
			governance.processIsolation.Info(),
		)
	}
}

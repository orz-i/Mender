package bootstrap

import (
	"context"

	invocationdomain "github.com/orz-i/Mender/backend/internal/invocation/domain"
	sandboxapp "github.com/orz-i/Mender/backend/internal/sandbox/application"
	sandboxdomain "github.com/orz-i/Mender/backend/internal/sandbox/domain"
)

// dockerBackendGate preserves the exact host-owned Plugin/Router binding.
// A registered Docker plugin is NOT necessarily a currently healthy Engine.
// Check the last observed state before opening a new execution lease, and
// verify the pinned daemon ID again inside the concrete Backend.Open.
type dockerBackendGate struct {
	owner   *Sandboxes
	backend sandboxapp.Backend
}

func (b dockerBackendGate) Kind() sandboxdomain.BackendKind {
	return b.backend.Kind()
}
func (b dockerBackendGate) Capabilities() sandboxdomain.Capabilities {
	return b.backend.Capabilities()
}
func (b dockerBackendGate) Open(ctx context.Context, item sandboxdomain.Sandbox, run sandboxdomain.Run) (sandboxapp.Session, error) {
	if b.owner == nil || !b.owner.DockerAvailable() {
		return nil, sandboxdomain.ErrBackendUnavailable
	}
	session, err := b.backend.Open(ctx, item, run)
	if err != nil {
		return nil, err
	}
	workspace, ok := session.(sandboxapp.WorkspaceSession)
	if !ok {
		if session != nil {
			_ = session.Close()
		}
		return nil, sandboxdomain.ErrBackendUnavailable
	}
	if !b.owner.DockerAvailable() {
		_ = workspace.Close()
		return nil, sandboxdomain.ErrBackendUnavailable
	}
	return dockerWorkspaceSessionGate{WorkspaceSession: workspace, owner: b.owner}, nil
}

// Already-acquired sessions must not silently keep executing after the host
// has observed daemon loss or identity drift. Lease cleanup remains delegated
// to the exact original session; no lease is auto-released by health polling.
type dockerWorkspaceSessionGate struct {
	sandboxapp.WorkspaceSession
	owner *Sandboxes
}

func (s dockerWorkspaceSessionGate) ExecuteWorkspace(ctx context.Context, cap sandboxdomain.Capability, args invocationdomain.JSONValue) (invocationdomain.Outcome, error) {
	if s.owner == nil || !s.owner.DockerAvailable() {
		return invocationdomain.Outcome{}, sandboxdomain.ErrBackendUnavailable
	}
	return s.WorkspaceSession.ExecuteWorkspace(ctx, cap, args)
}

var _ sandboxapp.Backend = dockerBackendGate{}
var _ sandboxapp.WorkspaceSession = dockerWorkspaceSessionGate{}

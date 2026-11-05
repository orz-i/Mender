package bootstrap

import (
	"context"

	sandboxapp "github.com/orz-i/Mender/backend/internal/sandbox/application"
	sandboxdomain "github.com/orz-i/Mender/backend/internal/sandbox/domain"
	workspaceapp "github.com/orz-i/Mender/backend/internal/workspace/application"
	workspace "github.com/orz-i/Mender/backend/internal/workspace/domain"
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
	workspace, ok := session.(workspaceapp.Session)
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
	return dockerWorkspaceSessionGate{Session: workspace, owner: b.owner}, nil
}

// Already-acquired sessions must not silently keep executing after the host
// has observed daemon loss or identity drift. Lease cleanup remains delegated
// to the exact original session; no lease is auto-released by health polling.
type dockerWorkspaceSessionGate struct {
	workspaceapp.Session
	owner *Sandboxes
}

func (s dockerWorkspaceSessionGate) Read(ctx context.Context, in workspace.ReadRequest) (workspace.ReadResult, error) {
	if s.owner == nil || !s.owner.DockerAvailable() {
		return workspace.ReadResult{}, sandboxdomain.ErrBackendUnavailable
	}
	return s.Session.Read(ctx, in)
}

func (s dockerWorkspaceSessionGate) Search(ctx context.Context, in workspace.SearchRequest) (workspace.SearchResult, error) {
	if s.owner == nil || !s.owner.DockerAvailable() {
		return workspace.SearchResult{}, sandboxdomain.ErrBackendUnavailable
	}
	return s.Session.Search(ctx, in)
}

func (s dockerWorkspaceSessionGate) Edit(ctx context.Context, in workspace.EditRequest) (workspace.EditResult, error) {
	if s.owner == nil || !s.owner.DockerAvailable() {
		return workspace.EditResult{}, sandboxdomain.ErrBackendUnavailable
	}
	return s.Session.Edit(ctx, in)
}

func (s dockerWorkspaceSessionGate) Shell(ctx context.Context, in workspace.ShellRequest) (workspace.ShellResult, error) {
	if s.owner == nil || !s.owner.DockerAvailable() {
		return workspace.ShellResult{}, sandboxdomain.ErrBackendUnavailable
	}
	return s.Session.Shell(ctx, in)
}

var _ sandboxapp.Backend = dockerBackendGate{}
var _ workspaceapp.Session = dockerWorkspaceSessionGate{}

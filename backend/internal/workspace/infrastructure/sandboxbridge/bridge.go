// Package sandboxbridge adapts the admitted Sandbox router to Workspace ports.
package sandboxbridge

import (
	"context"
	"errors"
	"fmt"

	governance "github.com/orz-i/Mender/backend/internal/governance/domain"
	sandboxapp "github.com/orz-i/Mender/backend/internal/sandbox/application"
	sandbox "github.com/orz-i/Mender/backend/internal/sandbox/domain"
	workspaceapp "github.com/orz-i/Mender/backend/internal/workspace/application"
	workspace "github.com/orz-i/Mender/backend/internal/workspace/domain"
)

type Provider struct{ Router sandboxapp.ExecutionRouter }

func (p Provider) Acquire(ctx context.Context, caller governance.Context, target string, operation workspace.Operation) (*workspaceapp.Lease, error) {
	if p.Router == nil {
		return nil, sandbox.ErrBackendUnavailable
	}
	capability := sandbox.Capability(operation)
	switch operation {
	case workspace.Read, workspace.Search, workspace.Edit, workspace.Shell:
	default:
		return nil, sandbox.ErrCapabilityUnavailable
	}
	placement, err := p.Router.Resolve(ctx, sandbox.ID(target), capability)
	if err != nil {
		return nil, fmt.Errorf("resolve Sandbox Workspace execution: %w", err)
	}
	if placement.Sandbox.ID().String() != target || placement.Required != capability {
		return nil, sandbox.ErrConflict
	}
	lease, err := p.Router.Acquire(ctx, caller, placement)
	if err != nil {
		return nil, fmt.Errorf("acquire Sandbox Workspace execution: %w", err)
	}
	if lease == nil {
		return nil, fmt.Errorf("acquire Sandbox Workspace execution: missing lease")
	}
	session, ok := lease.Session.(workspaceapp.Session)
	if !ok {
		return nil, errors.Join(sandbox.ErrCapabilityUnavailable, lease.Close())
	}
	compatibility := workspaceapp.Compatibility{}
	switch placement.Sandbox.Backend() {
	case sandbox.BackendSSHRemote:
		compatibility.MaxArgumentBytes = 1 << 20
		compatibility.ArgumentLimitMessage = "SSH workspace arguments exceed limit"
		compatibility.ArgumentLimitIsToolError = true
	case sandbox.BackendExternal:
		compatibility.PreserveDefaults = true
		compatibility.MaxArgumentBytes = 1 << 20
		compatibility.ArgumentLimitMessage = "sandbox plugin arguments exceed limit"
	}
	return workspaceapp.NewLease(session, lease.Close, compatibility), nil
}

var _ workspaceapp.LeaseProvider = Provider{}

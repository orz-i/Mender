package bootstrap

import (
	"context"

	sandbox "github.com/orz-i/Mender/backend/internal/sandbox/domain"
	workspace "github.com/orz-i/Mender/backend/internal/workspace/application"
)

// Sandbox authority is supplied to Workspace as a narrow read-only consumer
// port. Workspace must not import the Sandbox domain or execute its backends.
type workspaceSandboxLookup struct{ sandboxes *Sandboxes }

func (a workspaceSandboxLookup) GetSandbox(ctx context.Context, id string) (workspace.SandboxBinding, error) {
	sid, err := sandbox.ParseID(id)
	if err != nil {
		return workspace.SandboxBinding{}, err
	}
	item, err := a.sandboxes.Get(ctx, sid)
	if err != nil {
		return workspace.SandboxBinding{}, err
	}
	spec := item.Spec()
	return workspace.SandboxBinding{Backend: string(spec.Backend), LocalRoot: spec.LocalRoot, OwnerKind: spec.OwnerKind, OwnerRef: spec.OwnerRef}, nil
}

var _ workspace.SandboxLookup = workspaceSandboxLookup{}

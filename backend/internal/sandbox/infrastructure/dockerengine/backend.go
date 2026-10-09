package dockerengine

import (
	"context"
	"fmt"
	"sync/atomic"

	mobyclient "github.com/moby/moby/client"
	invocationdomain "github.com/orz-i/Mender/backend/internal/invocation/domain"
	sandboxapplication "github.com/orz-i/Mender/backend/internal/sandbox/application"
	sandboxdomain "github.com/orz-i/Mender/backend/internal/sandbox/domain"
)

// SessionAPI is a deliberately bounded subset of Docker Engine required for
// file transfer and in-container exec. No daemon-wide list, mounts, builds,
// arbitrary container lifecycle or privilege changes are available to a Tool.
type SessionAPI interface {
	ContainerInspect(context.Context, string, mobyclient.ContainerInspectOptions) (mobyclient.ContainerInspectResult, error)
	ExecCreate(context.Context, string, mobyclient.ExecCreateOptions) (mobyclient.ExecCreateResult, error)
	ExecAttach(context.Context, string, mobyclient.ExecAttachOptions) (mobyclient.ExecAttachResult, error)
	ExecInspect(context.Context, string, mobyclient.ExecInspectOptions) (mobyclient.ExecInspectResult, error)
}

// Backend is registered only when the configured host socket has passed
// preflight and Engine capability probe. This does NOT grant Tool permissions;
// the outer Sandbox Router applies Owner/Policy/Lease before calling Open.
type Backend struct {
	inspector *Inspector
	sessions  SessionAPI
	templates TemplateLookup
}

func NewBackend(inspector *Inspector, templates TemplateLookup) (*Backend, error) {
	if inspector == nil || templates == nil {
		return nil, fmt.Errorf("docker Sandbox: Engine or Template store unavailable")
	}
	api, ok := inspector.api.(SessionAPI)
	if !ok {
		return nil, sandboxdomain.ErrBackendUnavailable
	}
	return &Backend{inspector: inspector, sessions: api, templates: templates}, nil
}
func (*Backend) Kind() sandboxdomain.BackendKind { return sandboxdomain.BackendDocker }
func (*Backend) Capabilities() sandboxdomain.Capabilities {
	return sandboxdomain.Capabilities{Read: true, Search: true, Edit: true, Shell: true}
}
func (b *Backend) Open(ctx context.Context, item sandboxdomain.Sandbox, run sandboxdomain.Run) (sandboxapplication.Session, error) {
	if b == nil || b.inspector == nil || b.sessions == nil || b.templates == nil {
		return nil, sandboxdomain.ErrBackendUnavailable
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if err := item.Validate(); err != nil {
		return nil, err
	}
	if item.Backend() != sandboxdomain.BackendDocker || item.Isolation() != sandboxdomain.IsolationContainer || run.SandboxID != item.ID() || run.State != sandboxdomain.StateReady || !containerIDPattern.MatchString(run.ExternalID) {
		return nil, sandboxdomain.ErrConflict
	}
	template, err := b.templates.GetDockerTemplate(ctx, item.ID())
	if err != nil {
		return nil, fmt.Errorf("open Docker Sandbox: trusted template unavailable: %w", sandboxdomain.ErrBackendUnavailable)
	}
	image, err := b.inspector.InspectLocalImage(ctx, template)
	if err != nil {
		return nil, err
	}
	staged, err := sandboxdomain.NewRun(item.ID(), run.Generation, sandboxdomain.StateProvisioning, "")
	if err != nil {
		return nil, err
	}
	plan, err := NewContainerPlan(item, staged, template, image)
	if err != nil {
		return nil, err
	}
	observed, err := b.sessions.ContainerInspect(ctx, run.ExternalID, mobyclient.ContainerInspectOptions{})
	if err != nil {
		return nil, safeDaemonError(ctx, err)
	}
	if observed.Container.ID != run.ExternalID {
		return nil, sandboxdomain.ErrConflict
	}
	if err := plan.VerifyInspect(observed.Container); err != nil {
		return nil, fmt.Errorf("open Docker Sandbox: security profile mismatch: %w", sandboxdomain.ErrBackendUnavailable)
	}
	if !observed.Container.State.Running {
		return nil, sandboxdomain.ErrConflict
	}
	return &workspaceSession{api: b.sessions, containerID: run.ExternalID, template: template}, nil
}

// workspaceSession never exposes daemon identity or Tool-selected mounts.
// Unknown command/file mutation status leaves the durable outer lease in place
// (Close fails), requiring review rather than an unsafe immediate re-acquire.
type workspaceSession struct {
	api         SessionAPI
	containerID string
	template    Template
	uncertain   atomic.Bool
}

func (s *workspaceSession) Close() error {
	if s == nil {
		return nil
	}
	if s.uncertain.Load() {
		return fmt.Errorf("docker Sandbox operation outcome uncertain; durable execution lease retained: %w", sandboxdomain.ErrConflict)
	}
	return nil
}
func (s *workspaceSession) ExecuteWorkspace(ctx context.Context, cap sandboxdomain.Capability, args invocationdomain.JSONValue) (invocationdomain.Outcome, error) {
	if s == nil || s.api == nil || !containerIDPattern.MatchString(s.containerID) {
		return invocationdomain.Outcome{}, sandboxdomain.ErrBackendUnavailable
	}
	if err := ctx.Err(); err != nil {
		return invocationdomain.Outcome{}, err
	}
	switch cap {
	case sandboxdomain.CapabilityRead:
		return s.read(ctx, args)
	case sandboxdomain.CapabilitySearch:
		return s.search(ctx, args)
	case sandboxdomain.CapabilityEdit:
		return s.edit(ctx, args)
	case sandboxdomain.CapabilityShell:
		return s.shell(ctx, args)
	default:
		return invocationdomain.Outcome{}, sandboxdomain.ErrCapabilityUnavailable
	}
}

var _ sandboxapplication.Backend = (*Backend)(nil)
var _ sandboxapplication.WorkspaceSession = (*workspaceSession)(nil)

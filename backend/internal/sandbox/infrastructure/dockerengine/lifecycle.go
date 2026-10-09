package dockerengine

import (
	"context"
	"errors"
	"fmt"

	mobyclient "github.com/moby/moby/client"
	governancedomain "github.com/orz-i/Mender/backend/internal/governance/domain"
	sandboxdomain "github.com/orz-i/Mender/backend/internal/sandbox/domain"
)

type dockerLifecycleStore interface {
	TransitionIfUnleased(context.Context, sandboxdomain.Run, sandboxdomain.State, string, bool) (sandboxdomain.Run, error)
	FinishDockerLifecycle(context.Context, sandboxdomain.Run, sandboxdomain.State, string, governancedomain.Action, governancedomain.Context) (sandboxdomain.Run, error)
}

// ChangeLifecycle admits exactly one Docker action before Engine I/O, verifies
// the immutable container identity, fences new leases in SQLite, and persists
// the observed completion with audit. Start provisions a NEW run generation:
// the tmpfs is intentionally ephemeral and the previous stopped container is
// removed only after an exact identity check; no host files are ever touched.
func (p *Provisioner) ChangeLifecycle(ctx context.Context, caller governancedomain.Context, id sandboxdomain.ID, action governancedomain.Action) (sandboxdomain.Run, error) {
	if p == nil || p.store == nil || p.templates == nil || p.daemon == nil || p.admit == nil || p.observer == nil {
		return sandboxdomain.Run{}, sandboxdomain.ErrBackendUnavailable
	}
	if err := caller.Validate(); err != nil {
		return sandboxdomain.Run{}, err
	}
	switch action {
	case governancedomain.ActionSandboxStart, governancedomain.ActionSandboxStop, governancedomain.ActionSandboxDestroy:
	default:
		return sandboxdomain.Run{}, sandboxdomain.ErrCapabilityUnavailable
	}
	item, err := p.store.Get(ctx, id)
	if err != nil {
		return sandboxdomain.Run{}, err
	}
	if item.Backend() != sandboxdomain.BackendDocker || item.Isolation() != sandboxdomain.IsolationContainer {
		return sandboxdomain.Run{}, sandboxdomain.ErrBackendUnavailable
	}
	principal := caller.Principal()
	if item.OwnerKind() != string(principal.Kind()) || item.OwnerRef() != principal.Reference() {
		return sandboxdomain.Run{}, sandboxdomain.ErrNotAuthorized
	}
	run, err := p.store.GetRun(ctx, id)
	if err != nil {
		return sandboxdomain.Run{}, err
	}
	switch action {
	case governancedomain.ActionSandboxStart:
		if run.State != sandboxdomain.StateStopped {
			return sandboxdomain.Run{}, sandboxdomain.ErrConflict
		}
	case governancedomain.ActionSandboxStop:
		if run.State != sandboxdomain.StateReady {
			return sandboxdomain.Run{}, sandboxdomain.ErrConflict
		}
	case governancedomain.ActionSandboxDestroy:
		if run.State != sandboxdomain.StateReady && run.State != sandboxdomain.StateStopped {
			return sandboxdomain.Run{}, sandboxdomain.ErrConflict
		}
	}
	if !containerIDPattern.MatchString(run.ExternalID) {
		return sandboxdomain.Run{}, sandboxdomain.ErrConflict
	}
	if err := p.admit.AdmitAction(ctx, caller, item, action); err != nil {
		return sandboxdomain.Run{}, err
	}
	lifecycle, ok := p.store.(dockerLifecycleStore)
	if !ok {
		return sandboxdomain.Run{}, sandboxdomain.ErrBackendUnavailable
	}
	template, err := p.templates.GetDockerTemplate(ctx, id)
	if err != nil {
		return sandboxdomain.Run{}, err
	}
	image, err := p.observer.InspectLocalImage(ctx, template)
	if err != nil {
		return sandboxdomain.Run{}, err
	}
	synthetic, err := sandboxdomain.NewRun(id, run.Generation, sandboxdomain.StateProvisioning, "")
	if err != nil {
		return sandboxdomain.Run{}, err
	}
	plan, err := NewContainerPlan(item, synthetic, template, image)
	if err != nil {
		return sandboxdomain.Run{}, err
	}
	observed, err := p.daemon.ContainerInspect(ctx, run.ExternalID, mobyclient.ContainerInspectOptions{})
	if err != nil {
		return sandboxdomain.Run{}, safeDaemonError(ctx, err)
	}
	if observed.Container.ID != run.ExternalID || plan.VerifyInspect(observed.Container) != nil {
		return sandboxdomain.Run{}, sandboxdomain.ErrBackendUnavailable
	}
	if action == governancedomain.ActionSandboxStart && observed.Container.State.Running {
		return sandboxdomain.Run{}, sandboxdomain.ErrConflict
	}
	var intermediate sandboxdomain.State
	switch action {
	case governancedomain.ActionSandboxStart:
		intermediate = sandboxdomain.StateProvisioning
	case governancedomain.ActionSandboxStop:
		intermediate = sandboxdomain.StateStopping
	default:
		intermediate = sandboxdomain.StateDestroying
	}
	fenced, err := lifecycle.TransitionIfUnleased(ctx, run, intermediate, run.ExternalID, action == governancedomain.ActionSandboxDestroy)
	if err != nil {
		return sandboxdomain.Run{}, err
	}
	if err := ctx.Err(); err != nil {
		return sandboxdomain.Run{}, errors.Join(err, p.makeUnknown(ctx, fenced, run.ExternalID))
	}
	switch action {
	case governancedomain.ActionSandboxStop:
		if observed.Container.State.Running {
			seconds := 5
			if _, err := p.daemon.ContainerStop(ctx, run.ExternalID, mobyclient.ContainerStopOptions{Timeout: &seconds}); err != nil {
				return sandboxdomain.Run{}, errors.Join(safeDaemonError(ctx, err), p.makeUnknown(ctx, fenced, run.ExternalID))
			}
		}
		inspected, err := p.daemon.ContainerInspect(ctx, run.ExternalID, mobyclient.ContainerInspectOptions{})
		if err != nil || inspected.Container.State == nil || inspected.Container.State.Running {
			return sandboxdomain.Run{}, errors.Join(sandboxdomain.ErrBackendUnavailable, p.makeUnknown(ctx, fenced, run.ExternalID))
		}
		next, err := lifecycle.FinishDockerLifecycle(ctx, fenced, sandboxdomain.StateStopped, run.ExternalID, action, caller)
		if err != nil {
			return sandboxdomain.Run{}, errors.Join(err, p.makeUnknown(ctx, fenced, run.ExternalID))
		}
		return next, nil
	case governancedomain.ActionSandboxDestroy:
		if observed.Container.State.Running {
			seconds := 5
			if _, err := p.daemon.ContainerStop(ctx, run.ExternalID, mobyclient.ContainerStopOptions{Timeout: &seconds}); err != nil {
				return sandboxdomain.Run{}, errors.Join(safeDaemonError(ctx, err), p.makeUnknown(ctx, fenced, run.ExternalID))
			}
		}
		if _, err := p.daemon.ContainerRemove(ctx, run.ExternalID, mobyclient.ContainerRemoveOptions{Force: false, RemoveVolumes: false}); err != nil {
			return sandboxdomain.Run{}, errors.Join(safeDaemonError(ctx, err), p.makeUnknown(ctx, fenced, run.ExternalID))
		}
		next, err := lifecycle.FinishDockerLifecycle(ctx, fenced, sandboxdomain.StateDestroyed, run.ExternalID, action, caller)
		if err != nil {
			return sandboxdomain.Run{}, errors.Join(err, p.makeUnknown(ctx, fenced, run.ExternalID))
		}
		return next, nil
	case governancedomain.ActionSandboxStart:
		// Start re-creates the verified stopped container under the next fencer
		// generation and fresh memory-backed Workspace. A removal uncertainty
		// retains unknown; it is never re-tried automatically.
		if _, err := p.daemon.ContainerRemove(ctx, run.ExternalID, mobyclient.ContainerRemoveOptions{Force: false, RemoveVolumes: false}); err != nil {
			return sandboxdomain.Run{}, errors.Join(safeDaemonError(ctx, err), p.makeUnknown(ctx, fenced, run.ExternalID))
		}
		return p.provisionStaged(ctx, caller, item, fenced, template, image, true)
	}
	return sandboxdomain.Run{}, fmt.Errorf("docker lifecycle unreachable action")
}

// RestoreUnknown reconciles a previously unknown Docker create/launch outcome.
// This never executes a second side-effecting Docker create.
func (p *Provisioner) RestoreUnknown(ctx context.Context, caller governancedomain.Context, id sandboxdomain.ID) (sandboxdomain.Run, error) {
	return p.Reconcile(ctx, caller, id)
}

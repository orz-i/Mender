package bootstrap

import (
	"context"

	gov "github.com/orz-i/Mender/backend/internal/governance/domain"
	sandboxapp "github.com/orz-i/Mender/backend/internal/sandbox/application"
	sandboxdomain "github.com/orz-i/Mender/backend/internal/sandbox/domain"
)

// Concrete adapters retain each backend's existing Policy admission,
// immutable templates, durable leases, and observed lifecycle state machine.
// The shared Registry only resolves implementations and cannot authorize them.
type localLifecycle struct{ sandboxes *Sandboxes }

func (c localLifecycle) Change(ctx context.Context, id sandboxdomain.ID, action gov.Action, caller gov.Context) (sandboxdomain.Run, error) {
	return c.sandboxes.ChangeLocalLifecycle(ctx, id, action, caller)
}
func (c localLifecycle) Reconcile(context.Context, sandboxdomain.ID, gov.Context) (sandboxdomain.Run, error) {
	return sandboxdomain.Run{}, sandboxdomain.ErrBackendUnavailable
}

type dockerLifecycle struct{ sandboxes *Sandboxes }

func (c dockerLifecycle) Change(ctx context.Context, id sandboxdomain.ID, action gov.Action, caller gov.Context) (sandboxdomain.Run, error) {
	if !c.sandboxes.DockerAvailable() {
		return sandboxdomain.Run{}, sandboxdomain.ErrBackendUnavailable
	}
	return c.sandboxes.docker.ChangeLifecycle(ctx, caller, id, action)
}
func (c dockerLifecycle) Reconcile(ctx context.Context, id sandboxdomain.ID, caller gov.Context) (sandboxdomain.Run, error) {
	if !c.sandboxes.DockerAvailable() {
		return sandboxdomain.Run{}, sandboxdomain.ErrBackendUnavailable
	}
	return c.sandboxes.docker.Reconcile(ctx, caller, id)
}

type daytonaLifecycle struct{ sandboxes *Sandboxes }

func (c daytonaLifecycle) Change(ctx context.Context, id sandboxdomain.ID, action gov.Action, caller gov.Context) (sandboxdomain.Run, error) {
	if !c.sandboxes.DaytonaConfigured() {
		return sandboxdomain.Run{}, sandboxdomain.ErrBackendUnavailable
	}
	return c.sandboxes.daytona.ChangeLifecycle(ctx, caller, id, action)
}
func (c daytonaLifecycle) Reconcile(ctx context.Context, id sandboxdomain.ID, caller gov.Context) (sandboxdomain.Run, error) {
	if !c.sandboxes.DaytonaConfigured() {
		return sandboxdomain.Run{}, sandboxdomain.ErrBackendUnavailable
	}
	return c.sandboxes.daytona.Reconcile(ctx, caller, id)
}

type externalLifecycle struct{ sandboxes *Sandboxes }

func (c externalLifecycle) Change(ctx context.Context, id sandboxdomain.ID, action gov.Action, caller gov.Context) (sandboxdomain.Run, error) {
	return c.sandboxes.ChangeExternalLifecycle(ctx, id, action, caller)
}
func (c externalLifecycle) Reconcile(ctx context.Context, id sandboxdomain.ID, caller gov.Context) (sandboxdomain.Run, error) {
	return c.sandboxes.ReconcileExternalSandbox(ctx, id, caller)
}

func (s *Sandboxes) registerBuiltinLifecycle(backends []sandboxapp.Backend) error {
	for _, backend := range backends {
		var controller sandboxapp.LifecycleController
		switch backend.Kind() {
		case sandboxdomain.BackendLocalWorkspace:
			controller = localLifecycle{sandboxes: s}
		case sandboxdomain.BackendDocker:
			controller = dockerLifecycle{sandboxes: s}
		case sandboxdomain.BackendDaytona:
			controller = daytonaLifecycle{sandboxes: s}
		default:
			return sandboxdomain.ErrBackendUnavailable
		}
		binding := sandboxdomain.BuiltinBinding(backend.Kind())
		if err := s.backendPlugins.RegisterLifecycle(backend.Kind(), binding, controller); err != nil {
			return err
		}
	}
	return nil
}

func (s *Sandboxes) Reconcile(ctx context.Context, id sandboxdomain.ID, caller gov.Context) (sandboxdomain.Run, error) {
	if s == nil || s.store == nil || s.backendPlugins == nil {
		return sandboxdomain.Run{}, sandboxdomain.ErrBackendUnavailable
	}
	item, err := s.store.Get(ctx, id)
	if err != nil {
		return sandboxdomain.Run{}, err
	}
	controller, err := s.backendPlugins.ResolveLifecycle(item.Backend(), item.Spec().Binding)
	if err != nil {
		return sandboxdomain.Run{}, err
	}
	return controller.Reconcile(ctx, id, caller)
}

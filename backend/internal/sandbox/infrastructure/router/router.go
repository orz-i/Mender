// Package router implements side-effect-free Sandbox placement and guarded,
// capability-checked acquisition without changing the Provider executor router.
package router

import (
	"context"
	"errors"
	"fmt"
	"time"

	governancedomain "github.com/orz-i/Mender/backend/internal/governance/domain"
	sandboxapp "github.com/orz-i/Mender/backend/internal/sandbox/application"
	sandboxdomain "github.com/orz-i/Mender/backend/internal/sandbox/domain"
)

type Router struct {
	definitions sandboxapp.SandboxLookup
	runs        sandboxapp.RunLookup
	gate        sandboxapp.AdmissionGate
	backends    map[sandboxdomain.BackendKind]sandboxapp.Backend
	leases      sandboxapp.ExecutionLeaseStore
}

func New(definitions sandboxapp.SandboxLookup, runs sandboxapp.RunLookup, gate sandboxapp.AdmissionGate, leases sandboxapp.ExecutionLeaseStore, backends ...sandboxapp.Backend) (*Router, error) {
	if definitions == nil || runs == nil || gate == nil || leases == nil {
		return nil, fmt.Errorf("sandbox router: definitions, runs, admission gate and durable leases required")
	}
	router := &Router{definitions: definitions, runs: runs, gate: gate, leases: leases, backends: make(map[sandboxdomain.BackendKind]sandboxapp.Backend)}
	for _, backend := range backends {
		if backend == nil {
			return nil, fmt.Errorf("sandbox router: nil backend")
		}
		kind := backend.Kind()
		if err := kind.Validate(); err != nil {
			return nil, err
		}
		if _, found := router.backends[kind]; found {
			return nil, fmt.Errorf("sandbox router: duplicate backend %q", kind)
		}
		router.backends[kind] = backend
	}
	return router, nil
}

// Resolve reads only persisted metadata. It cannot probe, launch or connect to
// a backend; callers may use this placement to evaluate policy before side effects.
func (r *Router) Resolve(ctx context.Context, id sandboxdomain.ID, required sandboxdomain.Capability) (sandboxapp.Placement, error) {
	if err := ctx.Err(); err != nil {
		return sandboxapp.Placement{}, err
	}
	if err := required.Validate(); err != nil {
		return sandboxapp.Placement{}, err
	}
	if _, err := sandboxdomain.ParseID(id.String()); err != nil {
		return sandboxapp.Placement{}, err
	}
	item, err := r.definitions.Get(ctx, id)
	if err != nil {
		return sandboxapp.Placement{}, err
	}
	run, err := r.runs.GetRun(ctx, id)
	if err != nil {
		return sandboxapp.Placement{}, err
	}
	if run.SandboxID != item.ID() {
		return sandboxapp.Placement{}, fmt.Errorf("sandbox router: run belongs to another sandbox")
	}
	if run.State == sandboxdomain.StateDestroying || run.State == sandboxdomain.StateDestroyed {
		return sandboxapp.Placement{}, sandboxdomain.ErrConflict
	}
	return sandboxapp.Placement{Sandbox: item, Run: run, Required: required}, nil
}

// Acquire only works after an AdmissionGate decides sandbox.acquire. It reads
// state again and compares identity and generation to reject stale placements.
// No backend substitution or weakening of the bound security class is allowed.
func (r *Router) Acquire(ctx context.Context, principal governancedomain.Context, placement sandboxapp.Placement) (*sandboxapp.Lease, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if err := principal.Validate(); err != nil {
		return nil, fmt.Errorf("sandbox router: principal: %w", err)
	}
	spec := placement.Sandbox.Spec()
	if err := placement.Sandbox.Validate(); err != nil {
		return nil, err
	}
	if err := placement.Required.Validate(); err != nil {
		return nil, err
	}
	// An owner is a security boundary. Delegations are a future explicit feature.
	subject := principal.Principal()
	if spec.OwnerKind != string(subject.Kind()) || spec.OwnerRef != subject.Reference() {
		return nil, sandboxdomain.ErrNotAuthorized
	}
	// Admit first, before any I/O to a backend or credential store.
	if err := r.gate.AdmitAcquire(ctx, principal, placement); err != nil {
		return nil, err
	}
	current, err := r.definitions.Get(ctx, placement.Sandbox.ID())
	if err != nil {
		return nil, err
	}
	if current.Spec() != spec {
		return nil, sandboxdomain.ErrConflict
	}
	run, err := r.runs.GetRun(ctx, placement.Sandbox.ID())
	if err != nil {
		return nil, err
	}
	if run != placement.Run || run.State != sandboxdomain.StateReady {
		return nil, sandboxdomain.ErrConflict
	}
	backend, ok := r.backends[spec.Backend]
	if !ok {
		return nil, sandboxdomain.ErrBackendUnavailable
	}
	if !backend.Capabilities().Supports(placement.Required) {
		return nil, sandboxdomain.ErrCapabilityUnavailable
	}
	if backend.Kind() != spec.Backend {
		return nil, sandboxdomain.ErrBackendUnavailable
	}
	// Reserve from the persistent store before touching a backend; all Runtime
	// processes sharing the same SQLite database compete for this one lease.
	token, err := r.leases.ReserveExecution(ctx, spec.ID, run.Generation, principal)
	if err != nil {
		return nil, fmt.Errorf("reserve Sandbox execution: %w", err)
	}
	releaseExecution := func() error {
		releaseCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 5*time.Second)
		defer cancel()
		return r.leases.ReleaseExecution(releaseCtx, spec.ID, token)
	}
	session, err := backend.Open(ctx, current, run)
	if err != nil {
		return nil, errors.Join(fmt.Errorf("acquire Sandbox: open backend: %w", err), releaseExecution())
	}
	if session == nil {
		return nil, errors.Join(fmt.Errorf("acquire Sandbox: backend returned no session"), releaseExecution())
	}
	if err := ctx.Err(); err != nil {
		return nil, errors.Join(err, session.Close(), releaseExecution())
	}
	return sandboxapp.NewLease(spec.ID, run.Generation, session, func() error {
		if err := session.Close(); err != nil {
			return fmt.Errorf("release Sandbox: backend cleanup failed, durable lease retained: %w", err)
		}
		return releaseExecution()
	}), nil
}

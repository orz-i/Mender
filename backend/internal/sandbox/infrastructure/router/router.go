// Package router implements side-effect-free Sandbox placement and guarded,
// capability-checked acquisition without changing the Provider executor router.
package router

import (
	"context"
	"fmt"
	"sync"

	governancedomain "github.com/orz-i/Mender/backend/internal/governance/domain"
	sandboxapp "github.com/orz-i/Mender/backend/internal/sandbox/application"
	sandboxdomain "github.com/orz-i/Mender/backend/internal/sandbox/domain"
)

const maxSandboxLeases = 1 // V1: prevent overlapping mutable local workspace sessions.

type Router struct {
	definitions sandboxapp.SandboxLookup
	runs        sandboxapp.RunLookup
	gate        sandboxapp.AdmissionGate
	backends    map[sandboxdomain.BackendKind]sandboxapp.Backend
	mu          sync.Mutex
	active      map[sandboxdomain.ID]int
}

func New(definitions sandboxapp.SandboxLookup, runs sandboxapp.RunLookup, gate sandboxapp.AdmissionGate, backends ...sandboxapp.Backend) (*Router, error) {
	if definitions == nil || runs == nil || gate == nil {
		return nil, fmt.Errorf("sandbox router: definitions, runs and admission gate required")
	}
	router := &Router{definitions: definitions, runs: runs, gate: gate, backends: make(map[sandboxdomain.BackendKind]sandboxapp.Backend), active: make(map[sandboxdomain.ID]int)}
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
	return sandboxapp.Placement{Sandbox: item, Run: run, Required: required}, nil
}

type Lease struct {
	SandboxID  sandboxdomain.ID
	Generation uint64
	Session    sandboxapp.Session
	release    func() error
	once       sync.Once
	releaseErr error
}

func (l *Lease) Close() error {
	if l == nil || l.release == nil {
		return nil
	}
	l.once.Do(func() { l.releaseErr = l.release() })
	return l.releaseErr
}

// Acquire only works after an AdmissionGate decides sandbox.acquire. It reads
// state again and compares identity and generation to reject stale placements.
// No backend substitution or weakening of the bound security class is allowed.
func (r *Router) Acquire(ctx context.Context, principal governancedomain.Context, placement sandboxapp.Placement) (*Lease, error) {
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
	r.mu.Lock()
	if r.active[spec.ID] >= maxSandboxLeases {
		r.mu.Unlock()
		return nil, sandboxdomain.ErrConflict
	}
	r.active[spec.ID]++
	r.mu.Unlock()
	releaseBudget := func() {
		r.mu.Lock()
		defer r.mu.Unlock()
		if r.active[spec.ID] <= 1 {
			delete(r.active, spec.ID)
		} else {
			r.active[spec.ID]--
		}
	}
	session, err := backend.Open(ctx, current, run)
	if err != nil {
		releaseBudget()
		return nil, fmt.Errorf("acquire sandbox: backend unavailable: %w", err)
	}
	if session == nil {
		releaseBudget()
		return nil, fmt.Errorf("acquire sandbox: backend returned no session")
	}
	if err := ctx.Err(); err != nil {
		_ = session.Close()
		releaseBudget()
		return nil, err
	}
	return &Lease{SandboxID: spec.ID, Generation: run.Generation, Session: session, release: func() error {
		defer releaseBudget()
		if err := session.Close(); err != nil {
			return fmt.Errorf("release sandbox session: %w", err)
		}
		return nil
	}}, nil
}

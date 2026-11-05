package application

import (
	"context"

	governancedomain "github.com/orz-i/Mender/backend/internal/governance/domain"
	sandboxdomain "github.com/orz-i/Mender/backend/internal/sandbox/domain"
)

// SandboxLookup is defined at the consuming application boundary.
type SandboxLookup interface {
	Get(context.Context, sandboxdomain.ID) (sandboxdomain.Sandbox, error)
}

// RunLookup reads persisted backend identity and observed readiness.
type RunLookup interface {
	GetRun(context.Context, sandboxdomain.ID) (sandboxdomain.Run, error)
}

// AdmissionGate authorizes an access *after* canonical Invocation has admitted
// tool.invoke. Without a gate no Sandbox can be acquired. A future production
// implementation must evaluate the explicit sandbox.acquire Policy action.
type AdmissionGate interface {
	AdmitAcquire(context.Context, governancedomain.Context, Placement) error
}

// Session is a backend-owned lifetime. Operation ports are defined by their
// consuming applications, independently of Tool JSON and Invocation outcomes.
type Session interface{ Close() error }

type Backend interface {
	Kind() sandboxdomain.BackendKind
	Capabilities() sandboxdomain.Capabilities
	Open(context.Context, sandboxdomain.Sandbox, sandboxdomain.Run) (Session, error)
}

// BackendResolver resolves only registered, host-reviewed typed Backend
// extensions after Sandbox admission. It must have no observable side effects.
// LifecycleController is the domain-owned control-plane port for an already
// configured Sandbox Backend. It never decides Policy or changes ownership.
type LifecycleController interface {
	Change(context.Context, sandboxdomain.ID, governancedomain.Action, governancedomain.Context) (sandboxdomain.Run, error)
	Reconcile(context.Context, sandboxdomain.ID, governancedomain.Context) (sandboxdomain.Run, error)
}

// LifecycleResolver routes an immutable Sandbox extension pin to its exact
// lifecycle implementation; no kind-only or host execution fallback.
type LifecycleResolver interface {
	ResolveLifecycle(sandboxdomain.BackendKind, sandboxdomain.BackendBinding) (LifecycleController, error)
}

type BackendResolver interface {
	ResolveBackend(sandboxdomain.BackendKind, sandboxdomain.BackendBinding, sandboxdomain.Capability) (Backend, error)
	Available(sandboxdomain.BackendKind, sandboxdomain.BackendBinding) bool
}

type Placement struct {
	Sandbox  sandboxdomain.Sandbox
	Run      sandboxdomain.Run
	Required sandboxdomain.Capability
}

// ExecutionLeaseStore serializes Sandbox operations across Runtime processes.
// One persisted lease is held per Sandbox until successful backend cleanup.
// A crash must NOT automatically expire an uncertain lease.
type ExecutionLeaseStore interface {
	ReserveExecution(context.Context, sandboxdomain.ID, uint64, governancedomain.Context, sandboxdomain.Capability) (string, error)
	ReleaseExecution(context.Context, sandboxdomain.ID, string) error
}

package application

import (
	"context"

	governancedomain "github.com/orz-i/Mender/backend/internal/governance/domain"
	invocationdomain "github.com/orz-i/Mender/backend/internal/invocation/domain"
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

// Session is a backend-owned execution context. Its File and Process ports
// will be added with concrete operations in the Workspace migration cluster.
type Session interface{ Close() error }

// WorkspaceSession executes an explicitly selected Workspace capability on
// the same backend filesystem, without permitting Tool arguments to choose a
// Sandbox or remount the host.
type WorkspaceSession interface {
	Session
	ExecuteWorkspace(context.Context, sandboxdomain.Capability, invocationdomain.JSONValue) (invocationdomain.Outcome, error)
}

type Backend interface {
	Kind() sandboxdomain.BackendKind
	Capabilities() sandboxdomain.Capabilities
	Open(context.Context, sandboxdomain.Sandbox, sandboxdomain.Run) (Session, error)
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
	ReserveExecution(context.Context, sandboxdomain.ID, uint64, governancedomain.Context) (string, error)
	ReleaseExecution(context.Context, sandboxdomain.ID, string) error
}

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

// Session is a backend-owned execution context. Its File and Process ports
// will be added with concrete operations in the Workspace migration cluster.
type Session interface{ Close() error }

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

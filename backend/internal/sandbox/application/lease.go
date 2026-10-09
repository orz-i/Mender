package application

import (
	"context"
	"sync"

	governancedomain "github.com/orz-i/Mender/backend/internal/governance/domain"
	sandboxdomain "github.com/orz-i/Mender/backend/internal/sandbox/domain"
)

// ExecutionRouter is the consumer-owned port used by Sandbox-compatible
// Provider tools. It is distinct from the Provider executor Router.
type ExecutionRouter interface {
	Resolve(context.Context, sandboxdomain.ID, sandboxdomain.Capability) (Placement, error)
	Acquire(context.Context, governancedomain.Context, Placement) (*Lease, error)
}

// Lease couples one admitted Sandbox run generation to its backend session.
// Release is idempotent and always returns the original close error.
type Lease struct {
	SandboxID  sandboxdomain.ID
	Generation uint64
	Session    Session
	once       sync.Once
	release    func() error
	releaseErr error
}

func NewLease(id sandboxdomain.ID, generation uint64, session Session, release func() error) *Lease {
	return &Lease{SandboxID: id, Generation: generation, Session: session, release: release}
}
func (l *Lease) Close() error {
	if l == nil || l.release == nil {
		return nil
	}
	l.once.Do(func() { l.releaseErr = l.release() })
	return l.releaseErr
}

package bootstrap

import (
	"context"
	"fmt"

	governanceapplication "github.com/orz-i/Mender/backend/internal/governance/application"
	sandboxapplication "github.com/orz-i/Mender/backend/internal/sandbox/application"
	sandboxdomain "github.com/orz-i/Mender/backend/internal/sandbox/domain"
	sandboxidentity "github.com/orz-i/Mender/backend/internal/sandbox/infrastructure/identity"
	sandboxrouter "github.com/orz-i/Mender/backend/internal/sandbox/infrastructure/router"
	sandboxsqlite "github.com/orz-i/Mender/backend/internal/sandbox/infrastructure/sqlite"
)

// Sandboxes owns the persistent Sandbox inventory and a fail-closed Execution
// Router. Provider/Tool execution is not redirected until the Workspace hard cut.
type Sandboxes struct {
	store  *sandboxsqlite.Store
	router *sandboxrouter.Router
}

func NewSandboxes(ctx context.Context, databasePath string, policy sandboxapplication.AdmissionPolicy, approval governanceapplication.ApprovalGate) (*Sandboxes, error) {
	if policy == nil || approval == nil {
		return nil, fmt.Errorf("compose sandboxes: policy/approval unavailable")
	}
	store, err := sandboxsqlite.Open(ctx, databasePath)
	if err != nil {
		return nil, fmt.Errorf("compose sandboxes: %w", err)
	}
	gate := sandboxapplication.NewAdmit(policy, approval, store, sandboxidentity.Generator{}, systemClock{})
	router, err := sandboxrouter.New(store, store, gate)
	if err != nil {
		_ = store.Close()
		return nil, fmt.Errorf("compose sandboxes: router: %w", err)
	}
	return &Sandboxes{store: store, router: router}, nil
}

// List returns inventory metadata, not a privilege to start a Sandbox.
func (s *Sandboxes) List(ctx context.Context) ([]sandboxdomain.Sandbox, error) {
	if s == nil || s.store == nil {
		return nil, fmt.Errorf("sandbox inventory unavailable")
	}
	return s.store.List(ctx)
}
func (s *Sandboxes) Get(ctx context.Context, id sandboxdomain.ID) (sandboxdomain.Sandbox, error) {
	if s == nil || s.store == nil {
		return sandboxdomain.Sandbox{}, fmt.Errorf("sandbox inventory unavailable")
	}
	return s.store.Get(ctx, id)
}
func (s *Sandboxes) Close() error {
	if s == nil || s.store == nil {
		return nil
	}
	return s.store.Close()
}

package typedlease

import (
	"context"

	governance "github.com/orz-i/Mender/backend/internal/governance/domain"
	sandboxapp "github.com/orz-i/Mender/backend/internal/sandbox/application"
	sandbox "github.com/orz-i/Mender/backend/internal/sandbox/domain"
)

// requestContextKey cannot be constructed by external callers or Tool inputs.
// The exact request is passed only across the existing Sandbox Router Acquire
// boundary. A matching native ToolID alone cannot reserve an execution lease.
type requestContextKey struct{}

// WithRequest pins the already reviewed/approved native operation to one
// attempt-scoped execution. The persisted lease check will revalidate grants,
// identity and run generation atomically, not trust the context as authority.
func WithRequest(ctx context.Context, req Request) (context.Context, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if err := req.Validate(); err != nil {
		return nil, err
	}
	return context.WithValue(ctx, requestContextKey{}, req), nil
}

func requestFromContext(ctx context.Context) (Request, error) {
	req, ok := ctx.Value(requestContextKey{}).(Request)
	if !ok || req.Validate() != nil {
		return Request{}, ErrUnadmitted
	}
	return req, nil
}

// ReserveExecution implements Sandbox ExecutionLeaseStore without a second
// lease ledger or backend. It is usable only with a matching reviewed native
// request; ordinary Provider or Agent calls cannot acquire from this store.
func (s *Store) ReserveExecution(ctx context.Context, id sandbox.ID, generation uint64,
	caller governance.Context, capability sandbox.Capability,
) (string, error) {
	req, err := requestFromContext(ctx)
	if err != nil {
		return "", err
	}
	_, required := operationCapability(req.Operation)
	if id != req.Sandbox || generation != req.Generation || caller != req.Caller ||
		string(capability) != required {
		return "", sandbox.ErrNotAuthorized
	}
	return s.Reserve(ctx, req)
}

// ReleaseExecution is the existing Router cleanup boundary. The original
// request context remains attached through context.WithoutCancel, so the
// same caller and Sandbox must release their own opaque durable lease token.
func (s *Store) ReleaseExecution(ctx context.Context, id sandbox.ID, token string) error {
	req, err := requestFromContext(ctx)
	if err != nil {
		return err
	}
	if req.Sandbox != id {
		return sandbox.ErrNotAuthorized
	}
	return s.Release(ctx, req.Caller, id, token)
}

var _ sandboxapp.ExecutionLeaseStore = (*Store)(nil)

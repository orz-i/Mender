package application

import (
	"context"
	"time"

	agentdomain "github.com/orz-i/Mender/backend/internal/agent/domain"
)

// pendingApprovalHandoff is a two-stage in-process reservation. The ACP
// requester receives the offer FIRST, but cannot return an ACP choice until
// the corresponding durable SQLite decision completes successfully.
type pendingApprovalHandoff struct {
	decision  PermissionDecision
	committed chan error // buffered: owner never waits on a dead child
}

// resolveWaitingApproval never records a new positive approval for an already
// absent, detached or concurrently claimed waiter. Only the owner/Policy path
// controls persist. No durable status claims external Agent receipt/obedience.
func (s *Service) resolveWaitingApproval(
	ctx context.Context,
	id agentdomain.ApprovalID,
	taskID agentdomain.TaskID,
	decision PermissionDecision,
	persist func() error,
) error {
	s.mu.Lock()
	waiter := s.pending[id]
	if s.closed || s.active[taskID] == nil || waiter == nil || waiter.reserved {
		s.mu.Unlock()
		return agentdomain.ErrConflict
	}
	select {
	case <-waiter.done:
		s.mu.Unlock()
		return agentdomain.ErrConflict
	default:
	}
	waiter.reserved = true
	s.mu.Unlock()
	defer func() {
		s.mu.Lock()
		waiter.reserved = false
		s.mu.Unlock()
	}()
	offer := pendingApprovalHandoff{decision: decision, committed: make(chan error, 1)}
	timer := time.NewTimer(2 * time.Second)
	defer timer.Stop()
	// An unbuffered offer can only be accepted by an actual live requester,
	// unlike the former buffered send after the SQLite commit.
	select {
	case waiter.offers <- offer:
	case <-waiter.done:
		return agentdomain.ErrConflict
	case <-ctx.Done():
		return ctx.Err()
	case <-timer.C:
		return agentdomain.ErrUnavailable
	}
	// Even after reservation, prevent a stale/crashed requester from being
	// treated as a newly accepted Policy action before the database write.
	if err := ctx.Err(); err != nil {
		offer.committed <- err
		return err
	}
	select {
	case <-waiter.done:
		offer.committed <- agentdomain.ErrConflict
		return agentdomain.ErrConflict
	default:
	}
	err := persist()
	offer.committed <- err
	return err
}

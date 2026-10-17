package application

import (
	"context"
	"errors"
	"sync/atomic"
	"time"

	agentdomain "github.com/orz-i/Mender/backend/internal/agent/domain"
)

// A request may withdraw its handoff before the owner starts persisting.
// Once persistence begins, the request always consumes its result, even if
// cancellation and the database commit become ready concurrently.
const (
	approvalOfferOpen int32 = iota
	approvalOfferWithdrawn
	approvalOfferCommitting
)

type pendingApprovalHandoff struct {
	decision  PermissionDecision
	committed chan error
	consumed  chan struct{}
	state     atomic.Int32
}

// waitForApprovalCommit is the only consumer of a handed-off decision.
// Returning the choice here proves an in-process result, NOT that an
// external ACP subprocess received or acted upon the JSON-RPC response.
func waitForApprovalCommit(ctx context.Context, deadline <-chan time.Time, offer *pendingApprovalHandoff) (PermissionDecision, bool, error) {
	var none PermissionDecision
	defer close(offer.consumed)
	finish := func(err error) (PermissionDecision, bool, error) {
		if errors.Is(err, agentdomain.ErrConflict) && ctx.Err() == nil {
			return none, true, nil // valid request returns to manual review
		}
		if err != nil {
			return none, false, err
		}
		return offer.decision, false, nil
	}
	for {
		select {
		case err := <-offer.committed:
			return finish(err)
		case <-ctx.Done():
			if offer.state.CompareAndSwap(approvalOfferOpen, approvalOfferWithdrawn) {
				return none, false, ctx.Err()
			}
			// If persistence has already started, do not discard its final
			// decision in a select racing ctx.Done with commit completion.
			return finish(<-offer.committed)
		case <-deadline:
			if offer.state.CompareAndSwap(approvalOfferOpen, approvalOfferWithdrawn) {
				return none, false, agentdomain.ErrUnavailable
			}
			return finish(<-offer.committed)
		}
	}
}

// resolveWaitingApproval reserves precisely one live ACP request. It does not
// persist anything until a request consumer has accepted the offer. Durable
// storage has an independent 2-second deadline. The owner observes completion
// only after the in-process consumer acknowledges the persistence result.
func (s *Service) resolveWaitingApproval(
	ctx context.Context,
	id agentdomain.ApprovalID,
	taskID agentdomain.TaskID,
	decision PermissionDecision,
	persist func(context.Context) error,
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

	offer := &pendingApprovalHandoff{
		decision: decision, committed: make(chan error, 1), consumed: make(chan struct{}),
	}
	timer := time.NewTimer(2 * time.Second)
	defer timer.Stop()
	select {
	case waiter.offers <- offer:
	case <-waiter.done:
		return agentdomain.ErrConflict
	case <-ctx.Done():
		return ctx.Err()
	case <-timer.C:
		return agentdomain.ErrUnavailable
	}
	// Offer delivery has completed; persistence and acknowledgement have
	// separate bounded deadlines, rather than sharing an already-fired timer.
	timer.Stop()
	var result error
	switch {
	case ctx.Err() != nil:
		result = ctx.Err()
	case !offer.state.CompareAndSwap(approvalOfferOpen, approvalOfferCommitting):
		result = agentdomain.ErrConflict
	default:
		persistCtx, cancel := context.WithTimeout(ctx, 2*time.Second)
		result = persist(persistCtx)
		cancel()
	}
	offer.committed <- result
	ackTimer := time.NewTimer(2 * time.Second)
	defer ackTimer.Stop()
	select {
	case <-offer.consumed:
		return result
	case <-ackTimer.C:
		// The owner cannot prove in-process consumption. Do not claim
		// successful handoff even if persistence has already committed.
		return agentdomain.ErrUnavailable
	}
}

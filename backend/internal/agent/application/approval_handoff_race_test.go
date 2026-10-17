package application

import (
	"context"
	"errors"
	"sync/atomic"
	"testing"
	"time"

	agentdomain "github.com/orz-i/Mender/backend/internal/agent/domain"
)

func TestACPCommitWinsConcurrentCancellationAndExpiredDeadline(t *testing.T) {
	// Both cancellation and deadline may be ready in a Go select on the
	// precise instruction where a durable decision becomes available. Once
	// persistence began, the request must observe that result, not randomly
	// discard it and leave an unconsumed allowed_once audit receipt.
	for i := 0; i < 128; i++ {
		ctx, cancel := context.WithCancel(context.Background())
		offer := &pendingApprovalHandoff{
			decision:  PermissionDecision{Choice: agentdomain.ApprovalAllowOnce},
			committed: make(chan error, 1), consumed: make(chan struct{}),
		}
		offer.state.Store(approvalOfferCommitting)
		offer.committed <- nil
		deadline := make(chan time.Time)
		close(deadline)
		cancel()
		got, retry, err := waitForApprovalCommit(ctx, deadline, offer)
		if err != nil || retry || got.Choice != agentdomain.ApprovalAllowOnce {
			t.Fatalf("committed result lost to concurrent cancellation (round=%d, got=%+v, retry=%t, err=%v)", i, got, retry, err)
		}
		select {
		case <-offer.consumed:
		default:
			t.Fatal("in-process receipt acknowledgement not completed")
		}
	}
}

func TestACPCancelAndTimeoutWithdrawBeforePersistence(t *testing.T) {
	for _, tc := range []struct {
		name     string
		cancel   bool
		expected error
	}{
		{name: "cancel", cancel: true, expected: context.Canceled},
		{name: "timeout", cancel: false, expected: agentdomain.ErrUnavailable},
	} {
		t.Run(tc.name, func(t *testing.T) {
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			deadline := make(chan time.Time)
			if tc.cancel {
				cancel()
			} else {
				close(deadline)
			}
			offer := &pendingApprovalHandoff{
				decision:  PermissionDecision{Choice: agentdomain.ApprovalAllowOnce},
				committed: make(chan error, 1), consumed: make(chan struct{}),
			}
			got, retry, err := waitForApprovalCommit(ctx, deadline, offer)
			if !errors.Is(err, tc.expected) || retry || got.Choice != "" {
				t.Fatalf("uncommitted request did not withdraw: %+v retry=%v err=%v", got, retry, err)
			}
			if offer.state.Load() != approvalOfferWithdrawn ||
				offer.state.CompareAndSwap(approvalOfferOpen, approvalOfferCommitting) {
				t.Fatal("a withdrawn ACP decision can still be persisted")
			}
			select {
			case <-offer.consumed:
			default:
				t.Fatal("withdrawal did not acknowledge request completion")
			}
		})
	}
}

func TestACPFailedPolicyCASReturnsToManualWaiter(t *testing.T) {
	offer := &pendingApprovalHandoff{
		decision:  PermissionDecision{Choice: agentdomain.ApprovalAllowOnce},
		committed: make(chan error, 1), consumed: make(chan struct{}),
	}
	offer.state.Store(approvalOfferCommitting)
	offer.committed <- agentdomain.ErrConflict
	got, retry, err := waitForApprovalCommit(context.Background(), make(chan time.Time), offer)
	if err != nil || !retry || got.Choice != "" {
		t.Fatalf("a rejected Policy generation was delivered to ACP: %+v retry=%v err=%v", got, retry, err)
	}
}

type handoffTestResult struct {
	decision PermissionDecision
	retry    bool
	err      error
}

func TestACPConcurrentCancellationAfterPersistenceStartsKeepsInProcessReceipt(t *testing.T) {
	s, _ := newFixtureService(t, fakeExecutor{
		started: make(chan struct{}), release: make(chan struct{}),
	})
	id, err := agentdomain.NewApprovalID()
	if err != nil {
		t.Fatal(err)
	}
	taskID, err := agentdomain.NewTaskID()
	if err != nil {
		t.Fatal(err)
	}
	waiter := &pendingApproval{
		offers: make(chan *pendingApprovalHandoff),
		done:   make(chan struct{}),
	}
	s.mu.Lock()
	s.active[taskID] = func() {}
	s.pending[id] = waiter
	s.mu.Unlock()

	requestCtx, cancelRequest := context.WithCancel(context.Background())
	defer cancelRequest()
	started := make(chan struct{})
	proceed := make(chan struct{})
	writes := atomic.Int32{}
	ownerDone := make(chan error, 1)
	go func() {
		ownerDone <- s.resolveWaitingApproval(
			context.Background(), id, taskID,
			PermissionDecision{Choice: agentdomain.ApprovalAllowOnce},
			func(context.Context) error {
				writes.Add(1)
				close(started)
				<-proceed
				return nil
			},
		)
	}()
	offer := <-waiter.offers
	requestDone := make(chan handoffTestResult, 1)
	go func() {
		d, retry, err := waitForApprovalCommit(requestCtx, make(chan time.Time), offer)
		requestDone <- handoffTestResult{d, retry, err}
	}()
	select {
	case <-started:
	case <-time.After(time.Second):
		t.Fatal("owner never reached persistent decision")
	}
	select {
	case <-ownerDone:
		t.Fatal("owner reported success before the request consumed the result")
	default:
	}
	cancelRequest()
	close(proceed)
	select {
	case result := <-requestDone:
		if result.err != nil || result.retry || result.decision.Choice != agentdomain.ApprovalAllowOnce {
			t.Fatalf("a committed one-shot decision was lost inside Mender: %+v", result)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("cancelled requester failed to consume commit result")
	}
	select {
	case err := <-ownerDone:
		if err != nil || writes.Load() != 1 {
			t.Fatalf("owner handoff failed: err=%v writes=%d", err, writes.Load())
		}
	case <-time.After(3 * time.Second):
		t.Fatal("owner did not observe requester acknowledgement")
	}
}

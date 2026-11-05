package application

import (
	"context"
	"sync"

	agentdomain "github.com/orz-i/Mender/backend/internal/agent/domain"
)

// runtimeCoordinator is the sole in-process owner of task/approval liveness
// and cancellation. Task admission, waiter attachment, handoff reservation and
// shutdown use this same mutex and the same maps; the façade stores no copy.
type runtimeCoordinator struct {
	runtimeInstance    string
	rootContext        context.Context
	rootCancel         context.CancelFunc
	toolSupplyResolver ToolSupplyResolver
	mu                 sync.Mutex
	active             map[agentdomain.TaskID]context.CancelFunc
	pending            map[agentdomain.ApprovalID]*pendingApproval
	closed             bool
	wg                 sync.WaitGroup
}

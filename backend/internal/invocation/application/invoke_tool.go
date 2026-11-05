package application

import (
	"context"
	"fmt"
	"time"

	catalogdomain "github.com/orz-i/Mender/backend/internal/catalog/domain"
	governanceapplication "github.com/orz-i/Mender/backend/internal/governance/application"
	governancedomain "github.com/orz-i/Mender/backend/internal/governance/domain"
	invocationdomain "github.com/orz-i/Mender/backend/internal/invocation/domain"
	runtimegovernancedomain "github.com/orz-i/Mender/backend/internal/runtimegovernance/domain"
)

const fallbackExecutionTimeout = 30 * time.Second

// InvokeToolRequest is the adapter-independent entry point for tool execution.
type InvokeToolRequest struct {
	ToolID    catalogdomain.ToolID
	Arguments []byte
	Context   governancedomain.Context
	Timeout   time.Duration
}

// InvokeToolResponse identifies the invocation and its canonical outcome.
type InvokeToolResponse struct {
	InvocationID invocationdomain.InvocationID
	Outcome      invocationdomain.Outcome
}

// InvokeTool owns the single ordered entry point into governed Tool execution.
// Its private collaborators separate preparation, admission, execution and
// terminal recording without exposing alternative execution paths to adapters.
type InvokeTool struct {
	preflight invocationPreflight
	admission invocationAdmission
	execution invocationExecution
	finalizer invocationFinalizer
	observer  InvocationObserver
	ids       InvocationIDGenerator
	clock     Clock
}

// invocationMetadata contains only stable, data-minimized attempt identity.
// Audit receives this projection, never the request's arguments or credentials.
type invocationMetadata struct {
	id        invocationdomain.InvocationID
	toolID    catalogdomain.ToolID
	context   governancedomain.Context
	startedAt time.Time
}

// invocationAttempt retains the one budget snapshot for the complete attempt.
// It is local to Execute and is never shared between concurrent invocations.
type invocationAttempt struct {
	metadata invocationMetadata
	budget   runtimegovernancedomain.Budget
	timeout  time.Duration
	events   *eventEmitter
}

// NewInvokeTool constructs the shared execution pipeline.
func NewInvokeTool(
	tools ToolLookup,
	profileScopes ProfileScopeResolver,
	arguments ArgumentValidator,
	facts GovernanceFactExtractor,
	policy PolicyEvaluator,
	approvals governanceapplication.ApprovalGate,
	runtimeBudget RuntimeBudgetController,
	bindingExecutionGuard BindingExecutionGuard,
	resultGuard ResultGuard,
	observer InvocationObserver,
	router ExecutorRouter,
	credentials CredentialResolver,
	audit AuditRecorder,
	ids InvocationIDGenerator,
	clock Clock,
	defaultTimeout time.Duration,
) InvokeTool {
	if defaultTimeout <= 0 {
		defaultTimeout = fallbackExecutionTimeout
	}
	return InvokeTool{
		preflight: invocationPreflight{tools: tools, profileScopes: profileScopes,
			arguments: arguments, facts: facts, runtimeBudget: runtimeBudget},
		admission: invocationAdmission{policy: policy, approvals: approvals, hasCredentials: credentials != nil},
		execution: invocationExecution{runtimeBudget: runtimeBudget, bindingGuard: bindingExecutionGuard,
			resultGuard: resultGuard, router: router, credentials: credentials, defaultTimeout: defaultTimeout},
		finalizer: invocationFinalizer{audit: invocationAuditWriter{recorder: audit, clock: clock}},
		observer:  observer, ids: ids, clock: clock,
	}
}

// Execute invokes one Tool through policy, approval, execution, and audit.
func (uc InvokeTool) Execute(ctx context.Context, request InvokeToolRequest) (response InvokeToolResponse, retErr error) {
	if request.Timeout < 0 {
		return InvokeToolResponse{}, ErrInvalidTimeout
	}
	if err := request.Context.Validate(); err != nil {
		return InvokeToolResponse{}, fmt.Errorf("%w: %w", ErrInvalidInvocationContext, err)
	}
	id, err := uc.ids.NewInvocationID()
	if err != nil {
		return InvokeToolResponse{}, fmt.Errorf("invoke tool: allocate id: %w", err)
	}
	attempt := invocationAttempt{metadata: invocationMetadata{
		id: id, toolID: request.ToolID, context: request.Context, startedAt: uc.clock.Now(),
	}, timeout: request.Timeout}
	budget, stop := uc.preflight.snapshotBudget(ctx, request.Arguments)
	if stop != nil {
		return uc.finalizer.finish(ctx, attempt, *stop)
	}
	attempt.budget = budget
	attempt.events = newEventEmitter(uc.observer, id, uc.clock)
	attempt.events.lifecycle(invocationdomain.EventAccepted)
	defer func() { uc.finalizer.observeTerminal(attempt.events, response, retErr) }()

	prepared, stop := uc.preflight.prepare(ctx, attempt, request.Arguments)
	if stop != nil {
		return uc.finalizer.finish(ctx, attempt, *stop)
	}
	admitted, stop := uc.admission.admit(ctx, attempt, prepared)
	if stop != nil {
		return uc.finalizer.finish(ctx, attempt, *stop)
	}
	return uc.execution.execute(ctx, attempt, admitted, uc.finalizer)
}

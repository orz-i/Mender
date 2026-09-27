package application

import (
	"context"
	"errors"
	"fmt"
	"time"

	auditdomain "github.com/orz-i/Mender/backend/internal/audit/domain"
	catalogapplication "github.com/orz-i/Mender/backend/internal/catalog/application"
	catalogdomain "github.com/orz-i/Mender/backend/internal/catalog/domain"
	invocationdomain "github.com/orz-i/Mender/backend/internal/invocation/domain"
	policydomain "github.com/orz-i/Mender/backend/internal/policy/domain"
)

const fallbackExecutionTimeout = 30 * time.Second

// InvokeToolRequest is the adapter-independent entry point for tool execution.
type InvokeToolRequest struct {
	ToolID    catalogdomain.ToolID
	Arguments []byte
	Context   invocationdomain.InvocationContext
	Timeout   time.Duration
}

// InvokeToolResponse identifies the invocation and its canonical outcome.
type InvokeToolResponse struct {
	InvocationID invocationdomain.InvocationID
	Outcome      invocationdomain.Outcome
}

// InvokeTool implements Mender's shared execution pipeline.
type InvokeTool struct {
	tools                 ToolLookup
	profileScopes         ProfileScopeResolver
	arguments             ArgumentValidator
	facts                 GovernanceFactExtractor
	policy                PolicyEvaluator
	approvals             ApprovalGate
	executionGuard        ExecutionGuard
	bindingExecutionGuard BindingExecutionGuard
	resultGuard           ResultGuard
	observer              InvocationObserver
	router                ExecutorRouter
	credentials           CredentialResolver
	audit                 AuditRecorder
	ids                   InvocationIDGenerator
	clock                 Clock
	defaultTimeout        time.Duration
}

// NewInvokeTool constructs the shared execution pipeline.
func NewInvokeTool(
	tools ToolLookup,
	profileScopes ProfileScopeResolver,
	arguments ArgumentValidator,
	facts GovernanceFactExtractor,
	policy PolicyEvaluator,
	approvals ApprovalGate,
	executionGuard ExecutionGuard,
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
		tools:                 tools,
		profileScopes:         profileScopes,
		arguments:             arguments,
		facts:                 facts,
		policy:                policy,
		approvals:             approvals,
		executionGuard:        executionGuard,
		bindingExecutionGuard: bindingExecutionGuard,
		resultGuard:           resultGuard,
		observer:              observer,
		router:                router,
		credentials:           credentials,
		audit:                 audit,
		ids:                   ids,
		clock:                 clock,
		defaultTimeout:        defaultTimeout,
	}
}

// Execute invokes one Tool through policy, approval, execution, and audit.
func (uc InvokeTool) Execute(
	ctx context.Context,
	request InvokeToolRequest,
) (response InvokeToolResponse, retErr error) {
	if request.Timeout < 0 {
		return InvokeToolResponse{}, ErrInvalidTimeout
	}
	if err := request.Context.Validate(); err != nil {
		return InvokeToolResponse{}, fmt.Errorf("%w: %w", ErrInvalidInvocationContext, err)
	}

	invocationID, err := uc.ids.NewInvocationID()
	if err != nil {
		return InvokeToolResponse{}, fmt.Errorf("invoke tool: allocate id: %w", err)
	}
	response = InvokeToolResponse{InvocationID: invocationID}
	startedAt := uc.clock.Now()
	emitter := newEventEmitter(uc.observer, invocationID, uc.clock)
	emitter.lifecycle(invocationdomain.EventAccepted)
	defer func() {
		if retErr == nil && response.Outcome.Kind() != "" {
			emitter.terminalOutcome(response.Outcome)
		}
	}()

	scopeGovernance, scopeOutcome, terminal, scopeErr := uc.enforceExecutionScope(
		ctx,
		request,
		invocationID,
		startedAt,
	)
	if terminal {
		response.Outcome = scopeOutcome
	}
	if scopeErr != nil {
		return response, scopeErr
	}
	emitter.lifecycle(invocationdomain.EventScopeResolved)
	if terminal {
		return response, nil
	}

	tool, err := uc.tools.GetTool(ctx, request.ToolID)
	if err != nil {
		if errors.Is(err, catalogapplication.ErrToolNotFound) {
			outcome, outcomeErr := newRejectedOutcome(
				invocationdomain.RejectionToolNotFound,
				"tool was not found",
			)
			if outcomeErr != nil {
				return response, outcomeErr
			}
			response.Outcome = outcome
			recordErr := uc.recordUnresolvedTool(
				context.WithoutCancel(ctx),
				invocationID,
				request.ToolID,
				request.Context,
				startedAt,
				auditdomain.StatusFailed,
				auditdomain.ErrorToolNotFound,
			)
			if recordErr != nil {
				return response, recordErr
			}
			return response, nil
		}

		recordErr := uc.recordUnresolvedTool(
			context.WithoutCancel(ctx),
			invocationID,
			request.ToolID,
			request.Context,
			startedAt,
			auditdomain.StatusFailed,
			auditdomain.ErrorInternal,
		)
		return response, joinInvocationError(
			fmt.Errorf("invoke tool: resolve tool: %w", err),
			recordErr,
		)
	}

	arguments, err := invocationdomain.NewJSONValue(request.Arguments)
	if err != nil {
		return uc.rejectInvalidArguments(ctx, response, tool, request, startedAt)
	}
	if uc.arguments == nil {
		return uc.failPreflight(
			ctx,
			response,
			tool,
			request,
			startedAt,
			"argument validator is unavailable",
			nil,
		)
	}
	valid, err := uc.arguments.Validate(ctx, tool.InputSchema(), arguments)
	if err != nil {
		return uc.failPreflight(
			ctx,
			response,
			tool,
			request,
			startedAt,
			"validate tool arguments",
			err,
		)
	}
	if !valid {
		return uc.rejectInvalidArguments(ctx, response, tool, request, startedAt)
	}
	emitter.lifecycle(invocationdomain.EventArgumentsValidated)

	if uc.facts == nil {
		return uc.failPreflight(
			ctx,
			response,
			tool,
			request,
			startedAt,
			"governance fact extractor is unavailable",
			nil,
		)
	}
	facts, err := uc.facts.Extract(ctx, tool, arguments)
	if err != nil {
		return uc.failPreflight(
			ctx,
			response,
			tool,
			request,
			startedAt,
			"derive governance facts",
			err,
		)
	}
	if err := facts.Validate(); err != nil {
		return uc.failPreflight(
			ctx,
			response,
			tool,
			request,
			startedAt,
			"validate governance facts",
			err,
		)
	}

	decision, err := uc.policy.Evaluate(ctx, PolicyRequest{
		InvocationID:          invocationID,
		Tool:                  tool,
		Context:               request.Context,
		Facts:                 facts,
		Caller:                request.Context.Caller(),
		HasCredentialResolver: uc.credentials != nil,
	})
	if err != nil {
		recordErr := uc.record(
			context.WithoutCancel(ctx),
			invocationID,
			tool,
			request.Context,
			startedAt,
			policydomain.DecisionKind(""),
			"",
			auditdomain.ApprovalNotRequired,
			auditdomain.StatusFailed,
			"",
			auditdomain.ErrorPolicy,
			"",
		)
		return response, joinInvocationError(
			fmt.Errorf("invoke tool: evaluate policy: %w", err),
			recordErr,
		)
	}
	decision, err = applyBindingPolicyConstraint(
		decision,
		scopeGovernance.PolicyConstraint(),
	)
	if err != nil {
		recordErr := uc.record(
			context.WithoutCancel(ctx),
			invocationID,
			tool,
			request.Context,
			startedAt,
			policydomain.DecisionKind(""),
			"",
			auditdomain.ApprovalNotRequired,
			auditdomain.StatusFailed,
			"",
			auditdomain.ErrorInternal,
			"",
		)
		return response, joinInvocationError(
			fmt.Errorf("invoke tool: apply binding policy constraint: %w", err),
			recordErr,
		)
	}
	emitter.lifecycle(invocationdomain.EventPolicyDecided)

	switch decision.Kind() {
	case policydomain.DecisionDeny:
		message := decision.Explanation()
		if message == "" {
			message = "invocation denied by policy"
		}
		outcome, outcomeErr := newRejectedOutcome(
			invocationdomain.RejectionPolicyDenied,
			message,
		)
		if outcomeErr != nil {
			return response, outcomeErr
		}
		response.Outcome = outcome
		recordErr := uc.record(
			context.WithoutCancel(ctx),
			invocationID,
			tool,
			request.Context,
			startedAt,
			decision.Kind(),
			decision.ReasonCode(),
			auditdomain.ApprovalNotRequired,
			auditdomain.StatusDenied,
			"",
			auditdomain.ErrorPolicy,
			"",
		)
		if recordErr != nil {
			return response, recordErr
		}
		return response, nil

	case policydomain.DecisionRequireApproval:
		approved, approvalErr := uc.approvals.Approve(ctx, ApprovalRequest{
			InvocationID: invocationID,
			Tool:         tool,
			Caller:       request.Context.Caller(),
			Decision:     decision,
		})
		if approvalErr != nil {
			recordErr := uc.record(
				ctx,
				invocationID,
				tool,
				request.Context,
				startedAt,
				decision.Kind(),
				decision.ReasonCode(),
				auditdomain.ApprovalDenied,
				auditdomain.StatusFailed,
				"",
				auditdomain.ErrorApproval,
				"",
			)
			return response, joinInvocationError(
				fmt.Errorf("invoke tool: approval: %w", approvalErr),
				recordErr,
			)
		}
		emitter.lifecycle(invocationdomain.EventApprovalResolved)
		if !approved {
			outcome, outcomeErr := newRejectedOutcome(
				invocationdomain.RejectionApprovalDenied,
				"required approval was denied",
			)
			if outcomeErr != nil {
				return response, outcomeErr
			}
			response.Outcome = outcome
			recordErr := uc.record(
				ctx,
				invocationID,
				tool,
				request.Context,
				startedAt,
				decision.Kind(),
				decision.ReasonCode(),
				auditdomain.ApprovalDenied,
				auditdomain.StatusDenied,
				"",
				auditdomain.ErrorApproval,
				"",
			)
			if recordErr != nil {
				return response, recordErr
			}
			return response, nil
		}

		return uc.execute(
			ctx,
			response,
			tool,
			arguments,
			request,
			startedAt,
			decision,
			auditdomain.ApprovalApproved,
			scopeGovernance,
			emitter,
		)

	case policydomain.DecisionAllow:
		return uc.execute(
			ctx,
			response,
			tool,
			arguments,
			request,
			startedAt,
			decision,
			auditdomain.ApprovalNotRequired,
			scopeGovernance,
			emitter,
		)

	default:
		recordErr := uc.record(
			context.WithoutCancel(ctx),
			invocationID,
			tool,
			request.Context,
			startedAt,
			decision.Kind(),
			decision.ReasonCode(),
			auditdomain.ApprovalNotRequired,
			auditdomain.StatusFailed,
			"",
			auditdomain.ErrorPolicy,
			"",
		)
		return response, joinInvocationError(
			fmt.Errorf("invoke tool: unknown policy decision %q", decision.Kind()),
			recordErr,
		)
	}
}

func (uc InvokeTool) rejectInvalidArguments(
	ctx context.Context,
	response InvokeToolResponse,
	tool catalogdomain.Tool,
	request InvokeToolRequest,
	startedAt time.Time,
) (InvokeToolResponse, error) {
	outcome, err := newRejectedOutcome(
		invocationdomain.RejectionInvalidArguments,
		"arguments do not match the tool input schema",
	)
	if err != nil {
		return response, err
	}
	response.Outcome = outcome
	recordErr := uc.record(
		context.WithoutCancel(ctx),
		response.InvocationID,
		tool,
		request.Context,
		startedAt,
		policydomain.DecisionKind(""),
		"",
		auditdomain.ApprovalNotRequired,
		auditdomain.StatusFailed,
		"",
		auditdomain.ErrorInvalidArguments,
		"",
	)
	if recordErr != nil {
		return response, recordErr
	}
	return response, nil
}

func (uc InvokeTool) failPreflight(
	ctx context.Context,
	response InvokeToolResponse,
	tool catalogdomain.Tool,
	request InvokeToolRequest,
	startedAt time.Time,
	stage string,
	cause error,
) (InvokeToolResponse, error) {
	recordErr := uc.record(
		context.WithoutCancel(ctx),
		response.InvocationID,
		tool,
		request.Context,
		startedAt,
		policydomain.DecisionKind(""),
		"",
		auditdomain.ApprovalNotRequired,
		auditdomain.StatusFailed,
		"",
		auditdomain.ErrorInternal,
		"",
	)

	var primary error
	if cause == nil {
		primary = fmt.Errorf("invoke tool: preflight: %s", stage)
	} else {
		primary = fmt.Errorf("invoke tool: preflight: %s: %w", stage, cause)
	}
	return response, joinInvocationError(primary, recordErr)
}

func (uc InvokeTool) enforceExecutionScope(
	ctx context.Context,
	request InvokeToolRequest,
	invocationID invocationdomain.InvocationID,
	startedAt time.Time,
) (
	invocationdomain.ScopedBindingGovernance,
	invocationdomain.Outcome,
	bool,
	error,
) {
	scope := request.Context.Scope()
	switch scope.Kind() {
	case invocationdomain.ScopeCatalog:
		return invocationdomain.InheritedScopedBindingGovernance(),
			invocationdomain.Outcome{},
			false,
			nil

	case invocationdomain.ScopeProfile:
		if uc.profileScopes == nil {
			recordErr := uc.recordUnresolvedTool(
				context.WithoutCancel(ctx),
				invocationID,
				request.ToolID,
				request.Context,
				startedAt,
				auditdomain.StatusFailed,
				auditdomain.ErrorInternal,
			)
			return invocationdomain.ScopedBindingGovernance{},
				invocationdomain.Outcome{},
				false,
				joinInvocationError(
					fmt.Errorf("invoke tool: resolve profile scope: resolver is unavailable"),
					recordErr,
				)
		}

		governance, found, err := uc.profileScopes.ResolveBindingGovernance(
			ctx,
			scope.Reference(),
			request.ToolID,
		)
		if err != nil {
			recordErr := uc.recordUnresolvedTool(
				context.WithoutCancel(ctx),
				invocationID,
				request.ToolID,
				request.Context,
				startedAt,
				auditdomain.StatusFailed,
				auditdomain.ErrorInternal,
			)
			return invocationdomain.ScopedBindingGovernance{},
				invocationdomain.Outcome{},
				false,
				joinInvocationError(
					fmt.Errorf("invoke tool: resolve profile scope: %w", err),
					recordErr,
				)
		}
		if !found {
			outcome, outcomeErr := newRejectedOutcome(
				invocationdomain.RejectionToolOutsideScope,
				"tool is not available in this execution scope",
			)
			if outcomeErr != nil {
				return invocationdomain.ScopedBindingGovernance{},
					invocationdomain.Outcome{},
					false,
					outcomeErr
			}
			recordErr := uc.recordUnresolvedTool(
				context.WithoutCancel(ctx),
				invocationID,
				request.ToolID,
				request.Context,
				startedAt,
				auditdomain.StatusDenied,
				auditdomain.ErrorPolicy,
				outcome,
			)
			return invocationdomain.ScopedBindingGovernance{},
				outcome,
				true,
				recordErr
		}
		if err := governance.Validate(); err != nil {
			recordErr := uc.recordUnresolvedTool(
				context.WithoutCancel(ctx),
				invocationID,
				request.ToolID,
				request.Context,
				startedAt,
				auditdomain.StatusFailed,
				auditdomain.ErrorInternal,
			)
			return invocationdomain.ScopedBindingGovernance{},
				invocationdomain.Outcome{},
				false,
				joinInvocationError(
					fmt.Errorf("invoke tool: invalid scoped binding governance: %w", err),
					recordErr,
				)
		}
		return governance, invocationdomain.Outcome{}, false, nil

	default:
		return invocationdomain.ScopedBindingGovernance{},
			invocationdomain.Outcome{},
			false,
			fmt.Errorf(
				"%w: unknown scope kind %q",
				ErrInvalidInvocationContext,
				scope.Kind(),
			)
	}
}

func (uc InvokeTool) execute(
	parent context.Context,
	response InvokeToolResponse,
	tool catalogdomain.Tool,
	arguments invocationdomain.JSONValue,
	request InvokeToolRequest,
	startedAt time.Time,
	decision policydomain.Decision,
	approval auditdomain.ApprovalOutcome,
	scopeGovernance invocationdomain.ScopedBindingGovernance,
	emitter *eventEmitter,
) (InvokeToolResponse, error) {
	timeout := request.Timeout
	if timeout == 0 {
		timeout = uc.defaultTimeout
	}
	execCtx, cancel := context.WithTimeout(parent, timeout)
	defer cancel()

	emitter.lifecycle(invocationdomain.EventExecutionWaiting)
	if uc.executionGuard == nil {
		recordErr := uc.record(
			context.WithoutCancel(parent),
			response.InvocationID,
			tool,
			request.Context,
			startedAt,
			decision.Kind(),
			decision.ReasonCode(),
			approval,
			auditdomain.StatusFailed,
			"",
			auditdomain.ErrorInternal,
			"",
		)
		return response, joinInvocationError(
			fmt.Errorf("invoke tool: execution guard is unavailable"),
			recordErr,
		)
	}

	baselineRelease, err := uc.executionGuard.Acquire(execCtx, ExecutionGuardRequest{
		InvocationID: response.InvocationID,
		Tool:         tool,
		Context:      request.Context,
	})
	if err != nil {
		if isExecutionContextError(execCtx, err) {
			return uc.returnExecutionFailure(
				parent,
				response,
				tool,
				request,
				startedAt,
				decision,
				approval,
				execCtx,
				err,
			)
		}

		recordErr := uc.record(
			context.WithoutCancel(parent),
			response.InvocationID,
			tool,
			request.Context,
			startedAt,
			decision.Kind(),
			decision.ReasonCode(),
			approval,
			auditdomain.StatusFailed,
			"",
			auditdomain.ErrorInternal,
			"",
		)
		return response, joinInvocationError(
			fmt.Errorf("invoke tool: acquire execution guard: %w", err),
			recordErr,
		)
	}
	if baselineRelease == nil {
		recordErr := uc.record(
			context.WithoutCancel(parent),
			response.InvocationID,
			tool,
			request.Context,
			startedAt,
			decision.Kind(),
			decision.ReasonCode(),
			approval,
			auditdomain.StatusFailed,
			"",
			auditdomain.ErrorInternal,
			"",
		)
		return response, joinInvocationError(
			fmt.Errorf("invoke tool: execution guard returned no release function"),
			recordErr,
		)
	}

	if uc.bindingExecutionGuard == nil {
		baselineRelease()
		recordErr := uc.record(
			context.WithoutCancel(parent),
			response.InvocationID,
			tool,
			request.Context,
			startedAt,
			decision.Kind(),
			decision.ReasonCode(),
			approval,
			auditdomain.StatusFailed,
			"",
			auditdomain.ErrorInternal,
			"",
		)
		return response, joinInvocationError(
			fmt.Errorf("invoke tool: binding execution guard is unavailable"),
			recordErr,
		)
	}

	bindingRelease, err := uc.bindingExecutionGuard.Acquire(
		execCtx,
		BindingExecutionGuardRequest{
			InvocationID: response.InvocationID,
			Tool:         tool,
			Context:      request.Context,
			Governance:   scopeGovernance,
		},
	)
	if err != nil {
		baselineRelease()
		if isExecutionContextError(execCtx, err) {
			return uc.returnExecutionFailure(
				parent,
				response,
				tool,
				request,
				startedAt,
				decision,
				approval,
				execCtx,
				err,
			)
		}
		recordErr := uc.record(
			context.WithoutCancel(parent),
			response.InvocationID,
			tool,
			request.Context,
			startedAt,
			decision.Kind(),
			decision.ReasonCode(),
			approval,
			auditdomain.StatusFailed,
			"",
			auditdomain.ErrorInternal,
			"",
		)
		return response, joinInvocationError(
			fmt.Errorf("invoke tool: acquire binding execution guard: %w", err),
			recordErr,
		)
	}
	if bindingRelease == nil {
		baselineRelease()
		recordErr := uc.record(
			context.WithoutCancel(parent),
			response.InvocationID,
			tool,
			request.Context,
			startedAt,
			decision.Kind(),
			decision.ReasonCode(),
			approval,
			auditdomain.StatusFailed,
			"",
			auditdomain.ErrorInternal,
			"",
		)
		return response, joinInvocationError(
			fmt.Errorf("invoke tool: binding execution guard returned no release function"),
			recordErr,
		)
	}

	release := func() {
		bindingRelease()
		baselineRelease()
	}
	executor, err := uc.router.ExecutorFor(execCtx, tool)
	if err != nil {
		release()
		outcome, outcomeErr := newFailedOutcome(
			invocationdomain.FailureSourceRuntime,
			invocationdomain.FailureExecutorUnavailable,
			"tool executor is unavailable",
		)
		if outcomeErr != nil {
			return response, outcomeErr
		}
		response.Outcome = outcome
		recordErr := uc.record(
			context.WithoutCancel(parent),
			response.InvocationID,
			tool,
			request.Context,
			startedAt,
			decision.Kind(),
			decision.ReasonCode(),
			approval,
			auditdomain.StatusFailed,
			"",
			auditdomain.ErrorExecutorNotFound,
			"",
		)
		if recordErr != nil {
			return response, recordErr
		}
		return response, nil
	}
	emitter.lifecycle(invocationdomain.EventExecutionStarted)

	outcome, err := executeWithGuardRelease(
		execCtx,
		executor,
		ExecutionRequest{
			InvocationID: response.InvocationID,
			Tool:         tool,
			Arguments:    arguments,
		},
		uc.credentials,
		emitter,
		release,
	)
	if err != nil {
		return uc.returnExecutionFailure(
			parent,
			response,
			tool,
			request,
			startedAt,
			decision,
			approval,
			execCtx,
			err,
		)
	}

	if uc.resultGuard == nil {
		recordErr := uc.record(
			context.WithoutCancel(parent),
			response.InvocationID,
			tool,
			request.Context,
			startedAt,
			decision.Kind(),
			decision.ReasonCode(),
			approval,
			auditdomain.StatusFailed,
			"",
			auditdomain.ErrorInternal,
			"",
		)
		return response, joinInvocationError(
			fmt.Errorf("invoke tool: result guard is unavailable"),
			recordErr,
		)
	}

	guardedOutcome, err := uc.resultGuard.Guard(execCtx, tool, outcome)
	if err != nil {
		if isExecutionContextError(execCtx, err) {
			return uc.returnExecutionFailure(
				parent,
				response,
				tool,
				request,
				startedAt,
				decision,
				approval,
				execCtx,
				err,
			)
		}

		recordErr := uc.record(
			context.WithoutCancel(parent),
			response.InvocationID,
			tool,
			request.Context,
			startedAt,
			decision.Kind(),
			decision.ReasonCode(),
			approval,
			auditdomain.StatusFailed,
			"",
			auditdomain.ErrorInternal,
			"",
		)
		return response, joinInvocationError(
			fmt.Errorf("invoke tool: result guard: %w", err),
			recordErr,
		)
	}
	outcome = guardedOutcome

	status, resultKind, class, ok := auditClassificationForExecutorOutcome(outcome)
	if !ok {
		recordErr := uc.record(
			context.WithoutCancel(parent),
			response.InvocationID,
			tool,
			request.Context,
			startedAt,
			decision.Kind(),
			decision.ReasonCode(),
			approval,
			auditdomain.StatusFailed,
			"",
			auditdomain.ErrorExecution,
			"",
		)
		return response, joinInvocationError(ErrInvalidExecutorResult, recordErr)
	}

	var summary string
	if outcome.Kind() == invocationdomain.OutcomeKindFailed {
		summary = redactDiagnosticSummary(outcome.Message())
	}
	recordErr := uc.record(
		context.WithoutCancel(parent),
		response.InvocationID,
		tool,
		request.Context,
		startedAt,
		decision.Kind(),
		decision.ReasonCode(),
		approval,
		status,
		resultKind,
		class,
		summary,
		outcome,
	)
	if recordErr != nil {
		return response, recordErr
	}

	response.Outcome = outcome
	return response, nil
}

func executeWithGuardRelease(
	ctx context.Context,
	executor ToolExecutor,
	request ExecutionRequest,
	credentials CredentialResolver,
	progress ProgressReporter,
	release func(),
) (invocationdomain.Outcome, error) {
	defer release()
	return executor.Execute(ctx, request, credentials, progress)
}

func (uc InvokeTool) returnExecutionFailure(
	parent context.Context,
	response InvokeToolResponse,
	tool catalogdomain.Tool,
	request InvokeToolRequest,
	startedAt time.Time,
	decision policydomain.Decision,
	approval auditdomain.ApprovalOutcome,
	execCtx context.Context,
	err error,
) (InvokeToolResponse, error) {
	failed, status, class, outcomeErr := failedOutcomeForExecutionError(execCtx, err)
	if outcomeErr != nil {
		return response, outcomeErr
	}
	response.Outcome = failed
	summary := auditErrorSummary(err)
	recordErr := uc.record(
		context.WithoutCancel(parent),
		response.InvocationID,
		tool,
		request.Context,
		startedAt,
		decision.Kind(),
		decision.ReasonCode(),
		approval,
		status,
		"",
		class,
		summary,
		failed,
	)
	if recordErr != nil {
		return response, recordErr
	}
	return response, nil
}

func isExecutionContextError(ctx context.Context, err error) bool {
	return errors.Is(err, context.Canceled) ||
		errors.Is(err, context.DeadlineExceeded) ||
		errors.Is(ctx.Err(), context.Canceled) ||
		errors.Is(ctx.Err(), context.DeadlineExceeded)
}

func newRejectedOutcome(
	code invocationdomain.RejectionCode,
	message string,
) (invocationdomain.Outcome, error) {
	outcome, err := invocationdomain.Rejected(code, message)
	if err != nil {
		return invocationdomain.Outcome{}, fmt.Errorf("invoke tool: construct rejection outcome: %w", err)
	}
	return outcome, nil
}

func newFailedOutcome(
	source invocationdomain.FailureSource,
	code invocationdomain.FailureCode,
	message string,
) (invocationdomain.Outcome, error) {
	outcome, err := invocationdomain.Failed(source, code, message)
	if err != nil {
		return invocationdomain.Outcome{}, fmt.Errorf("invoke tool: construct failure outcome: %w", err)
	}
	return outcome, nil
}

func failedOutcomeForExecutionError(
	ctx context.Context,
	err error,
) (invocationdomain.Outcome, auditdomain.Status, auditdomain.ErrorClass, error) {
	status, class := classifyExecutionError(ctx, err)
	source := invocationdomain.FailureSourceProvider
	code := invocationdomain.FailureProviderError
	message := "provider execution failed"

	switch status {
	case auditdomain.StatusTimedOut:
		source = invocationdomain.FailureSourceRuntime
		code = invocationdomain.FailureTimedOut
		message = "tool execution timed out"
	case auditdomain.StatusCancelled:
		source = invocationdomain.FailureSourceRuntime
		code = invocationdomain.FailureCancelled
		message = "tool execution was cancelled"
	default:
		if summary := auditErrorSummary(err); summary != "" {
			message = summary
		}
	}

	outcome, outcomeErr := newFailedOutcome(source, code, message)
	return outcome, status, class, outcomeErr
}

func auditClassificationForExecutorOutcome(
	outcome invocationdomain.Outcome,
) (auditdomain.Status, invocationdomain.ResultKind, auditdomain.ErrorClass, bool) {
	switch outcome.Kind() {
	case invocationdomain.OutcomeKindComplete:
		return auditdomain.StatusCompleted, invocationdomain.ResultKindComplete, auditdomain.ErrorNone, true
	case invocationdomain.OutcomeKindInputRequired:
		return auditdomain.StatusInputRequired, invocationdomain.ResultKindInputRequired, auditdomain.ErrorNone, true
	case invocationdomain.OutcomeKindTask:
		return auditdomain.StatusTask, invocationdomain.ResultKindTask, auditdomain.ErrorNone, true
	case invocationdomain.OutcomeKindFailed:
		switch outcome.FailureSource() {
		case invocationdomain.FailureSourceTool, invocationdomain.FailureSourceProvider:
			return auditdomain.StatusFailed, "", auditdomain.ErrorExecution, true
		case invocationdomain.FailureSourceRuntime:
			switch outcome.FailureCode() {
			case invocationdomain.FailureExecutorUnavailable:
				return auditdomain.StatusFailed, "", auditdomain.ErrorExecutorNotFound, true
			case invocationdomain.FailureTimedOut:
				return auditdomain.StatusTimedOut, "", auditdomain.ErrorTimedOut, true
			case invocationdomain.FailureCancelled:
				return auditdomain.StatusCancelled, "", auditdomain.ErrorCancelled, true
			default:
				return "", "", "", false
			}
		case invocationdomain.FailureSourceResultGuard:
			return auditdomain.StatusFailed, "", auditdomain.ErrorResultGuard, true
		default:
			return "", "", "", false
		}
	default:
		return "", "", "", false
	}
}

func (uc InvokeTool) record(
	ctx context.Context,
	invocationID invocationdomain.InvocationID,
	tool catalogdomain.Tool,
	executionContext invocationdomain.InvocationContext,
	startedAt time.Time,
	policyKind policydomain.DecisionKind,
	policyReason string,
	approval auditdomain.ApprovalOutcome,
	status auditdomain.Status,
	resultKind invocationdomain.ResultKind,
	errorClass auditdomain.ErrorClass,
	errorSummary string,
	outcomes ...invocationdomain.Outcome,
) error {
	providerID := tool.ProviderID()
	record, err := auditdomain.NewRecord(
		invocationID,
		tool.ID(),
		&providerID,
		executionContext.Caller(),
		startedAt,
		uc.clock.Now(),
		policyKind,
		policyReason,
		approval,
		status,
		resultKind,
		errorClass,
	)
	if err != nil {
		return fmt.Errorf("%w: construct: %w", ErrAuditFailed, err)
	}
	record, err = record.WithExecutionContext(executionContext)
	if err != nil {
		return fmt.Errorf("%w: execution metadata: %w", ErrAuditFailed, err)
	}
	if len(outcomes) > 1 {
		return fmt.Errorf("%w: multiple canonical outcomes supplied", ErrAuditFailed)
	}
	if len(outcomes) == 1 {
		record, err = record.WithOutcome(outcomes[0])
	} else {
		record, err = attachInferredAuditOutcome(record, status, resultKind, errorClass)
	}
	if err != nil {
		return fmt.Errorf("%w: outcome metadata: %w", ErrAuditFailed, err)
	}
	if errorSummary != "" {
		record = record.WithErrorSummary(errorSummary)
	}
	if err := uc.audit.Record(ctx, record); err != nil {
		return fmt.Errorf("%w: persist: %w", ErrAuditFailed, err)
	}
	return nil
}

func (uc InvokeTool) recordUnresolvedTool(
	ctx context.Context,
	invocationID invocationdomain.InvocationID,
	toolID catalogdomain.ToolID,
	executionContext invocationdomain.InvocationContext,
	startedAt time.Time,
	status auditdomain.Status,
	errorClass auditdomain.ErrorClass,
	outcomes ...invocationdomain.Outcome,
) error {
	record, err := auditdomain.NewRecord(
		invocationID,
		toolID,
		nil,
		executionContext.Caller(),
		startedAt,
		uc.clock.Now(),
		policydomain.DecisionKind(""),
		"",
		auditdomain.ApprovalNotRequired,
		status,
		"",
		errorClass,
	)
	if err != nil {
		return fmt.Errorf("%w: construct unresolved tool record: %w", ErrAuditFailed, err)
	}
	record, err = record.WithExecutionContext(executionContext)
	if err != nil {
		return fmt.Errorf("%w: unresolved execution metadata: %w", ErrAuditFailed, err)
	}
	if len(outcomes) > 1 {
		return fmt.Errorf("%w: multiple unresolved canonical outcomes supplied", ErrAuditFailed)
	}
	if len(outcomes) == 1 {
		record, err = record.WithOutcome(outcomes[0])
	} else {
		record, err = attachInferredUnresolvedOutcome(record, status, errorClass)
	}
	if err != nil {
		return fmt.Errorf("%w: unresolved outcome metadata: %w", ErrAuditFailed, err)
	}
	if err := uc.audit.Record(ctx, record); err != nil {
		return fmt.Errorf("%w: persist unresolved tool record: %w", ErrAuditFailed, err)
	}
	return nil
}

func attachInferredAuditOutcome(
	record auditdomain.Record,
	status auditdomain.Status,
	resultKind invocationdomain.ResultKind,
	errorClass auditdomain.ErrorClass,
) (auditdomain.Record, error) {
	switch resultKind {
	case invocationdomain.ResultKindComplete:
		return record.WithOutcomeClassification(invocationdomain.OutcomeKindComplete, "", "", "")
	case invocationdomain.ResultKindInputRequired:
		return record.WithOutcomeClassification(invocationdomain.OutcomeKindInputRequired, "", "", "")
	case invocationdomain.ResultKindTask:
		return record.WithOutcomeClassification(invocationdomain.OutcomeKindTask, "", "", "")
	}

	switch errorClass {
	case auditdomain.ErrorToolNotFound:
		return record.WithOutcomeClassification(
			invocationdomain.OutcomeKindRejected,
			invocationdomain.RejectionToolNotFound,
			"",
			"",
		)
	case auditdomain.ErrorInvalidArguments:
		return record.WithOutcomeClassification(
			invocationdomain.OutcomeKindRejected,
			invocationdomain.RejectionInvalidArguments,
			"",
			"",
		)
	case auditdomain.ErrorPolicy:
		if status == auditdomain.StatusDenied {
			return record.WithOutcomeClassification(
				invocationdomain.OutcomeKindRejected,
				invocationdomain.RejectionPolicyDenied,
				"",
				"",
			)
		}
	case auditdomain.ErrorApproval:
		if status == auditdomain.StatusDenied {
			return record.WithOutcomeClassification(
				invocationdomain.OutcomeKindRejected,
				invocationdomain.RejectionApprovalDenied,
				"",
				"",
			)
		}
	case auditdomain.ErrorExecutorNotFound:
		return record.WithOutcomeClassification(
			invocationdomain.OutcomeKindFailed,
			"",
			invocationdomain.FailureSourceRuntime,
			invocationdomain.FailureExecutorUnavailable,
		)
	case auditdomain.ErrorTimedOut:
		return record.WithOutcomeClassification(
			invocationdomain.OutcomeKindFailed,
			"",
			invocationdomain.FailureSourceRuntime,
			invocationdomain.FailureTimedOut,
		)
	case auditdomain.ErrorCancelled:
		return record.WithOutcomeClassification(
			invocationdomain.OutcomeKindFailed,
			"",
			invocationdomain.FailureSourceRuntime,
			invocationdomain.FailureCancelled,
		)
	}
	return record, nil
}

func attachInferredUnresolvedOutcome(
	record auditdomain.Record,
	status auditdomain.Status,
	errorClass auditdomain.ErrorClass,
) (auditdomain.Record, error) {
	switch {
	case errorClass == auditdomain.ErrorToolNotFound:
		return record.WithOutcomeClassification(
			invocationdomain.OutcomeKindRejected,
			invocationdomain.RejectionToolNotFound,
			"",
			"",
		)
	case errorClass == auditdomain.ErrorPolicy && status == auditdomain.StatusDenied:
		return record.WithOutcomeClassification(
			invocationdomain.OutcomeKindRejected,
			invocationdomain.RejectionToolOutsideScope,
			"",
			"",
		)
	default:
		return record, nil
	}
}

func classifyExecutionError(
	ctx context.Context,
	err error,
) (auditdomain.Status, auditdomain.ErrorClass) {
	switch {
	case errors.Is(err, context.DeadlineExceeded), errors.Is(ctx.Err(), context.DeadlineExceeded):
		return auditdomain.StatusTimedOut, auditdomain.ErrorTimedOut
	case errors.Is(err, context.Canceled), errors.Is(ctx.Err(), context.Canceled):
		return auditdomain.StatusCancelled, auditdomain.ErrorCancelled
	default:
		return auditdomain.StatusFailed, auditdomain.ErrorExecution
	}
}

func joinInvocationError(primary, auditErr error) error {
	if auditErr == nil {
		return primary
	}
	return errors.Join(primary, auditErr)
}

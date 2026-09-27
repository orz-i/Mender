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
	tools          ToolLookup
	profileScopes  ProfileScopeAuthorizer
	policy         PolicyEvaluator
	approvals      ApprovalGate
	router         ExecutorRouter
	credentials    CredentialResolver
	audit          AuditRecorder
	ids            InvocationIDGenerator
	clock          Clock
	defaultTimeout time.Duration
}

// NewInvokeTool constructs the shared execution pipeline.
func NewInvokeTool(
	tools ToolLookup,
	profileScopes ProfileScopeAuthorizer,
	policy PolicyEvaluator,
	approvals ApprovalGate,
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
		tools:          tools,
		profileScopes:  profileScopes,
		policy:         policy,
		approvals:      approvals,
		router:         router,
		credentials:    credentials,
		audit:          audit,
		ids:            ids,
		clock:          clock,
		defaultTimeout: defaultTimeout,
	}
}

// Execute invokes one Tool through policy, approval, execution, and audit.
func (uc InvokeTool) Execute(
	ctx context.Context,
	request InvokeToolRequest,
) (InvokeToolResponse, error) {
	if request.Timeout < 0 {
		return InvokeToolResponse{}, ErrInvalidTimeout
	}
	if err := request.Context.Validate(); err != nil {
		return InvokeToolResponse{}, fmt.Errorf("%w: %w", ErrInvalidInvocationContext, err)
	}

	arguments, err := invocationdomain.NewJSONValue(request.Arguments)
	if err != nil {
		return InvokeToolResponse{}, fmt.Errorf("invoke tool: arguments: %w", err)
	}

	invocationID, err := uc.ids.NewInvocationID()
	if err != nil {
		return InvokeToolResponse{}, fmt.Errorf("invoke tool: allocate id: %w", err)
	}
	response := InvokeToolResponse{InvocationID: invocationID}
	startedAt := uc.clock.Now()

	scopeOutcome, terminal, scopeErr := uc.enforceExecutionScope(
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
				request.Context.Caller(),
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
			request.Context.Caller(),
			startedAt,
			auditdomain.StatusFailed,
			auditdomain.ErrorInternal,
		)
		return response, joinInvocationError(
			fmt.Errorf("invoke tool: resolve tool: %w", err),
			recordErr,
		)
	}

	decision, err := uc.policy.Evaluate(ctx, PolicyRequest{
		InvocationID:          invocationID,
		Tool:                  tool,
		Caller:                request.Context.Caller(),
		HasCredentialResolver: uc.credentials != nil,
	})
	if err != nil {
		recordErr := uc.record(
			context.WithoutCancel(ctx),
			invocationID,
			tool,
			request.Context.Caller(),
			startedAt,
			policydomain.DecisionKind(""),
			"",
			auditdomain.ApprovalNotRequired,
			auditdomain.StatusFailed,
			"",
			auditdomain.ErrorPolicy,
		)
		return response, joinInvocationError(
			fmt.Errorf("invoke tool: evaluate policy: %w", err),
			recordErr,
		)
	}

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
			request.Context.Caller(),
			startedAt,
			decision.Kind(),
			decision.ReasonCode(),
			auditdomain.ApprovalNotRequired,
			auditdomain.StatusDenied,
			"",
			auditdomain.ErrorPolicy,
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
				request.Context.Caller(),
				startedAt,
				decision.Kind(),
				decision.ReasonCode(),
				auditdomain.ApprovalDenied,
				auditdomain.StatusFailed,
				"",
				auditdomain.ErrorApproval,
			)
			return response, joinInvocationError(
				fmt.Errorf("invoke tool: approval: %w", approvalErr),
				recordErr,
			)
		}
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
				request.Context.Caller(),
				startedAt,
				decision.Kind(),
				decision.ReasonCode(),
				auditdomain.ApprovalDenied,
				auditdomain.StatusDenied,
				"",
				auditdomain.ErrorApproval,
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
		)

	default:
		recordErr := uc.record(
			context.WithoutCancel(ctx),
			invocationID,
			tool,
			request.Context.Caller(),
			startedAt,
			decision.Kind(),
			decision.ReasonCode(),
			auditdomain.ApprovalNotRequired,
			auditdomain.StatusFailed,
			"",
			auditdomain.ErrorPolicy,
		)
		return response, joinInvocationError(
			fmt.Errorf("invoke tool: unknown policy decision %q", decision.Kind()),
			recordErr,
		)
	}
}

func (uc InvokeTool) enforceExecutionScope(
	ctx context.Context,
	request InvokeToolRequest,
	invocationID invocationdomain.InvocationID,
	startedAt time.Time,
) (invocationdomain.Outcome, bool, error) {
	scope := request.Context.Scope()
	switch scope.Kind() {
	case invocationdomain.ScopeCatalog:
		return invocationdomain.Outcome{}, false, nil

	case invocationdomain.ScopeProfile:
		if uc.profileScopes == nil {
			recordErr := uc.recordUnresolvedTool(
				context.WithoutCancel(ctx),
				invocationID,
				request.ToolID,
				request.Context.Caller(),
				startedAt,
				auditdomain.StatusFailed,
				auditdomain.ErrorInternal,
			)
			return invocationdomain.Outcome{}, false, joinInvocationError(
				fmt.Errorf("invoke tool: resolve profile scope: authorizer is unavailable"),
				recordErr,
			)
		}

		allowed, err := uc.profileScopes.AllowsTool(ctx, scope.Reference(), request.ToolID)
		if err != nil {
			recordErr := uc.recordUnresolvedTool(
				context.WithoutCancel(ctx),
				invocationID,
				request.ToolID,
				request.Context.Caller(),
				startedAt,
				auditdomain.StatusFailed,
				auditdomain.ErrorInternal,
			)
			return invocationdomain.Outcome{}, false, joinInvocationError(
				fmt.Errorf("invoke tool: resolve profile scope: %w", err),
				recordErr,
			)
		}
		if !allowed {
			outcome, outcomeErr := newRejectedOutcome(
				invocationdomain.RejectionToolOutsideScope,
				"tool is not available in this execution scope",
			)
			if outcomeErr != nil {
				return invocationdomain.Outcome{}, false, outcomeErr
			}
			recordErr := uc.recordUnresolvedTool(
				context.WithoutCancel(ctx),
				invocationID,
				request.ToolID,
				request.Context.Caller(),
				startedAt,
				auditdomain.StatusDenied,
				auditdomain.ErrorPolicy,
			)
			return outcome, true, recordErr
		}
		return invocationdomain.Outcome{}, false, nil

	default:
		return invocationdomain.Outcome{}, false, fmt.Errorf(
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
) (InvokeToolResponse, error) {
	executor, err := uc.router.ExecutorFor(parent, tool)
	if err != nil {
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
			parent,
			response.InvocationID,
			tool,
			request.Context.Caller(),
			startedAt,
			decision.Kind(),
			decision.ReasonCode(),
			approval,
			auditdomain.StatusFailed,
			"",
			auditdomain.ErrorExecutorNotFound,
		)
		if recordErr != nil {
			return response, recordErr
		}
		return response, nil
	}

	timeout := request.Timeout
	if timeout == 0 {
		timeout = uc.defaultTimeout
	}
	execCtx, cancel := context.WithTimeout(parent, timeout)
	defer cancel()

	outcome, err := executor.Execute(execCtx, ExecutionRequest{
		InvocationID: response.InvocationID,
		Tool:         tool,
		Arguments:    arguments,
	}, uc.credentials)
	if err != nil {
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
			request.Context.Caller(),
			startedAt,
			decision.Kind(),
			decision.ReasonCode(),
			approval,
			status,
			"",
			class,
			summary,
		)
		if recordErr != nil {
			return response, recordErr
		}
		return response, nil
	}

	status, resultKind, class, ok := auditClassificationForExecutorOutcome(outcome)
	if !ok {
		recordErr := uc.record(
			context.WithoutCancel(parent),
			response.InvocationID,
			tool,
			request.Context.Caller(),
			startedAt,
			decision.Kind(),
			decision.ReasonCode(),
			approval,
			auditdomain.StatusFailed,
			"",
			auditdomain.ErrorExecution,
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
		request.Context.Caller(),
		startedAt,
		decision.Kind(),
		decision.ReasonCode(),
		approval,
		status,
		resultKind,
		class,
		summary,
	)
	if recordErr != nil {
		return response, recordErr
	}

	response.Outcome = outcome
	return response, nil
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
			return auditdomain.StatusFailed, "", auditdomain.ErrorExecution, true
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
	caller string,
	startedAt time.Time,
	policyKind policydomain.DecisionKind,
	policyReason string,
	approval auditdomain.ApprovalOutcome,
	status auditdomain.Status,
	resultKind invocationdomain.ResultKind,
	errorClass auditdomain.ErrorClass,
	errorSummary ...string,
) error {
	providerID := tool.ProviderID()
	record, err := auditdomain.NewRecord(
		invocationID,
		tool.ID(),
		&providerID,
		caller,
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
	if len(errorSummary) != 0 {
		record = record.WithErrorSummary(errorSummary[0])
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
	caller string,
	startedAt time.Time,
	status auditdomain.Status,
	errorClass auditdomain.ErrorClass,
) error {
	record, err := auditdomain.NewRecord(
		invocationID,
		toolID,
		nil,
		caller,
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
	if err := uc.audit.Record(ctx, record); err != nil {
		return fmt.Errorf("%w: persist unresolved tool record: %w", ErrAuditFailed, err)
	}
	return nil
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

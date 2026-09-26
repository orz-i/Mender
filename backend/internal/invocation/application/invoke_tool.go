package application

import (
	"context"
	"errors"
	"fmt"
	"time"

	auditdomain "github.com/orz-i/Mender/backend/internal/audit/domain"
	catalogdomain "github.com/orz-i/Mender/backend/internal/catalog/domain"
	invocationdomain "github.com/orz-i/Mender/backend/internal/invocation/domain"
	policydomain "github.com/orz-i/Mender/backend/internal/policy/domain"
)

const fallbackExecutionTimeout = 30 * time.Second

// InvokeToolRequest is the adapter-independent entry point for tool execution.
type InvokeToolRequest struct {
	ToolID    catalogdomain.ToolID
	Arguments []byte
	Caller    string
	Timeout   time.Duration
}

// InvokeToolResponse identifies the invocation and its canonical result.
type InvokeToolResponse struct {
	InvocationID invocationdomain.InvocationID
	Result       invocationdomain.Result
}

// InvokeTool implements Mender's shared execution pipeline.
type InvokeTool struct {
	tools          ToolLookup
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

	tool, err := uc.tools.GetTool(ctx, request.ToolID)
	if err != nil {
		return response, fmt.Errorf("invoke tool: resolve tool: %w", err)
	}

	decision, err := uc.policy.Evaluate(ctx, PolicyRequest{
		InvocationID:          invocationID,
		Tool:                  tool,
		Caller:                request.Caller,
		HasCredentialResolver: uc.credentials != nil,
	})
	if err != nil {
		recordErr := uc.record(
			ctx,
			invocationID,
			tool,
			request.Caller,
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
		recordErr := uc.record(
			ctx,
			invocationID,
			tool,
			request.Caller,
			startedAt,
			decision.Kind(),
			decision.ReasonCode(),
			auditdomain.ApprovalNotRequired,
			auditdomain.StatusDenied,
			"",
			auditdomain.ErrorPolicy,
		)
		return response, joinInvocationError(ErrPolicyDenied, recordErr)

	case policydomain.DecisionRequireApproval:
		approved, approvalErr := uc.approvals.Approve(ctx, ApprovalRequest{
			InvocationID: invocationID,
			Tool:         tool,
			Caller:       request.Caller,
			Decision:     decision,
		})
		if approvalErr != nil {
			recordErr := uc.record(
				ctx,
				invocationID,
				tool,
				request.Caller,
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
			recordErr := uc.record(
				ctx,
				invocationID,
				tool,
				request.Caller,
				startedAt,
				decision.Kind(),
				decision.ReasonCode(),
				auditdomain.ApprovalDenied,
				auditdomain.StatusDenied,
				"",
				auditdomain.ErrorApproval,
			)
			return response, joinInvocationError(ErrApprovalDenied, recordErr)
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
			ctx,
			invocationID,
			tool,
			request.Caller,
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
		recordErr := uc.record(
			parent,
			response.InvocationID,
			tool,
			request.Caller,
			startedAt,
			decision.Kind(),
			decision.ReasonCode(),
			approval,
			auditdomain.StatusFailed,
			"",
			auditdomain.ErrorExecutorNotFound,
		)
		return response, joinInvocationError(
			fmt.Errorf("%w: %w", ErrExecutorNotFound, err),
			recordErr,
		)
	}

	timeout := request.Timeout
	if timeout == 0 {
		timeout = uc.defaultTimeout
	}
	execCtx, cancel := context.WithTimeout(parent, timeout)
	defer cancel()

	result, err := executor.Execute(execCtx, ExecutionRequest{
		InvocationID: response.InvocationID,
		Tool:         tool,
		Arguments:    arguments,
	}, uc.credentials)
	if err != nil {
		status, class := classifyExecutionError(execCtx, err)
		recordErr := uc.record(
			context.WithoutCancel(parent),
			response.InvocationID,
			tool,
			request.Caller,
			startedAt,
			decision.Kind(),
			decision.ReasonCode(),
			approval,
			status,
			"",
			class,
		)
		return response, joinInvocationError(
			fmt.Errorf("invoke tool: execute: %w", err),
			recordErr,
		)
	}

	status, ok := auditStatusForResult(result.Kind())
	if !ok {
		recordErr := uc.record(
			context.WithoutCancel(parent),
			response.InvocationID,
			tool,
			request.Caller,
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

	recordErr := uc.record(
		context.WithoutCancel(parent),
		response.InvocationID,
		tool,
		request.Caller,
		startedAt,
		decision.Kind(),
		decision.ReasonCode(),
		approval,
		status,
		result.Kind(),
		auditdomain.ErrorNone,
	)
	if recordErr != nil {
		return response, recordErr
	}

	response.Result = result
	return response, nil
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
) error {
	record, err := auditdomain.NewRecord(
		invocationID,
		tool.ID(),
		tool.ProviderID(),
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
	if err := uc.audit.Record(ctx, record); err != nil {
		return fmt.Errorf("%w: persist: %w", ErrAuditFailed, err)
	}
	return nil
}

func auditStatusForResult(kind invocationdomain.ResultKind) (auditdomain.Status, bool) {
	switch kind {
	case invocationdomain.ResultKindComplete:
		return auditdomain.StatusCompleted, true
	case invocationdomain.ResultKindInputRequired:
		return auditdomain.StatusInputRequired, true
	case invocationdomain.ResultKindTask:
		return auditdomain.StatusTask, true
	default:
		return "", false
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

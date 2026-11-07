package application

import (
	"context"
	"errors"
	"fmt"
	"time"

	auditdomain "github.com/orz-i/Mender/backend/internal/audit/domain"
	invocationdomain "github.com/orz-i/Mender/backend/internal/invocation/domain"
	runtimegovernancedomain "github.com/orz-i/Mender/backend/internal/runtimegovernance/domain"
)

type invocationExecution struct {
	runtimeBudget  RuntimeBudgetController
	bindingGuard   BindingExecutionGuard
	resultGuard    ResultGuard
	router         ExecutorRouter
	credentials    CredentialResolver
	defaultTimeout time.Duration
}

func (e invocationExecution) execute(parent context.Context, attempt invocationAttempt,
	admitted admittedInvocation, finalizer invocationFinalizer,
) (InvokeToolResponse, error) {
	timeout := attempt.timeout
	if timeout == 0 {
		timeout = e.defaultTimeout
	}
	if maximum := attempt.budget.MaxExecutionTimeout(); timeout > maximum {
		timeout = maximum
	}
	// Approval is outside this execution deadline. Capacity waits, execution
	// and result guarding share it. Cancel only after terminal audit returns.
	execCtx, cancel := context.WithTimeout(parent, timeout)
	defer cancel()
	completion := e.run(execCtx, attempt, admitted)
	return finalizer.finish(parent, attempt, completion)
}

func (e invocationExecution) run(ctx context.Context, attempt invocationAttempt, admitted admittedInvocation) invocationCompletion {
	attempt.events.lifecycle(invocationdomain.EventExecutionWaiting)
	release, stop := e.acquire(ctx, attempt, admitted)
	if stop != nil {
		return *stop
	}
	prepared := admitted.prepared
	executor, err := e.router.ExecutorFor(ctx, prepared.tool)
	if err != nil {
		release()
		outcome, outcomeErr := newFailedOutcome(invocationdomain.FailureSourceRuntime,
			invocationdomain.FailureExecutorUnavailable, "tool executor is unavailable")
		stop := failedInvocation(&prepared.tool, auditdomain.ErrorExecutorNotFound, outcomeErr).withAdmission(admitted)
		stop.outcome, stop.skipAudit = outcome, outcomeErr != nil
		return stop
	}
	attempt.events.lifecycle(invocationdomain.EventExecutionStarted)
	credentials := credentialResolverForScope(e.credentials, attempt.metadata.context.Scope(), prepared.governance)
	executionRequest := ExecutionRequest{
		InvocationID: attempt.metadata.id, Tool: prepared.tool, Arguments: prepared.arguments,
		MaxResultBytes: attempt.budget.MaxResultBytes(),
		AdmittedFacts:  prepared.facts, Target: prepared.target, Context: attempt.metadata.context,
	}
	// Mint an attempt-local receipt only after the canonical Policy/Approval,
	// Runtime Budget and binding gates succeed. Native Sandbox acquisition
	// consumes this proof; arguments and plugins can never manufacture it.
	admittedCtx := withAdmittedAttempt(ctx, executionRequest)
	outcome, err := executeWithGuardRelease(admittedCtx, executor, executionRequest,
		credentials, attempt.events, release)
	if err != nil {
		return executionFailure(ctx, admitted, err)
	}
	// Capacity protects provider execution, as before: both leases have been
	// released before postflight or audit, including executor errors/panics.
	if e.resultGuard == nil {
		return executionInternalFailure(admitted, "result guard is unavailable", nil)
	}
	outcome, err = e.resultGuard.Guard(ctx, prepared.tool, outcome, attempt.budget.MaxResultBytes())
	if err != nil {
		if isExecutionContextError(ctx, err) {
			return executionFailure(ctx, admitted, err)
		}
		return executionInternalFailure(admitted, "result guard", err)
	}
	status, resultKind, class, valid := auditClassificationForExecutorOutcome(outcome)
	if !valid {
		return failedInvocation(&prepared.tool, auditdomain.ErrorExecution, ErrInvalidExecutorResult).withAdmission(admitted)
	}
	completion := invocationCompletion{tool: &prepared.tool, outcome: outcome, publishAfterAudit: true,
		audit: invocationAuditDisposition{status: status, resultKind: resultKind, errorClass: class}}.withAdmission(admitted)
	if outcome.Kind() == invocationdomain.OutcomeKindFailed {
		completion.audit.summary = redactDiagnosticSummary(outcome.Message())
	}
	return completion
}

func (e invocationExecution) acquire(ctx context.Context, attempt invocationAttempt, admitted admittedInvocation) (func(), *invocationCompletion) {
	if e.runtimeBudget == nil {
		stop := executionInternalFailure(admitted, "runtime budget controller is unavailable", nil)
		return nil, &stop
	}
	tool := admitted.prepared.tool
	budgetSource := tool.SourceRef().ID()
	if tool.SourceRef().Kind() != "provider" {
		budgetSource = tool.SourceRef().String()
	}
	budgetRelease, err := e.runtimeBudget.AcquireInvocation(ctx, attempt.budget,
		attempt.metadata.context.Principal().Reference(), budgetSource, tool.ID().String())
	if errors.Is(err, runtimegovernancedomain.ErrBudgetExceeded) {
		stop := rejectedInvocation(&tool, invocationdomain.RejectionRuntimeBudgetExceeded,
			"Runtime execution budget is saturated", auditdomain.StatusDenied, auditdomain.ErrorBudget).withAdmission(admitted)
		return nil, &stop
	}
	if err != nil {
		if isExecutionContextError(ctx, err) {
			stop := executionFailure(ctx, admitted, err)
			return nil, &stop
		}
		stop := executionInternalFailure(admitted, "acquire Runtime budget", err)
		return nil, &stop
	}
	if budgetRelease == nil {
		stop := executionInternalFailure(admitted, "Runtime budget returned no release function", nil)
		return nil, &stop
	}
	if e.bindingGuard == nil {
		budgetRelease()
		stop := executionInternalFailure(admitted, "binding execution guard is unavailable", nil)
		return nil, &stop
	}
	bindingRelease, err := e.bindingGuard.Acquire(ctx, BindingExecutionGuardRequest{
		InvocationID: attempt.metadata.id, Tool: tool, Context: attempt.metadata.context,
		Governance: admitted.prepared.governance,
	})
	if err != nil {
		budgetRelease()
		if isExecutionContextError(ctx, err) {
			stop := executionFailure(ctx, admitted, err)
			return nil, &stop
		}
		stop := executionInternalFailure(admitted, "acquire binding execution guard", err)
		return nil, &stop
	}
	if bindingRelease == nil {
		budgetRelease()
		stop := executionInternalFailure(admitted, "binding execution guard returned no release function", nil)
		return nil, &stop
	}
	return func() { bindingRelease(); budgetRelease() }, nil
}

func executeWithGuardRelease(ctx context.Context, executor ToolExecutor, request ExecutionRequest,
	credentials CredentialResolver, progress ProgressReporter, release func(),
) (invocationdomain.Outcome, error) {
	defer release()
	return executor.Execute(ctx, request, credentials, progress)
}

func executionInternalFailure(admitted admittedInvocation, stage string, cause error) invocationCompletion {
	var primary error
	if cause == nil {
		primary = fmt.Errorf("invoke tool: %s", stage)
	} else {
		primary = fmt.Errorf("invoke tool: %s: %w", stage, cause)
	}
	return failedInvocation(&admitted.prepared.tool, auditdomain.ErrorInternal, primary).withAdmission(admitted)
}

func executionFailure(ctx context.Context, admitted admittedInvocation, err error) invocationCompletion {
	outcome, status, class, outcomeErr := failedOutcomeForExecutionError(ctx, err)
	return invocationCompletion{tool: &admitted.prepared.tool, outcome: outcome, err: outcomeErr,
		skipAudit: outcomeErr != nil, audit: invocationAuditDisposition{status: status,
			errorClass: class, summary: auditErrorSummary(err)}}.withAdmission(admitted)
}

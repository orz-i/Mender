package application

import (
	"context"
	"errors"
	"fmt"

	auditdomain "github.com/orz-i/Mender/backend/internal/audit/domain"
	catalogapplication "github.com/orz-i/Mender/backend/internal/catalog/application"
	catalogdomain "github.com/orz-i/Mender/backend/internal/catalog/domain"
	governancedomain "github.com/orz-i/Mender/backend/internal/governance/domain"
	invocationdomain "github.com/orz-i/Mender/backend/internal/invocation/domain"
	runtimegovernancedomain "github.com/orz-i/Mender/backend/internal/runtimegovernance/domain"
)

type invocationPreflight struct {
	tools         ToolLookup
	profileScopes ProfileScopeResolver
	arguments     ArgumentValidator
	facts         GovernanceFactExtractor
	runtimeBudget RuntimeBudgetController
}

// preparedInvocation contains only input that has passed scope/schema/fact
// validation. The exact facts are retained through Policy and Executor.
type preparedInvocation struct {
	tool       catalogdomain.Tool
	arguments  invocationdomain.JSONValue
	facts      invocationdomain.GovernanceFacts
	governance invocationdomain.ScopedBindingGovernance
	target     ExecutionTarget
}

func (p invocationPreflight) snapshotBudget(ctx context.Context, rawArguments []byte) (runtimegovernancedomain.Budget, *invocationCompletion) {
	budget, err := p.runtimeBudgetSnapshot(ctx)
	if err != nil {
		stop := failedInvocation(nil, auditdomain.ErrorInternal, fmt.Errorf("invoke tool: load runtime budget: %w", err))
		return budget, &stop
	}
	if len(rawArguments) > budget.MaxArgumentBytes() {
		stop := rejectedInvocation(nil, invocationdomain.RejectionRuntimeBudgetExceeded,
			"tool arguments exceed the configured Runtime budget", auditdomain.StatusDenied, auditdomain.ErrorBudget)
		return budget, &stop
	}
	return budget, nil
}

func (p invocationPreflight) runtimeBudgetSnapshot(ctx context.Context) (runtimegovernancedomain.Budget, error) {
	if p.runtimeBudget == nil {
		return runtimegovernancedomain.Budget{}, fmt.Errorf("runtime budget controller is unavailable")
	}
	budget, err := p.runtimeBudget.Snapshot(ctx)
	if err != nil {
		return runtimegovernancedomain.Budget{}, err
	}
	if err := budget.Validate(); err != nil {
		return runtimegovernancedomain.Budget{}, fmt.Errorf("invalid Runtime budget snapshot: %w", err)
	}
	return budget, nil
}

func (p invocationPreflight) prepare(ctx context.Context, attempt invocationAttempt, rawArguments []byte) (preparedInvocation, *invocationCompletion) {
	governance, stop := p.resolveScope(ctx, attempt.metadata)
	if stop != nil {
		return preparedInvocation{}, stop
	}
	attempt.events.lifecycle(invocationdomain.EventScopeResolved)
	tool, err := p.tools.GetTool(ctx, attempt.metadata.toolID)
	if err != nil {
		var stop invocationCompletion
		if errors.Is(err, catalogapplication.ErrToolNotFound) {
			stop = rejectedInvocation(nil, invocationdomain.RejectionToolNotFound,
				"tool was not found", auditdomain.StatusFailed, auditdomain.ErrorToolNotFound)
		} else {
			stop = failedInvocation(nil, auditdomain.ErrorInternal, fmt.Errorf("invoke tool: resolve tool: %w", err))
		}
		return preparedInvocation{}, &stop
	}
	arguments, err := invocationdomain.NewJSONValue(rawArguments)
	if err != nil {
		return preparedInvocation{}, invalidArguments(tool)
	}
	if p.arguments == nil {
		return preparedInvocation{}, preflightFailure(tool, "argument validator is unavailable", nil)
	}
	valid, err := p.arguments.Validate(ctx, tool.InputSchema(), arguments)
	if err != nil {
		return preparedInvocation{}, preflightFailure(tool, "validate tool arguments", err)
	}
	if !valid {
		return preparedInvocation{}, invalidArguments(tool)
	}
	attempt.events.lifecycle(invocationdomain.EventArgumentsValidated)
	if p.facts == nil {
		return preparedInvocation{}, preflightFailure(tool, "governance fact extractor is unavailable", nil)
	}
	extracted, err := p.facts.Extract(ctx, tool, arguments, attempt.metadata.context)
	if err != nil {
		return preparedInvocation{}, preflightFailure(tool, "derive governance facts", err)
	}
	if err := extracted.Facts.Validate(); err != nil {
		return preparedInvocation{}, preflightFailure(tool, "validate governance facts", err)
	}
	if extracted.Target != nil {
		if tool.SourceRef().Kind() != catalogdomain.SourceNative ||
			!sameApprovedFacts(extracted.Facts, extracted.Target.Facts()) {
			return preparedInvocation{}, preflightFailure(tool, "execution target facts mismatch", nil)
		}
	} else if tool.SourceRef().Kind() == catalogdomain.SourceNative {
		return preparedInvocation{}, preflightFailure(tool, "native execution target is unavailable", nil)
	}
	return preparedInvocation{tool: tool, arguments: arguments, facts: extracted.Facts,
		governance: governance, target: extracted.Target}, nil
}

func (p invocationPreflight) resolveScope(ctx context.Context, metadata invocationMetadata) (invocationdomain.ScopedBindingGovernance, *invocationCompletion) {
	scope := metadata.context.Scope()
	switch scope.Kind() {
	case governancedomain.ScopeCatalog:
		return invocationdomain.InheritedScopedBindingGovernance(), nil
	case governancedomain.ScopeProfile:
		if p.profileScopes == nil {
			stop := failedInvocation(nil, auditdomain.ErrorInternal, fmt.Errorf("invoke tool: resolve profile scope: resolver is unavailable"))
			return invocationdomain.ScopedBindingGovernance{}, &stop
		}
		governance, found, err := p.profileScopes.ResolveBindingGovernance(ctx, scope.Reference(), metadata.toolID)
		if err != nil {
			stop := failedInvocation(nil, auditdomain.ErrorInternal, fmt.Errorf("invoke tool: resolve profile scope: %w", err))
			return invocationdomain.ScopedBindingGovernance{}, &stop
		}
		if !found {
			stop := rejectedInvocation(nil, invocationdomain.RejectionToolOutsideScope,
				"tool is not available in this execution scope", auditdomain.StatusDenied, auditdomain.ErrorPolicy)
			// ScopeResolved historically follows durable scope-denial audit.
			stop.afterAuditEvent = invocationdomain.EventScopeResolved
			return invocationdomain.ScopedBindingGovernance{}, &stop
		}
		if err := governance.Validate(); err != nil {
			stop := failedInvocation(nil, auditdomain.ErrorInternal, fmt.Errorf("invoke tool: invalid scoped binding governance: %w", err))
			return invocationdomain.ScopedBindingGovernance{}, &stop
		}
		return governance, nil
	default:
		stop := invocationCompletion{err: fmt.Errorf("%w: unknown scope kind %q", ErrInvalidInvocationContext, scope.Kind()), skipAudit: true}
		return invocationdomain.ScopedBindingGovernance{}, &stop
	}
}

func invalidArguments(tool catalogdomain.Tool) *invocationCompletion {
	stop := rejectedInvocation(&tool, invocationdomain.RejectionInvalidArguments,
		"arguments do not match the tool input schema", auditdomain.StatusFailed, auditdomain.ErrorInvalidArguments)
	return &stop
}

func preflightFailure(tool catalogdomain.Tool, stage string, cause error) *invocationCompletion {
	var primary error
	if cause == nil {
		primary = fmt.Errorf("invoke tool: preflight: %s", stage)
	} else {
		primary = fmt.Errorf("invoke tool: preflight: %s: %w", stage, cause)
	}
	stop := failedInvocation(&tool, auditdomain.ErrorInternal, primary)
	return &stop
}

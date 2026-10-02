package application

import (
	"context"
	"errors"
	"fmt"
	"time"

	auditdomain "github.com/orz-i/Mender/backend/internal/audit/domain"
	catalogdomain "github.com/orz-i/Mender/backend/internal/catalog/domain"
	governanceapplication "github.com/orz-i/Mender/backend/internal/governance/application"
	governancedomain "github.com/orz-i/Mender/backend/internal/governance/domain"
	policyapplication "github.com/orz-i/Mender/backend/internal/policy/application"
	policydomain "github.com/orz-i/Mender/backend/internal/policy/domain"
	runtimegovernancedomain "github.com/orz-i/Mender/backend/internal/runtimegovernance/domain"
)

var (
	ErrPolicyDenied   = errors.New("provider activation denied by policy")
	ErrApprovalDenied = errors.New("provider activation approval denied")
)

type Request struct {
	ProviderID catalogdomain.ProviderID
	Context    governancedomain.Context
}

type Operation func(context.Context) error

type PolicyEvaluator interface {
	Execute(context.Context, policyapplication.EvaluateRequest) (policydomain.Decision, error)
}

type RuntimeBudgetController interface {
	Snapshot(context.Context) (runtimegovernancedomain.Budget, error)
	AcquireActivation(
		context.Context,
		runtimegovernancedomain.Budget,
	) (release func(), err error)
}

type AuditRecorder interface {
	RecordProviderActivation(context.Context, auditdomain.ProviderActivationRecord) error
}

type IDGenerator interface {
	NewActivationID() (string, error)
}

type Clock interface {
	Now() time.Time
}

type Admit struct {
	policy    PolicyEvaluator
	approvals governanceapplication.ApprovalGate
	budget    RuntimeBudgetController
	audit     AuditRecorder
	ids       IDGenerator
	clock     Clock
}

func NewAdmit(
	policy PolicyEvaluator,
	approvals governanceapplication.ApprovalGate,
	budget RuntimeBudgetController,
	audit AuditRecorder,
	ids IDGenerator,
	clock Clock,
) Admit {
	return Admit{
		policy:    policy,
		approvals: approvals,
		budget:    budget,
		audit:     audit,
		ids:       ids,
		clock:     clock,
	}
}

func (uc Admit) Execute(
	ctx context.Context,
	request Request,
	operation Operation,
) error {
	if err := request.Context.Validate(); err != nil {
		return fmt.Errorf("admit provider activation: context: %w", err)
	}
	if _, err := catalogdomain.ParseProviderID(request.ProviderID.String()); err != nil {
		return fmt.Errorf("admit provider activation: provider: %w", err)
	}
	if operation == nil {
		return fmt.Errorf("admit provider activation: operation is required")
	}
	if uc.policy == nil || uc.approvals == nil || uc.budget == nil || uc.audit == nil || uc.ids == nil || uc.clock == nil {
		return fmt.Errorf("admit provider activation: dependencies are unavailable")
	}

	activationID, err := uc.ids.NewActivationID()
	if err != nil {
		return fmt.Errorf("admit provider activation: allocate id: %w", err)
	}
	startedAt := uc.clock.Now()
	principal := request.Context.Principal()
	scope := request.Context.Scope()
	decision, err := uc.policy.Execute(ctx, policyapplication.EvaluateRequest{
		Action:             governancedomain.ActionProviderActivate,
		Origin:             request.Context.Origin(),
		PrincipalKind:      principal.Kind(),
		PrincipalReference: principal.Reference(),
		ScopeKind:          scope.Kind(),
		ScopeReference:     scope.Reference(),
		ProviderID:         request.ProviderID.String(),
	})
	if err != nil {
		recordErr := uc.record(
			context.WithoutCancel(ctx),
			activationID,
			request,
			startedAt,
			policydomain.DecisionKind(""),
			"",
			auditdomain.ApprovalNotRequired,
			auditdomain.ProviderActivationFailed,
			auditdomain.ProviderActivationErrorPolicy,
			err,
		)
		return errors.Join(
			fmt.Errorf("admit provider activation: evaluate policy: %w", err),
			recordErr,
		)
	}

	switch decision.Kind() {
	case policydomain.DecisionDeny:
		recordErr := uc.record(
			context.WithoutCancel(ctx),
			activationID,
			request,
			startedAt,
			decision.Kind(),
			decision.ReasonCode(),
			auditdomain.ApprovalNotRequired,
			auditdomain.ProviderActivationDenied,
			auditdomain.ProviderActivationErrorPolicy,
			nil,
		)
		return errors.Join(ErrPolicyDenied, recordErr)

	case policydomain.DecisionRequireApproval:
		approved, approvalErr := uc.approvals.Approve(
			ctx,
			governanceapplication.ApprovalRequest{
				OperationID: activationID,
				Action:      governancedomain.ActionProviderActivate,
				Context:     request.Context,
				ProviderID:  request.ProviderID,
				Decision:    decision,
			},
		)
		if approvalErr != nil {
			recordErr := uc.record(
				context.WithoutCancel(ctx),
				activationID,
				request,
				startedAt,
				decision.Kind(),
				decision.ReasonCode(),
				auditdomain.ApprovalDenied,
				auditdomain.ProviderActivationFailed,
				auditdomain.ProviderActivationErrorApproval,
				approvalErr,
			)
			return errors.Join(
				fmt.Errorf("admit provider activation: approval: %w", approvalErr),
				recordErr,
			)
		}
		if !approved {
			recordErr := uc.record(
				context.WithoutCancel(ctx),
				activationID,
				request,
				startedAt,
				decision.Kind(),
				decision.ReasonCode(),
				auditdomain.ApprovalDenied,
				auditdomain.ProviderActivationDenied,
				auditdomain.ProviderActivationErrorApproval,
				nil,
			)
			return errors.Join(ErrApprovalDenied, recordErr)
		}
		return uc.executeAdmitted(
			ctx,
			activationID,
			request,
			startedAt,
			decision,
			auditdomain.ApprovalApproved,
			operation,
		)

	case policydomain.DecisionAllow:
		return uc.executeAdmitted(
			ctx,
			activationID,
			request,
			startedAt,
			decision,
			auditdomain.ApprovalNotRequired,
			operation,
		)

	default:
		return fmt.Errorf("admit provider activation: unknown policy decision %q", decision.Kind())
	}
}

func (uc Admit) executeAdmitted(
	ctx context.Context,
	activationID string,
	request Request,
	startedAt time.Time,
	decision policydomain.Decision,
	approval auditdomain.ApprovalOutcome,
	operation Operation,
) error {
	snapshot, err := uc.budget.Snapshot(ctx)
	if err != nil {
		recordErr := uc.record(
			context.WithoutCancel(ctx),
			activationID,
			request,
			startedAt,
			decision.Kind(),
			decision.ReasonCode(),
			approval,
			auditdomain.ProviderActivationFailed,
			auditdomain.ProviderActivationErrorRuntime,
			err,
		)
		return errors.Join(
			fmt.Errorf("admit provider activation: load Runtime budget: %w", err),
			recordErr,
		)
	}
	if err := snapshot.Validate(); err != nil {
		recordErr := uc.record(
			context.WithoutCancel(ctx),
			activationID,
			request,
			startedAt,
			decision.Kind(),
			decision.ReasonCode(),
			approval,
			auditdomain.ProviderActivationFailed,
			auditdomain.ProviderActivationErrorRuntime,
			err,
		)
		return errors.Join(
			fmt.Errorf("admit provider activation: invalid Runtime budget: %w", err),
			recordErr,
		)
	}

	release, err := uc.budget.AcquireActivation(ctx, snapshot)
	if errors.Is(err, runtimegovernancedomain.ErrBudgetExceeded) {
		recordErr := uc.record(
			context.WithoutCancel(ctx),
			activationID,
			request,
			startedAt,
			decision.Kind(),
			decision.ReasonCode(),
			approval,
			auditdomain.ProviderActivationDenied,
			auditdomain.ProviderActivationErrorBudget,
			nil,
		)
		return errors.Join(runtimegovernancedomain.ErrBudgetExceeded, recordErr)
	}
	if err != nil {
		recordErr := uc.record(
			context.WithoutCancel(ctx),
			activationID,
			request,
			startedAt,
			decision.Kind(),
			decision.ReasonCode(),
			approval,
			auditdomain.ProviderActivationFailed,
			auditdomain.ProviderActivationErrorRuntime,
			err,
		)
		return errors.Join(
			fmt.Errorf("admit provider activation: acquire Runtime budget: %w", err),
			recordErr,
		)
	}
	if release == nil {
		err := fmt.Errorf("runtime budget returned no release function")
		recordErr := uc.record(
			context.WithoutCancel(ctx),
			activationID,
			request,
			startedAt,
			decision.Kind(),
			decision.ReasonCode(),
			approval,
			auditdomain.ProviderActivationFailed,
			auditdomain.ProviderActivationErrorRuntime,
			err,
		)
		return errors.Join(err, recordErr)
	}
	defer release()

	if err := operation(ctx); err != nil {
		recordErr := uc.record(
			context.WithoutCancel(ctx),
			activationID,
			request,
			startedAt,
			decision.Kind(),
			decision.ReasonCode(),
			approval,
			auditdomain.ProviderActivationFailed,
			auditdomain.ProviderActivationErrorRuntime,
			err,
		)
		return errors.Join(err, recordErr)
	}
	return uc.record(
		context.WithoutCancel(ctx),
		activationID,
		request,
		startedAt,
		decision.Kind(),
		decision.ReasonCode(),
		approval,
		auditdomain.ProviderActivationCompleted,
		auditdomain.ProviderActivationErrorNone,
		nil,
	)
}

func (uc Admit) record(
	ctx context.Context,
	activationID string,
	request Request,
	startedAt time.Time,
	policyKind policydomain.DecisionKind,
	policyReason string,
	approval auditdomain.ApprovalOutcome,
	status auditdomain.ProviderActivationStatus,
	errorClass auditdomain.ProviderActivationErrorClass,
	cause error,
) error {
	record, err := auditdomain.NewProviderActivationRecord(
		activationID,
		request.ProviderID,
		request.Context,
		startedAt,
		uc.clock.Now(),
		policyKind,
		policyReason,
		approval,
		status,
		errorClass,
	)
	if err != nil {
		return fmt.Errorf("record provider activation: build record: %w", err)
	}
	if cause != nil {
		record = record.WithErrorSummary(cause.Error())
	}
	if err := uc.audit.RecordProviderActivation(ctx, record); err != nil {
		return fmt.Errorf("record provider activation: %w", err)
	}
	return nil
}

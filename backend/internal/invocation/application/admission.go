package application

import (
	"context"
	"fmt"

	auditdomain "github.com/orz-i/Mender/backend/internal/audit/domain"
	governanceapplication "github.com/orz-i/Mender/backend/internal/governance/application"
	governancedomain "github.com/orz-i/Mender/backend/internal/governance/domain"
	invocationdomain "github.com/orz-i/Mender/backend/internal/invocation/domain"
	policydomain "github.com/orz-i/Mender/backend/internal/policy/domain"
)

type invocationAdmission struct {
	policy         PolicyEvaluator
	approvals      governanceapplication.ApprovalGate
	hasCredentials bool
}

type admittedInvocation struct {
	prepared preparedInvocation
	decision policydomain.Decision
	approval auditdomain.ApprovalOutcome
}

func (a invocationAdmission) admit(ctx context.Context, attempt invocationAttempt, prepared preparedInvocation) (admittedInvocation, *invocationCompletion) {
	decision, err := a.policy.Evaluate(ctx, PolicyRequest{
		InvocationID: attempt.metadata.id, Tool: prepared.tool, Context: attempt.metadata.context,
		Facts: prepared.facts, HasCredentialResolver: a.hasCredentials,
	})
	if err != nil {
		stop := failedInvocation(&prepared.tool, auditdomain.ErrorPolicy, fmt.Errorf("invoke tool: evaluate policy: %w", err))
		return admittedInvocation{}, &stop
	}
	decision, err = applyBindingPolicyConstraint(decision, prepared.governance.PolicyConstraint())
	if err != nil {
		stop := failedInvocation(&prepared.tool, auditdomain.ErrorInternal, fmt.Errorf("invoke tool: apply binding policy constraint: %w", err))
		return admittedInvocation{}, &stop
	}
	attempt.events.lifecycle(invocationdomain.EventPolicyDecided)
	admitted := admittedInvocation{prepared: prepared, decision: decision, approval: auditdomain.ApprovalNotRequired}
	switch decision.Kind() {
	case policydomain.DecisionDeny:
		message := decision.Explanation()
		if message == "" {
			message = "invocation denied by policy"
		}
		stop := rejectedInvocation(&prepared.tool, invocationdomain.RejectionPolicyDenied,
			message, auditdomain.StatusDenied, auditdomain.ErrorPolicy).withAdmission(admitted)
		return admittedInvocation{}, &stop
	case policydomain.DecisionRequireApproval:
		toolID := prepared.tool.ID()
		approved, err := a.approvals.Approve(ctx, governanceapplication.ApprovalRequest{
			OperationID: attempt.metadata.id.String(), Action: governancedomain.ActionToolInvoke,
			Context: attempt.metadata.context, SourceRef: prepared.tool.SourceRef(), ToolID: &toolID, Decision: decision,
		})
		if err != nil {
			admitted.approval = auditdomain.ApprovalDenied
			stop := failedInvocation(&prepared.tool, auditdomain.ErrorApproval, fmt.Errorf("invoke tool: approval: %w", err)).withAdmission(admitted)
			// Approval failures retain the parent's cancellation semantics.
			stop.useParentContext = true
			return admittedInvocation{}, &stop
		}
		attempt.events.lifecycle(invocationdomain.EventApprovalResolved)
		if !approved {
			admitted.approval = auditdomain.ApprovalDenied
			stop := rejectedInvocation(&prepared.tool, invocationdomain.RejectionApprovalDenied,
				"required approval was denied", auditdomain.StatusDenied, auditdomain.ErrorApproval).withAdmission(admitted)
			stop.useParentContext = true
			return admittedInvocation{}, &stop
		}
		admitted.approval = auditdomain.ApprovalApproved
		return admitted, nil
	case policydomain.DecisionAllow:
		return admitted, nil
	default:
		stop := failedInvocation(&prepared.tool, auditdomain.ErrorPolicy,
			fmt.Errorf("invoke tool: unknown policy decision %q", decision.Kind())).withAdmission(admitted)
		return admittedInvocation{}, &stop
	}
}

package application

import (
	"context"
	"errors"
	"fmt"
	"time"

	governanceapplication "github.com/orz-i/Mender/backend/internal/governance/application"
	governancedomain "github.com/orz-i/Mender/backend/internal/governance/domain"
	policyapplication "github.com/orz-i/Mender/backend/internal/policy/application"
	policydomain "github.com/orz-i/Mender/backend/internal/policy/domain"
	sandboxdomain "github.com/orz-i/Mender/backend/internal/sandbox/domain"
)

var ErrApprovalDenied = errors.New("sandbox approval denied")

// AdmissionAudit is policy/admission evidence, not a claim the Sandbox operation completed.
// It never stores command input, host roots, credentials, or backend errors.
type AdmissionAudit struct {
	OperationID    string
	SandboxID      sandboxdomain.ID
	Action         governancedomain.Action
	PrincipalKind  governancedomain.PrincipalKind
	PrincipalRef   string
	Origin         governancedomain.Origin
	ScopeKind      governancedomain.ScopeKind
	ScopeReference string
	PolicyKind     policydomain.DecisionKind
	PolicyReason   string
	Approval       string
	Status         string // admitted, denied, failed
	StartedAt      time.Time
	FinishedAt     time.Time
}

type AdmissionPolicy interface {
	Execute(context.Context, policyapplication.EvaluateRequest) (policydomain.Decision, error)
}
type AdmissionAuditRecorder interface {
	RecordSandboxAdmission(context.Context, AdmissionAudit) error
}
type AdmissionIDGenerator interface{ NewAdmissionID() (string, error) }
type AdmissionClock interface{ Now() time.Time }

type Admit struct {
	policy   AdmissionPolicy
	approval governanceapplication.ApprovalGate
	audit    AdmissionAuditRecorder
	ids      AdmissionIDGenerator
	clock    AdmissionClock
}

func NewAdmit(policy AdmissionPolicy, approval governanceapplication.ApprovalGate, audit AdmissionAuditRecorder, ids AdmissionIDGenerator, clock AdmissionClock) Admit {
	return Admit{policy: policy, approval: approval, audit: audit, ids: ids, clock: clock}
}

// AdmitAcquire evaluates sandbox.acquire before backend access or secret resolution.
// A missing dependency or failed Audit persist prevents acquisition.
func (a Admit) AdmitAcquire(ctx context.Context, governance governancedomain.Context, placement Placement) error {
	if err := governance.Validate(); err != nil {
		return fmt.Errorf("admit sandbox: governance: %w", err)
	}
	if err := placement.Sandbox.Validate(); err != nil {
		return fmt.Errorf("admit sandbox: placement: %w", err)
	}
	if err := placement.Required.Validate(); err != nil {
		return fmt.Errorf("admit sandbox: capability: %w", err)
	}
	if a.policy == nil || a.approval == nil || a.audit == nil || a.ids == nil || a.clock == nil {
		return fmt.Errorf("admit sandbox: governance dependencies unavailable")
	}
	opID, err := a.ids.NewAdmissionID()
	if err != nil {
		return fmt.Errorf("admit sandbox: operation id: %w", err)
	}
	started := a.clock.Now().UTC()
	principal := governance.Principal()
	scope := governance.Scope()
	spec := placement.Sandbox.Spec()
	record := AdmissionAudit{OperationID: opID, SandboxID: spec.ID, Action: governancedomain.ActionSandboxAcquire, PrincipalKind: principal.Kind(), PrincipalRef: principal.Reference(), Origin: governance.Origin(), ScopeKind: scope.Kind(), ScopeReference: scope.Reference(), StartedAt: started, Approval: "not_required", Status: "failed"}
	persist := func() error {
		record.FinishedAt = a.clock.Now().UTC()
		auditCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 5*time.Second)
		defer cancel()
		return a.audit.RecordSandboxAdmission(auditCtx, record)
	}
	decision, err := a.policy.Execute(ctx, policyapplication.EvaluateRequest{
		Action: governancedomain.ActionSandboxAcquire,
		Origin: governance.Origin(), PrincipalKind: principal.Kind(), PrincipalReference: principal.Reference(),
		ScopeKind: scope.Kind(), ScopeReference: scope.Reference(),
		FactKnowledge: policydomain.FactKnown,
		Effects:       []string{"sandbox.acquire"},
		Resources:     []policyapplication.ResourceFact{{Kind: "sandbox_id", Value: spec.ID.String()}, {Kind: "sandbox_backend", Value: string(spec.Backend)}, {Kind: "sandbox_isolation", Value: string(spec.Isolation)}},
	})
	if err != nil {
		return errors.Join(fmt.Errorf("admit sandbox: evaluate policy: %w", err), persist())
	}
	record.PolicyKind = decision.Kind()
	record.PolicyReason = decision.ReasonCode()
	switch decision.Kind() {
	case policydomain.DecisionDeny:
		record.Status = "denied"
		return errors.Join(sandboxdomain.ErrNotAuthorized, persist())
	case policydomain.DecisionRequireApproval:
		approved, approvalErr := a.approval.Approve(ctx, governanceapplication.ApprovalRequest{OperationID: opID, Action: governancedomain.ActionSandboxAcquire, Context: governance, SandboxID: spec.ID.String(), Decision: decision})
		record.Approval = "denied"
		if approvalErr != nil {
			return errors.Join(fmt.Errorf("admit sandbox: approval failed: %w", approvalErr), persist())
		}
		if !approved {
			record.Status = "denied"
			return errors.Join(ErrApprovalDenied, persist())
		}
		record.Approval = "approved"
	case policydomain.DecisionAllow:
	default:
		return errors.Join(fmt.Errorf("admit sandbox: unknown policy decision %q", decision.Kind()), persist())
	}
	record.Status = "admitted"
	if err := persist(); err != nil {
		return fmt.Errorf("admit sandbox: audit failed: %w", err)
	}
	return nil
}

var _ AdmissionGate = Admit{}

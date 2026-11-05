package application

import (
	"context"
	"errors"
	"fmt"
	"time"

	agentdomain "github.com/orz-i/Mender/backend/internal/agent/domain"
	gov "github.com/orz-i/Mender/backend/internal/governance/domain"
	policyapplication "github.com/orz-i/Mender/backend/internal/policy/application"
	policydomain "github.com/orz-i/Mender/backend/internal/policy/domain"
)

type AdmissionPolicy interface {
	Execute(context.Context, policyapplication.EvaluateRequest) (policydomain.Decision, error)
}

type AdmissionRecorder interface {
	RecordAdmission(context.Context, agentdomain.AdmissionEvent) error
}

// Admit owns Agent action admission. ACP native permission requests remain a
// separate checkpoint; require_approval cannot admit an Agent action here.
type Admit struct {
	policy AdmissionPolicy
	audit  AdmissionRecorder
}

func NewAdmit(policy AdmissionPolicy, audit AdmissionRecorder) Admit {
	return Admit{policy: policy, audit: audit}
}

func (a Admit) Execute(ctx context.Context, caller gov.Context, agent agentdomain.Agent, action gov.Action) error {
	if err := caller.Validate(); err != nil {
		return agentdomain.ErrForbidden
	}
	if a.policy == nil || a.audit == nil {
		return agentdomain.ErrUnavailable
	}
	// Agent identity, backend and isolation are reviewed resource metadata,
	// never model-provided identity, an arbitrary executable or prompt content.
	decision, err := a.policy.Execute(ctx, policyapplication.EvaluateRequest{
		Action: action, Origin: caller.Origin(), PrincipalKind: caller.Principal().Kind(),
		PrincipalReference: caller.Principal().Reference(), ScopeKind: caller.Scope().Kind(),
		ScopeReference: caller.Scope().Reference(), FactKnowledge: policydomain.FactKnown,
		Resources: []policyapplication.ResourceFact{
			{Kind: "agent_id", Value: string(agent.ID())},
			{Kind: "agent_backend", Value: string(agent.Spec().Backend)},
			{Kind: "agent_isolation", Value: agent.Spec().Isolation},
		},
	})
	policyKind := "failed"
	policyReason := "policy_evaluation_error"
	status := "failed"
	if err == nil {
		policyKind = string(decision.Kind())
		policyReason = decision.ReasonCode()
		if decision.Kind() == policydomain.DecisionAllow {
			status = "admitted"
		} else {
			status = "denied"
		}
	}
	recordCtx, recordCancel := context.WithTimeout(context.WithoutCancel(ctx), 5*time.Second)
	recordErr := a.audit.RecordAdmission(recordCtx, agentdomain.AdmissionEvent{
		AgentID: agent.ID(), Action: string(action),
		PrincipalKind: string(caller.Principal().Kind()), PrincipalRef: caller.Principal().Reference(),
		Origin: string(caller.Origin()), ScopeKind: string(caller.Scope().Kind()),
		ScopeRef: caller.Scope().Reference(), PolicyKind: policyKind, PolicyReason: policyReason, Status: status,
	})
	recordCancel()
	if err != nil {
		return errors.Join(fmt.Errorf("agent admission: %w", err), recordErr)
	}
	if recordErr != nil {
		return fmt.Errorf("agent admission audit unavailable: %w", recordErr)
	}
	if decision.Kind() != policydomain.DecisionAllow {
		return agentdomain.ErrForbidden
	}
	return nil
}

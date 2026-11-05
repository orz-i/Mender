package application

import (
	"context"
	"errors"
	"path/filepath"
	"reflect"
	"testing"
	"time"

	agentdomain "github.com/orz-i/Mender/backend/internal/agent/domain"
	gov "github.com/orz-i/Mender/backend/internal/governance/domain"
	policyapplication "github.com/orz-i/Mender/backend/internal/policy/application"
	policydomain "github.com/orz-i/Mender/backend/internal/policy/domain"
)

type admissionPolicyFunc func(context.Context, policyapplication.EvaluateRequest) (policydomain.Decision, error)

func (f admissionPolicyFunc) Execute(ctx context.Context, request policyapplication.EvaluateRequest) (policydomain.Decision, error) {
	return f(ctx, request)
}

type admissionRecorderFunc func(context.Context, agentdomain.AdmissionEvent) error

func (f admissionRecorderFunc) RecordAdmission(ctx context.Context, event agentdomain.AdmissionEvent) error {
	return f(ctx, event)
}

func admissionAgent(t *testing.T, caller gov.Context) agentdomain.Agent {
	t.Helper()
	backend := agentdomain.BackendGrok
	agent, err := agentdomain.NewAgent(agentdomain.Spec{
		ID: "agent_00000000000000000000000000000001", Name: "Admission fixture",
		Backend: backend, PluginID: backend.PluginID(), ExtensionID: "main",
		WorkspaceRoot: filepath.Clean(t.TempDir()), Isolation: "host",
		OwnerKind: string(caller.Principal().Kind()), OwnerRef: caller.Principal().Reference(),
		CreatedAt: time.Now(),
	})
	if err != nil {
		t.Fatal(err)
	}
	return agent
}

func TestAdmitPreservesPolicyAndAuditFailureSemantics(t *testing.T) {
	policyErr := errors.New("fixture policy failure")
	auditErr := errors.New("fixture audit failure")
	allow, err := policydomain.Allow("fixture_allow", "")
	if err != nil {
		t.Fatal(err)
	}
	deny, err := policydomain.Deny("fixture_deny", "")
	if err != nil {
		t.Fatal(err)
	}
	approval, err := policydomain.RequireApproval("fixture_approval", "")
	if err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct {
		name       string
		decision   policydomain.Decision
		policyErr  error
		auditErr   error
		wantKind   string
		wantReason string
		wantStatus string
		wantErr    error
	}{
		{name: "allow", decision: allow, wantKind: "allow", wantReason: "fixture_allow", wantStatus: "admitted"},
		{name: "deny", decision: deny, wantKind: "deny", wantReason: "fixture_deny", wantStatus: "denied", wantErr: agentdomain.ErrForbidden},
		{name: "approval remains denied", decision: approval, wantKind: "require_approval", wantReason: "fixture_approval", wantStatus: "denied", wantErr: agentdomain.ErrForbidden},
		{name: "policy failure", policyErr: policyErr, wantKind: "failed", wantReason: "policy_evaluation_error", wantStatus: "failed", wantErr: policyErr},
		{name: "audit failure prevents allow", decision: allow, auditErr: auditErr, wantKind: "allow", wantReason: "fixture_allow", wantStatus: "admitted", wantErr: auditErr},
		{name: "both failures retained", policyErr: policyErr, auditErr: auditErr, wantKind: "failed", wantReason: "policy_evaluation_error", wantStatus: "failed", wantErr: policyErr},
	} {
		t.Run(tc.name, func(t *testing.T) {
			caller := owner(t)
			agent := admissionAgent(t, caller)
			var gotInput policyapplication.EvaluateRequest
			var gotEvent agentdomain.AdmissionEvent
			writes := 0
			policy := admissionPolicyFunc(func(_ context.Context, request policyapplication.EvaluateRequest) (policydomain.Decision, error) {
				gotInput = request
				return tc.decision, tc.policyErr
			})
			audit := admissionRecorderFunc(func(ctx context.Context, event agentdomain.AdmissionEvent) error {
				writes++
				gotEvent = event
				if ctx.Err() != nil {
					t.Fatalf("admission audit inherits caller cancellation: %v", ctx.Err())
				}
				if deadline, ok := ctx.Deadline(); !ok || time.Until(deadline) > 5*time.Second {
					t.Fatal("admission audit must have its own bounded deadline")
				}
				return tc.auditErr
			})
			ctx, cancel := context.WithCancel(context.Background())
			cancel()
			gotErr := NewAdmit(policy, audit).Execute(ctx, caller, agent, gov.ActionAgentTaskSubmit)
			if !errors.Is(gotErr, tc.wantErr) || (tc.wantErr == nil && gotErr != nil) {
				t.Fatalf("admission error = %v, want %v", gotErr, tc.wantErr)
			}
			if tc.auditErr != nil && !errors.Is(gotErr, tc.auditErr) {
				t.Fatalf("audit failure lost: %v", gotErr)
			}
			if writes != 1 || gotEvent.PolicyKind != tc.wantKind || gotEvent.PolicyReason != tc.wantReason || gotEvent.Status != tc.wantStatus {
				t.Fatalf("admission evidence = %+v (%d writes)", gotEvent, writes)
			}
			if gotEvent.AgentID != agent.ID() || gotEvent.Action != string(gov.ActionAgentTaskSubmit) ||
				gotEvent.PrincipalKind != string(caller.Principal().Kind()) || gotEvent.PrincipalRef != caller.Principal().Reference() ||
				gotEvent.Origin != string(caller.Origin()) || gotEvent.ScopeKind != string(caller.Scope().Kind()) || gotEvent.ScopeRef != caller.Scope().Reference() {
				t.Fatalf("admission attribution changed: %+v", gotEvent)
			}
			wantResources := []policyapplication.ResourceFact{
				{Kind: "agent_id", Value: string(agent.ID())},
				{Kind: "agent_backend", Value: string(agent.Spec().Backend)},
				{Kind: "agent_isolation", Value: agent.Spec().Isolation},
			}
			if gotInput.Action != gov.ActionAgentTaskSubmit || gotInput.FactKnowledge != policydomain.FactKnown ||
				gotInput.Origin != caller.Origin() || gotInput.PrincipalKind != caller.Principal().Kind() ||
				gotInput.PrincipalReference != caller.Principal().Reference() || gotInput.ScopeKind != caller.Scope().Kind() ||
				gotInput.ScopeReference != caller.Scope().Reference() || !reflect.DeepEqual(gotInput.Resources, wantResources) {
				t.Fatalf("policy input changed: %+v", gotInput)
			}
		})
	}
}

func TestAdmitRejectsInvalidContextBeforePolicyOrAudit(t *testing.T) {
	policy := admissionPolicyFunc(func(context.Context, policyapplication.EvaluateRequest) (policydomain.Decision, error) {
		t.Fatal("invalid identity reached Policy")
		return policydomain.Decision{}, nil
	})
	audit := admissionRecorderFunc(func(context.Context, agentdomain.AdmissionEvent) error {
		t.Fatal("invalid identity reached admission audit")
		return nil
	})
	err := NewAdmit(policy, audit).Execute(context.Background(), gov.Context{}, agentdomain.Agent{}, gov.ActionAgentActivate)
	if !errors.Is(err, agentdomain.ErrForbidden) {
		t.Fatalf("invalid identity admission = %v", err)
	}
}

package application

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	governanceapplication "github.com/orz-i/Mender/backend/internal/governance/application"
	gov "github.com/orz-i/Mender/backend/internal/governance/domain"
	policyapplication "github.com/orz-i/Mender/backend/internal/policy/application"
	policydomain "github.com/orz-i/Mender/backend/internal/policy/domain"
	sandboxdomain "github.com/orz-i/Mender/backend/internal/sandbox/domain"
)

type gatePolicy struct {
	decision policydomain.Decision
	err      error
	received []policyapplication.EvaluateRequest
}

func (f *gatePolicy) Execute(_ context.Context, request policyapplication.EvaluateRequest) (policydomain.Decision, error) {
	f.received = append(f.received, request)
	return f.decision, f.err
}

type gateApproval struct {
	allowed  bool
	requests []governanceapplication.ApprovalRequest
}

func (f *gateApproval) Approve(_ context.Context, r governanceapplication.ApprovalRequest) (bool, error) {
	f.requests = append(f.requests, r)
	return f.allowed, nil
}

type gateAudit struct {
	records []AdmissionAudit
	err     error
}

func (f *gateAudit) RecordSandboxAdmission(_ context.Context, record AdmissionAudit) error {
	f.records = append(f.records, record)
	return f.err
}

type gateID struct{}

func (gateID) NewAdmissionID() (string, error) { return "sadm_test1", nil }

type gateClock struct{}

func (gateClock) Now() time.Time { return time.Date(2026, 10, 8, 0, 0, 0, 0, time.UTC) }
func setUpGate(t *testing.T) (gov.Context, Placement, *gatePolicy, *gateApproval, *gateAudit) {
	t.Helper()
	principal, err := gov.NewPrincipal(gov.PrincipalLocalUser, "owner")
	if err != nil {
		t.Fatal(err)
	}
	caller, err := gov.NewContext(gov.OriginCLI, principal, gov.CatalogScope())
	if err != nil {
		t.Fatal(err)
	}
	s, err := sandboxdomain.NewSandbox(sandboxdomain.Spec{ID: sandboxdomain.ID("sbx_0123456789abcdef0123456789abcdef"), Name: "sandbox", Backend: sandboxdomain.BackendLocalWorkspace, Isolation: sandboxdomain.IsolationRestrictedHost, LocalRoot: t.TempDir(), OwnerKind: "local_user", OwnerRef: "owner"})
	if err != nil {
		t.Fatal(err)
	}
	run, err := sandboxdomain.NewRun(s.ID(), 1, sandboxdomain.StateReady, "ready")
	if err != nil {
		t.Fatal(err)
	}
	return caller, Placement{Sandbox: s, Run: run, Required: sandboxdomain.CapabilityRead}, &gatePolicy{}, &gateApproval{}, &gateAudit{}
}
func TestSandboxAdmitHonorsGlobalDecisionAndRecordsBoundedFacts(t *testing.T) {
	caller, placement, p, auditApproval, audit := setUpGate(t)
	p.decision, _ = policydomain.Allow("allowed", "allowed")
	gate := NewAdmit(p, auditApproval, audit, gateID{}, gateClock{}, nil)
	if err := gate.AdmitAcquire(context.Background(), caller, placement); err != nil {
		t.Fatal(err)
	}
	if len(p.received) != 1 || p.received[0].Action != gov.ActionSandboxAcquire {
		t.Fatalf("wrong action %+v", p.received)
	}
	request := p.received[0]
	if request.ProviderID != "" || request.ToolID != "" || request.FactKnowledge != policydomain.FactKnown || len(request.Resources) != 3 {
		t.Fatalf("invalid policy scope: %+v", request)
	}
	if request.Resources[0].Kind != "sandbox_id" || request.Resources[0].Value != placement.Sandbox.ID().String() {
		t.Fatalf("missing sandbox ID: %+v", request.Resources)
	}
	for _, v := range request.Resources {
		if strings.Contains(v.Value, placement.Sandbox.Spec().LocalRoot) {
			t.Fatal("raw root leaked to Policy")
		}
	}
	if len(audit.records) != 1 || audit.records[0].Status != "admitted" || audit.records[0].PolicyKind != policydomain.DecisionAllow {
		t.Fatalf("audit %+v", audit.records)
	}
	if len(auditApproval.requests) != 0 {
		t.Fatal("allowed decision invoked approval")
	}
}
func TestSandboxAdmitDeniedAndApprovalFailClosed(t *testing.T) {
	for _, test := range []struct {
		name     string
		decision policydomain.Decision
		approval bool
		want     error
	}{
		{"policy deny", mustDeny(t), false, sandboxdomain.ErrNotAuthorized},
		{"approval denied", mustRequireApproval(t), false, ErrApprovalDenied},
	} {
		t.Run(test.name, func(t *testing.T) {
			caller, place, p, auditApproval, audit := setUpGate(t)
			p.decision = test.decision
			auditApproval.allowed = test.approval
			err := NewAdmit(p, auditApproval, audit, gateID{}, gateClock{}, nil).AdmitAcquire(context.Background(), caller, place)
			if !errors.Is(err, test.want) {
				t.Fatalf("denial: %v", err)
			}
			if len(audit.records) != 1 || audit.records[0].Status != "denied" {
				t.Fatalf("audit %+v", audit.records)
			}
		})
	}
}
func TestSandboxAuditWriteFailureStopsAdmission(t *testing.T) {
	caller, place, p, approval, audit := setUpGate(t)
	p.decision, _ = policydomain.Allow("allow", "allow")
	audit.err = errors.New("disk unavailable")
	if err := NewAdmit(p, approval, audit, gateID{}, gateClock{}, nil).AdmitAcquire(context.Background(), caller, place); err == nil {
		t.Fatal("missing audit failed open")
	}
}
func TestSandboxMissingGateDependenciesFailClosed(t *testing.T) {
	caller, place, _, _, _ := setUpGate(t)
	if err := NewAdmit(nil, nil, nil, nil, nil, nil).AdmitAcquire(context.Background(), caller, place); err == nil {
		t.Fatal("nil governance gate allowed")
	}
}
func mustDeny(t *testing.T) policydomain.Decision {
	t.Helper()
	v, err := policydomain.Deny("deny", "no")
	if err != nil {
		t.Fatal(err)
	}
	return v
}
func mustRequireApproval(t *testing.T) policydomain.Decision {
	t.Helper()
	v, err := policydomain.RequireApproval("approval", "needed")
	if err != nil {
		t.Fatal(err)
	}
	return v
}

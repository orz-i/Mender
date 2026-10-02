package application

import (
	"context"
	"errors"
	"testing"
	"time"

	auditdomain "github.com/orz-i/Mender/backend/internal/audit/domain"
	catalogdomain "github.com/orz-i/Mender/backend/internal/catalog/domain"
	governanceapplication "github.com/orz-i/Mender/backend/internal/governance/application"
	governancedomain "github.com/orz-i/Mender/backend/internal/governance/domain"
	policyapplication "github.com/orz-i/Mender/backend/internal/policy/application"
	policydomain "github.com/orz-i/Mender/backend/internal/policy/domain"
)

func TestAdmitDenialPreventsActivationOperation(t *testing.T) {
	t.Parallel()
	fixture := newAdmitFixture(t, policydomain.DecisionDeny)

	calls := 0
	err := fixture.useCase.Execute(
		context.Background(),
		fixture.request,
		func(context.Context) error {
			calls++
			return nil
		},
	)
	if !errors.Is(err, ErrPolicyDenied) {
		t.Fatalf("Execute() error = %v, want ErrPolicyDenied", err)
	}
	if calls != 0 {
		t.Fatalf("activation operation calls = %d, want 0", calls)
	}
	if fixture.policy.last.Action != governancedomain.ActionProviderActivate ||
		fixture.policy.last.ToolID != "" ||
		fixture.policy.last.FactKnowledge != "" {
		t.Fatalf("policy request = %#v", fixture.policy.last)
	}
	record := fixture.audit.last(t)
	if record.Status() != auditdomain.ProviderActivationDenied ||
		record.ErrorClass() != auditdomain.ProviderActivationErrorPolicy {
		t.Fatalf("audit = %#v", record)
	}
}

func TestAdmitApprovalRequiredFailsClosedBeforeOperation(t *testing.T) {
	t.Parallel()
	fixture := newAdmitFixture(t, policydomain.DecisionRequireApproval)
	fixture.approvals.approved = false

	calls := 0
	err := fixture.useCase.Execute(
		context.Background(),
		fixture.request,
		func(context.Context) error {
			calls++
			return nil
		},
	)
	if !errors.Is(err, ErrApprovalDenied) {
		t.Fatalf("Execute() error = %v, want ErrApprovalDenied", err)
	}
	if calls != 0 {
		t.Fatalf("activation operation calls = %d, want 0", calls)
	}
	if fixture.approvals.calls != 1 ||
		fixture.approvals.last.Action != governancedomain.ActionProviderActivate ||
		fixture.approvals.last.ToolID != nil {
		t.Fatalf("approval = %#v calls=%d", fixture.approvals.last, fixture.approvals.calls)
	}
}

func TestAdmitAllowExecutesExactlyOnceAndAudits(t *testing.T) {
	t.Parallel()
	fixture := newAdmitFixture(t, policydomain.DecisionAllow)

	calls := 0
	err := fixture.useCase.Execute(
		context.Background(),
		fixture.request,
		func(context.Context) error {
			calls++
			return nil
		},
	)
	if err != nil {
		t.Fatalf("Execute() error = %v", err)
	}
	if calls != 1 {
		t.Fatalf("activation operation calls = %d, want 1", calls)
	}
	record := fixture.audit.last(t)
	if record.Status() != auditdomain.ProviderActivationCompleted ||
		record.PrincipalKind() != string(governancedomain.PrincipalLocalUser) ||
		record.PrincipalRef() != "test:user" ||
		record.PolicyKind() != policydomain.DecisionAllow {
		t.Fatalf("audit = %#v", record)
	}
}

type admitFixture struct {
	useCase   Admit
	request   Request
	policy    *fakeActivationPolicy
	approvals *fakeActivationApproval
	audit     *fakeActivationAudit
}

func newAdmitFixture(t *testing.T, decision policydomain.DecisionKind) admitFixture {
	t.Helper()
	providerID, err := catalogdomain.ParseProviderID("prov_00000000000000000000000000000001")
	if err != nil {
		t.Fatal(err)
	}
	principal, err := governancedomain.NewPrincipal(
		governancedomain.PrincipalLocalUser,
		"test:user",
	)
	if err != nil {
		t.Fatal(err)
	}
	governanceContext, err := governancedomain.NewContext(
		governancedomain.OriginCLI,
		principal,
		governancedomain.CatalogScope(),
	)
	if err != nil {
		t.Fatal(err)
	}
	policy := &fakeActivationPolicy{decision: mustDecision(t, decision)}
	approvals := &fakeActivationApproval{}
	audit := &fakeActivationAudit{}
	useCase := NewAdmit(
		policy,
		approvals,
		audit,
		fixedActivationIDs{},
		fixedActivationClock{now: time.Date(2026, 10, 2, 1, 0, 0, 0, time.UTC)},
	)
	return admitFixture{
		useCase:   useCase,
		request:   Request{ProviderID: providerID, Context: governanceContext},
		policy:    policy,
		approvals: approvals,
		audit:     audit,
	}
}

func mustDecision(t *testing.T, kind policydomain.DecisionKind) policydomain.Decision {
	t.Helper()
	var (
		decision policydomain.Decision
		err      error
	)
	switch kind {
	case policydomain.DecisionAllow:
		decision, err = policydomain.Allow("test.reason", "test")
	case policydomain.DecisionDeny:
		decision, err = policydomain.Deny("test.reason", "test")
	case policydomain.DecisionRequireApproval:
		decision, err = policydomain.RequireApproval("test.reason", "test")
	default:
		t.Fatalf("unsupported test decision %q", kind)
	}
	if err != nil {
		t.Fatal(err)
	}
	return decision
}

type fakeActivationPolicy struct {
	decision policydomain.Decision
	last     policyapplication.EvaluateRequest
}

func (f *fakeActivationPolicy) Execute(
	_ context.Context,
	request policyapplication.EvaluateRequest,
) (policydomain.Decision, error) {
	f.last = request
	return f.decision, nil
}

type fakeActivationApproval struct {
	calls    int
	approved bool
	last     governanceapplication.ApprovalRequest
}

func (f *fakeActivationApproval) Approve(
	_ context.Context,
	request governanceapplication.ApprovalRequest,
) (bool, error) {
	f.calls++
	f.last = request
	return f.approved, nil
}

type fakeActivationAudit struct {
	records []auditdomain.ProviderActivationRecord
}

func (f *fakeActivationAudit) RecordProviderActivation(
	_ context.Context,
	record auditdomain.ProviderActivationRecord,
) error {
	f.records = append(f.records, record)
	return nil
}

func (f *fakeActivationAudit) last(t *testing.T) auditdomain.ProviderActivationRecord {
	t.Helper()
	if len(f.records) == 0 {
		t.Fatal("no activation audit records")
	}
	return f.records[len(f.records)-1]
}

type fixedActivationIDs struct{}

func (fixedActivationIDs) NewActivationID() (string, error) {
	return "act_00000000000000000000000000000000", nil
}

type fixedActivationClock struct {
	now time.Time
}

func (c fixedActivationClock) Now() time.Time {
	return c.now
}

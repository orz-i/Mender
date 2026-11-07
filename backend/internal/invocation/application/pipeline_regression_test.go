package application

import (
	"context"
	"errors"
	"reflect"
	"testing"
	"time"

	auditdomain "github.com/orz-i/Mender/backend/internal/audit/domain"
	catalogapplication "github.com/orz-i/Mender/backend/internal/catalog/application"
	catalogdomain "github.com/orz-i/Mender/backend/internal/catalog/domain"
	governanceapplication "github.com/orz-i/Mender/backend/internal/governance/application"
	governancedomain "github.com/orz-i/Mender/backend/internal/governance/domain"
	invocationdomain "github.com/orz-i/Mender/backend/internal/invocation/domain"
	policydomain "github.com/orz-i/Mender/backend/internal/policy/domain"
	runtimegovernancedomain "github.com/orz-i/Mender/backend/internal/runtimegovernance/domain"
)

func TestInvocationPortOrderRetainsAdmissionSnapshotAndPostflightBoundary(t *testing.T) {
	t.Parallel()
	ports, invoke, request := tracedInvocation(t)
	ports.decision, _ = policydomain.RequireApproval("test.approval", "approval required")
	resource, err := invocationdomain.NewResourceFact("sandbox_id", "reviewed-target")
	if err != nil {
		t.Fatal(err)
	}
	ports.facts, err = invocationdomain.KnownGovernanceFacts([]invocationdomain.EffectFact{"fs.read"}, []invocationdomain.ResourceFact{resource})
	if err != nil {
		t.Fatal(err)
	}
	response, err := invoke.Execute(context.Background(), request)
	if err != nil || response.Outcome.Kind() != invocationdomain.OutcomeKindComplete {
		t.Fatalf("Execute() = %+v, %v", response, err)
	}
	want := []string{"snapshot", "scope", "tool", "arguments", "facts", "policy", "approval",
		"budget.acquire", "binding.acquire", "route", "executor", "binding.release", "budget.release", "result", "audit", "terminal"}
	if !reflect.DeepEqual(ports.order, want) {
		t.Fatalf("port order = %v, want %v", ports.order, want)
	}
	if ports.snapshotCalls != 1 || !reflect.DeepEqual(ports.acquiredBudget, ports.budget) || ports.resultMaxBytes != ports.budget.MaxResultBytes() {
		t.Fatalf("budget was not retained: snapshots=%d acquired=%+v resultMax=%d", ports.snapshotCalls, ports.acquiredBudget.Spec(), ports.resultMaxBytes)
	}
	if !reflect.DeepEqual(ports.policyRequest.Facts, ports.facts) || !reflect.DeepEqual(ports.executionRequest.AdmittedFacts, ports.policyRequest.Facts) {
		t.Fatal("executor did not receive the exact reviewed Policy facts")
	}
	if ports.approvalHadDeadline || !ports.executionHadDeadline || ports.executionCancelledAtAudit {
		t.Fatalf("execution deadline moved across approval/audit: approval=%v execution=%v cancelledAtAudit=%v",
			ports.approvalHadDeadline, ports.executionHadDeadline, ports.executionCancelledAtAudit)
	}
	if !errors.Is(ports.executionContext.Err(), context.Canceled) {
		t.Fatal("execution context was not closed after audit")
	}
	if ports.approvalRequest.SourceRef != ports.tool.SourceRef() || ports.approvalRequest.ToolID == nil || *ports.approvalRequest.ToolID != ports.tool.ID() {
		t.Fatalf("approval lost exact Tool provenance: %+v", ports.approvalRequest)
	}
	if ports.record.InvocationID() != response.InvocationID || ports.executionRequest.InvocationID != response.InvocationID ||
		ports.record.Approval() != auditdomain.ApprovalApproved || ports.record.ToolID() != ports.tool.ID() {
		t.Fatalf("attempt identity/admission changed at finalization: %+v", ports.record)
	}
}

func TestInvocationFailurePathsReleaseExactlyAcquiredLeases(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		stage        string
		wantBudget   int
		wantBinding  int
		wantInternal bool
	}{
		{"budget", 0, 0, false}, {"budget_nil", 0, 0, true},
		{"binding", 1, 0, true}, {"binding_nil", 1, 0, true},
		{"route", 1, 1, false}, {"executor", 1, 1, false},
		{"result", 1, 1, true}, {"audit", 1, 1, true},
	} {
		t.Run(tc.stage, func(t *testing.T) {
			ports, invoke, request := tracedInvocation(t)
			ports.failAt = tc.stage
			_, err := invoke.Execute(context.Background(), request)
			if (err != nil) != tc.wantInternal {
				t.Fatalf("error = %v, wantInternal=%v", err, tc.wantInternal)
			}
			if ports.budgetReleases != tc.wantBudget || ports.bindingReleases != tc.wantBinding {
				t.Fatalf("released budget/binding = %d/%d, want %d/%d; order=%v",
					ports.budgetReleases, ports.bindingReleases, tc.wantBudget, tc.wantBinding, ports.order)
			}
			if ports.auditCalls != 1 {
				t.Fatalf("terminal audit attempts=%d, want 1", ports.auditCalls)
			}
		})
	}
}

func TestInvocationAuditFailureCannotPublishSuccessfulOutcomeOrTerminalEvent(t *testing.T) {
	t.Parallel()
	ports, invoke, request := tracedInvocation(t)
	ports.failAt = "audit"
	response, err := invoke.Execute(context.Background(), request)
	if !errors.Is(err, ErrAuditFailed) || !errors.Is(err, errTracedInvocation) {
		t.Fatalf("audit error = %v", err)
	}
	if response.InvocationID == "" || response.Outcome.Kind() != "" {
		t.Fatalf("unaudited success escaped: %+v", response)
	}
	for _, event := range ports.events {
		if event.Kind() == invocationdomain.EventTerminal {
			t.Fatalf("unaudited success emitted terminal event: %+v", event)
		}
	}
}

func TestInvocationApprovalAuditRetainsParentCancellation(t *testing.T) {
	t.Parallel()
	for _, fail := range []bool{false, true} {
		t.Run(map[bool]string{false: "denied", true: "error"}[fail], func(t *testing.T) {
			ports, invoke, request := tracedInvocation(t)
			ports.decision, _ = policydomain.RequireApproval("test.approval", "approval required")
			ports.approved = false
			if fail {
				ports.failAt = "approval"
			}
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			ports.onApproval = cancel
			_, err := invoke.Execute(ctx, request)
			if (err != nil) != fail || !errors.Is(ports.auditContextErr, context.Canceled) {
				t.Fatalf("approval/audit context semantics changed: err=%v auditContextErr=%v", err, ports.auditContextErr)
			}
			if ports.budgetReleases != 0 || ports.bindingReleases != 0 || ports.record.Approval() != auditdomain.ApprovalDenied {
				t.Fatalf("approval denial reached execution: order=%v record=%+v", ports.order, ports.record)
			}
		})
	}
}

func TestInvocationExecutionFailureAuditsWithoutParentCancellation(t *testing.T) {
	t.Parallel()
	ports, invoke, request := tracedInvocation(t)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	ports.onExecute, ports.failAt = cancel, "executor"
	response, err := invoke.Execute(ctx, request)
	if err != nil || response.Outcome.FailureCode() != invocationdomain.FailureCancelled {
		t.Fatalf("cancelled execution = %+v, %v", response, err)
	}
	if ports.auditContextErr != nil || !ports.executionCancelledAtAudit || ports.record.Status() != auditdomain.StatusCancelled {
		t.Fatalf("execution/audit cancellation changed: auditContextErr=%v executionCancelled=%v record=%+v",
			ports.auditContextErr, ports.executionCancelledAtAudit, ports.record)
	}
}

func TestInvocationScopeDenialAuditsBeforeScopeResolvedEvent(t *testing.T) {
	t.Parallel()
	ports, invoke, request := tracedInvocation(t)
	ports.scopeFound = false
	_, err := invoke.Execute(context.Background(), request)
	if err != nil {
		t.Fatal(err)
	}
	want := []string{"snapshot", "scope", "audit", "scope.resolved", "terminal"}
	if !reflect.DeepEqual(ports.order, want) {
		t.Fatalf("scope denial order=%v, want %v", ports.order, want)
	}
	if ports.record.ProviderID() != nil || ports.record.RejectionCode() != invocationdomain.RejectionToolOutsideScope {
		t.Fatalf("scope denial leaked resolved Tool metadata: %+v", ports.record)
	}
}

func TestInvocationAuditUsesResolvedToolIdentityOnlyAfterLookup(t *testing.T) {
	t.Parallel()
	for _, unresolved := range []bool{false, true} {
		t.Run(map[bool]string{false: "resolved", true: "unresolved"}[unresolved], func(t *testing.T) {
			ports, invoke, request := tracedInvocation(t)
			requested, err := catalogdomain.ParseToolID("tool_00000000000000000000000000000099")
			if err != nil {
				t.Fatal(err)
			}
			request.ToolID = requested
			if unresolved {
				ports.failAt = "tool"
			}
			if _, err := invoke.Execute(context.Background(), request); err != nil {
				t.Fatal(err)
			}
			want := ports.tool.ID()
			if unresolved {
				want = requested
			}
			if ports.record.ToolID() != want || (ports.record.ProviderID() == nil) != unresolved {
				t.Fatalf("audit identity = tool %s provider %v, want tool %s unresolved=%v",
					ports.record.ToolID(), ports.record.ProviderID(), want, unresolved)
			}
		})
	}
}

var errTracedInvocation = errors.New("traced port failure")

// tracedInvocationPorts is one observable set of consumer ports. It checks
// admission and lease boundaries without executing provider/network/filesystem
// mechanisms, which remain covered by their existing adapter tests.
type tracedInvocationPorts struct {
	tool                      catalogdomain.Tool
	budget                    runtimegovernancedomain.Budget
	facts                     invocationdomain.GovernanceFacts
	decision                  policydomain.Decision
	outcome                   invocationdomain.Outcome
	approved                  bool
	scopeFound                bool
	failAt                    string
	onApproval                func()
	onExecute                 func()
	onExecuteCtx              func(context.Context, ExecutionRequest)
	order                     []string
	events                    []invocationdomain.Event
	snapshotCalls             int
	budgetReleases            int
	bindingReleases           int
	auditCalls                int
	acquiredBudget            runtimegovernancedomain.Budget
	resultMaxBytes            int
	policyRequest             PolicyRequest
	approvalRequest           governanceapplication.ApprovalRequest
	executionRequest          ExecutionRequest
	record                    auditdomain.Record
	approvalHadDeadline       bool
	executionHadDeadline      bool
	executionCancelledAtAudit bool
	executionContext          context.Context
	auditContextErr           error
}

func tracedInvocation(t *testing.T) (*tracedInvocationPorts, InvokeTool, InvokeToolRequest) {
	t.Helper()
	fixture := newFixture(t)
	ports := &tracedInvocationPorts{tool: fixture.tool, budget: fixture.runtimeBudget.snapshot,
		facts: fixture.facts.facts, decision: fixture.policy.decision, outcome: fixture.executor.result,
		approved: true, scopeFound: true}
	invoke := NewInvokeTool(ports, ports, ports, ports, ports, ports, ports, ports, ports,
		ports, ports, nil, ports, fixture.useCase.ids, fixture.useCase.clock, time.Minute)
	request := InvokeToolRequest{ToolID: ports.tool.ID(), Arguments: []byte(`{"message":"governed"}`),
		Context: profileInvocationContext(t, "mcp:trace", "prof_00000000000000000000000000000002"), Timeout: time.Minute}
	return ports, invoke, request
}

func (p *tracedInvocationPorts) Snapshot(context.Context) (runtimegovernancedomain.Budget, error) {
	p.order = append(p.order, "snapshot")
	p.snapshotCalls++
	return p.budget, nil
}

func (p *tracedInvocationPorts) ResolveBindingGovernance(context.Context, string, catalogdomain.ToolID) (invocationdomain.ScopedBindingGovernance, bool, error) {
	p.order = append(p.order, "scope")
	return invocationdomain.InheritedScopedBindingGovernance(), p.scopeFound, nil
}

func (p *tracedInvocationPorts) GetTool(context.Context, catalogdomain.ToolID) (catalogdomain.Tool, error) {
	p.order = append(p.order, "tool")
	if p.failAt == "tool" {
		return catalogdomain.Tool{}, catalogapplication.ErrToolNotFound
	}
	return p.tool, nil
}

func (p *tracedInvocationPorts) Validate(context.Context, catalogdomain.Schema, invocationdomain.JSONValue) (bool, error) {
	p.order = append(p.order, "arguments")
	return true, nil
}

func (p *tracedInvocationPorts) Extract(context.Context, catalogdomain.Tool, invocationdomain.JSONValue, governancedomain.Context) (GovernanceFactExtraction, error) {
	p.order = append(p.order, "facts")
	return GovernanceFactExtraction{Facts: p.facts}, nil
}

func (p *tracedInvocationPorts) Evaluate(_ context.Context, request PolicyRequest) (policydomain.Decision, error) {
	p.order = append(p.order, "policy")
	p.policyRequest = request
	return p.decision, nil
}

func (p *tracedInvocationPorts) Approve(ctx context.Context, request governanceapplication.ApprovalRequest) (bool, error) {
	p.order = append(p.order, "approval")
	p.approvalRequest = request
	_, p.approvalHadDeadline = ctx.Deadline()
	if p.onApproval != nil {
		p.onApproval()
	}
	if p.failAt == "approval" {
		return false, errTracedInvocation
	}
	return p.approved, nil
}

func (p *tracedInvocationPorts) AcquireInvocation(ctx context.Context, budget runtimegovernancedomain.Budget, _, _, _ string) (func(), error) {
	p.order = append(p.order, "budget.acquire")
	p.acquiredBudget = budget
	p.executionContext = ctx
	_, p.executionHadDeadline = ctx.Deadline()
	if p.failAt == "budget" {
		return nil, runtimegovernancedomain.ErrBudgetExceeded
	}
	if p.failAt == "budget_nil" {
		return nil, nil
	}
	return func() { p.order = append(p.order, "budget.release"); p.budgetReleases++ }, nil
}

func (p *tracedInvocationPorts) Acquire(context.Context, BindingExecutionGuardRequest) (func(), error) {
	p.order = append(p.order, "binding.acquire")
	if p.failAt == "binding" {
		return nil, errTracedInvocation
	}
	if p.failAt == "binding_nil" {
		return nil, nil
	}
	return func() { p.order = append(p.order, "binding.release"); p.bindingReleases++ }, nil
}

func (p *tracedInvocationPorts) ExecutorFor(context.Context, catalogdomain.Tool) (ToolExecutor, error) {
	p.order = append(p.order, "route")
	if p.failAt == "route" {
		return nil, ErrExecutorNotFound
	}
	return p, nil
}

func (p *tracedInvocationPorts) Execute(ctx context.Context, request ExecutionRequest, _ CredentialResolver, _ ProgressReporter) (invocationdomain.Outcome, error) {
	p.order = append(p.order, "executor")
	p.executionRequest = request
	if p.onExecuteCtx != nil {
		p.onExecuteCtx(ctx, request)
	}
	if p.onExecute != nil {
		p.onExecute()
	}
	if p.failAt == "executor" {
		return invocationdomain.Outcome{}, errTracedInvocation
	}
	return p.outcome, nil
}

func (p *tracedInvocationPorts) Guard(_ context.Context, _ catalogdomain.Tool, outcome invocationdomain.Outcome, maxBytes int) (invocationdomain.Outcome, error) {
	p.order = append(p.order, "result")
	p.resultMaxBytes = maxBytes
	if p.failAt == "result" {
		return invocationdomain.Outcome{}, errTracedInvocation
	}
	return outcome, nil
}

func (p *tracedInvocationPorts) Record(ctx context.Context, record auditdomain.Record) error {
	p.order = append(p.order, "audit")
	p.auditCalls++
	p.record = record
	p.auditContextErr = ctx.Err()
	p.executionCancelledAtAudit = p.executionContext != nil && p.executionContext.Err() != nil
	if p.failAt == "audit" {
		return errTracedInvocation
	}
	return nil
}

func (p *tracedInvocationPorts) Observe(event invocationdomain.Event) {
	p.events = append(p.events, event)
	if event.Kind() == invocationdomain.EventTerminal {
		p.order = append(p.order, "terminal")
	}
	if !p.scopeFound && event.Kind() == invocationdomain.EventScopeResolved {
		p.order = append(p.order, "scope.resolved")
	}
}

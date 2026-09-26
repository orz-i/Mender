package application

import (
	"bytes"
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	auditdomain "github.com/orz-i/Mender/backend/internal/audit/domain"
	catalogapplication "github.com/orz-i/Mender/backend/internal/catalog/application"
	catalogdomain "github.com/orz-i/Mender/backend/internal/catalog/domain"
	credentialdomain "github.com/orz-i/Mender/backend/internal/credential/domain"
	invocationdomain "github.com/orz-i/Mender/backend/internal/invocation/domain"
	policydomain "github.com/orz-i/Mender/backend/internal/policy/domain"
)

func TestInvokeToolSuccessfulResultVariants(t *testing.T) {
	t.Parallel()

	content, _ := invocationdomain.NewJSONValue([]byte(`{"ok":true}`))
	complete, _ := invocationdomain.Complete(content)
	inputRequired, _ := invocationdomain.InputRequired(
		"req-1",
		"Choose a target",
		[]byte(`{"type":"string"}`),
	)
	task, _ := invocationdomain.Task("task-1", "running", "")

	tests := []struct {
		name       string
		result     invocationdomain.Result
		wantStatus auditdomain.Status
	}{
		{"complete", complete, auditdomain.StatusCompleted},
		{"input required", inputRequired, auditdomain.StatusInputRequired},
		{"task", task, auditdomain.StatusTask},
	}

	for _, tc := range tests {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			fixture := newFixture(t)
			fixture.executor.result = tc.result

			response, err := fixture.useCase.Execute(context.Background(), InvokeToolRequest{
				ToolID:    fixture.tool.ID(),
				Arguments: []byte(`{"input":"value"}`),
				Caller:    "test",
			})
			if err != nil {
				t.Fatalf("Execute() error = %v", err)
			}
			if response.Result.Kind() != tc.result.Kind() {
				t.Fatalf("Result kind = %q, want %q", response.Result.Kind(), tc.result.Kind())
			}
			record := fixture.audit.last(t)
			if record.Status() != tc.wantStatus || record.ResultKind() != tc.result.Kind() {
				t.Fatalf("Audit record = %#v", record)
			}
		})
	}
}

func TestToolLookupFailureIsAuditedWithoutProvider(t *testing.T) {
	t.Parallel()

	fixture := newFixture(t)
	fixture.useCase.tools = fakeToolLookup{err: catalogapplication.ErrToolNotFound}

	_, err := fixture.useCase.Execute(context.Background(), InvokeToolRequest{
		ToolID:    fixture.tool.ID(),
		Arguments: []byte(`{}`),
	})
	if !errors.Is(err, catalogapplication.ErrToolNotFound) {
		t.Fatalf("Execute() error = %v, want catalog ErrToolNotFound", err)
	}

	record := fixture.audit.last(t)
	if record.ProviderID() != nil {
		t.Fatalf("ProviderID() = %v, want nil", record.ProviderID())
	}
	if record.ErrorClass() != auditdomain.ErrorToolNotFound {
		t.Fatalf("ErrorClass() = %q, want %q", record.ErrorClass(), auditdomain.ErrorToolNotFound)
	}
}

func TestPolicyDenyPreventsExecution(t *testing.T) {
	t.Parallel()

	fixture := newFixture(t)
	fixture.policy.decision, _ = policydomain.Deny("test.denied", "denied by test")

	_, err := fixture.useCase.Execute(context.Background(), InvokeToolRequest{
		ToolID:    fixture.tool.ID(),
		Arguments: []byte(`{}`),
	})
	if !errors.Is(err, ErrPolicyDenied) {
		t.Fatalf("Execute() error = %v, want ErrPolicyDenied", err)
	}
	if fixture.executor.calls != 0 {
		t.Fatalf("executor calls = %d, want 0", fixture.executor.calls)
	}
	if fixture.approvals.calls != 0 {
		t.Fatalf("approval calls = %d, want 0", fixture.approvals.calls)
	}

	record := fixture.audit.last(t)
	if record.Status() != auditdomain.StatusDenied || record.ErrorClass() != auditdomain.ErrorPolicy {
		t.Fatalf("Audit record = %#v", record)
	}
}

func TestRequireApprovalCallsGateAndDenialPreventsExecution(t *testing.T) {
	t.Parallel()

	fixture := newFixture(t)
	fixture.policy.decision, _ = policydomain.RequireApproval("risk.write", "write operation")
	fixture.approvals.approved = false

	_, err := fixture.useCase.Execute(context.Background(), InvokeToolRequest{
		ToolID:    fixture.tool.ID(),
		Arguments: []byte(`{}`),
		Caller:    "agent",
	})
	if !errors.Is(err, ErrApprovalDenied) {
		t.Fatalf("Execute() error = %v, want ErrApprovalDenied", err)
	}
	if fixture.approvals.calls != 1 {
		t.Fatalf("approval calls = %d, want 1", fixture.approvals.calls)
	}
	if fixture.executor.calls != 0 {
		t.Fatalf("executor calls = %d, want 0", fixture.executor.calls)
	}

	record := fixture.audit.last(t)
	if record.Approval() != auditdomain.ApprovalDenied || record.Status() != auditdomain.StatusDenied {
		t.Fatalf("Audit record = %#v", record)
	}
}

func TestCredentialResolverIsConsumedOnlyByExecutor(t *testing.T) {
	t.Parallel()

	fixture := newFixture(t)
	fixture.credentials.secret = []byte("TOP-SECRET-MATERIAL")
	fixture.executor.resolveCredential = true

	response, err := fixture.useCase.Execute(context.Background(), InvokeToolRequest{
		ToolID:    fixture.tool.ID(),
		Arguments: []byte(`{"credential":"credential://github/work"}`),
	})
	if err != nil {
		t.Fatalf("Execute() error = %v", err)
	}
	if response.Result.Kind() != invocationdomain.ResultKindComplete {
		t.Fatalf("Result kind = %q", response.Result.Kind())
	}
	if fixture.credentials.calls != 1 {
		t.Fatalf("credential resolver calls = %d, want 1", fixture.credentials.calls)
	}
	if fixture.policy.sawArguments {
		t.Fatal("policy evaluator received arguments")
	}

	record := fixture.audit.last(t)
	if bytes.Contains([]byte(record.Caller()), fixture.credentials.secret) ||
		bytes.Contains([]byte(record.PolicyReason()), fixture.credentials.secret) {
		t.Fatal("audit record exposed secret material")
	}
}

func TestParentCancellationIsAudited(t *testing.T) {
	t.Parallel()

	fixture := newFixture(t)
	fixture.executor.blockUntilDone = true

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	_, err := fixture.useCase.Execute(ctx, InvokeToolRequest{
		ToolID:    fixture.tool.ID(),
		Arguments: []byte(`{}`),
	})
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("Execute() error = %v, want context.Canceled", err)
	}

	record := fixture.audit.last(t)
	if record.Status() != auditdomain.StatusCancelled || record.ErrorClass() != auditdomain.ErrorCancelled {
		t.Fatalf("Audit record = %#v", record)
	}
}

func TestTimeoutIsAudited(t *testing.T) {
	t.Parallel()

	fixture := newFixture(t)
	fixture.executor.blockUntilDone = true

	_, err := fixture.useCase.Execute(context.Background(), InvokeToolRequest{
		ToolID:    fixture.tool.ID(),
		Arguments: []byte(`{}`),
		Timeout:   time.Millisecond,
	})
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("Execute() error = %v, want context.DeadlineExceeded", err)
	}

	record := fixture.audit.last(t)
	if record.Status() != auditdomain.StatusTimedOut || record.ErrorClass() != auditdomain.ErrorTimedOut {
		t.Fatalf("Audit record = %#v", record)
	}
}

type fixture struct {
	useCase     InvokeTool
	tool        catalogdomain.Tool
	policy      *fakePolicy
	approvals   *fakeApproval
	executor    *fakeExecutor
	credentials *fakeCredentials
	audit       *memoryAudit
}

func newFixture(t *testing.T) fixture {
	t.Helper()

	tool := testTool(t)
	content, _ := invocationdomain.NewJSONValue([]byte(`{"ok":true}`))
	result, _ := invocationdomain.Complete(content)
	allow, _ := policydomain.Allow("local.default", "")

	policy := &fakePolicy{decision: allow}
	approvals := &fakeApproval{approved: true}
	executor := &fakeExecutor{result: result}
	credentials := &fakeCredentials{}
	audit := &memoryAudit{}
	ids := &fixedInvocationIDs{
		ids: []invocationdomain.InvocationID{
			mustInvocationID(t, "inv_00000000000000000000000000000001"),
		},
	}
	clock := &monotonicClock{
		next: time.Date(2026, 9, 25, 12, 0, 0, 0, time.UTC),
		step: time.Millisecond,
	}

	useCase := NewInvokeTool(
		fakeToolLookup{tool: tool},
		policy,
		approvals,
		fakeRouter{executor: executor},
		credentials,
		audit,
		ids,
		clock,
		time.Second,
	)

	return fixture{
		useCase:     useCase,
		tool:        tool,
		policy:      policy,
		approvals:   approvals,
		executor:    executor,
		credentials: credentials,
		audit:       audit,
	}
}

type fakeToolLookup struct {
	tool catalogdomain.Tool
	err  error
}

func (f fakeToolLookup) GetTool(
	context.Context,
	catalogdomain.ToolID,
) (catalogdomain.Tool, error) {
	return f.tool, f.err
}

type fakePolicy struct {
	decision     policydomain.Decision
	err          error
	sawArguments bool
}

func (f *fakePolicy) Evaluate(
	_ context.Context,
	request PolicyRequest,
) (policydomain.Decision, error) {
	if request.Tool.ID().String() == "" {
		f.sawArguments = true
	}
	return f.decision, f.err
}

type fakeApproval struct {
	approved bool
	err      error
	calls    int
}

func (f *fakeApproval) Approve(context.Context, ApprovalRequest) (bool, error) {
	f.calls++
	return f.approved, f.err
}

type fakeRouter struct {
	executor ToolExecutor
	err      error
}

func (f fakeRouter) ExecutorFor(context.Context, catalogdomain.Tool) (ToolExecutor, error) {
	return f.executor, f.err
}

type fakeExecutor struct {
	result            invocationdomain.Result
	err               error
	calls             int
	resolveCredential bool
	blockUntilDone    bool
}

func (f *fakeExecutor) Execute(
	ctx context.Context,
	_ ExecutionRequest,
	credentials CredentialResolver,
) (invocationdomain.Result, error) {
	f.calls++
	if f.blockUntilDone {
		<-ctx.Done()
		return invocationdomain.Result{}, ctx.Err()
	}
	if f.resolveCredential {
		ref, err := credentialdomain.ParseReference("credential://github/work")
		if err != nil {
			return invocationdomain.Result{}, err
		}
		secret, err := credentials.Resolve(ctx, ref)
		if err != nil {
			return invocationdomain.Result{}, err
		}
		for i := range secret {
			secret[i] = 0
		}
	}
	return f.result, f.err
}

type fakeCredentials struct {
	secret []byte
	calls  int
}

func (f *fakeCredentials) Resolve(
	context.Context,
	credentialdomain.Reference,
) ([]byte, error) {
	f.calls++
	return bytes.Clone(f.secret), nil
}

type memoryAudit struct {
	mu      sync.Mutex
	records []auditdomain.Record
}

func (m *memoryAudit) Record(_ context.Context, record auditdomain.Record) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.records = append(m.records, record)
	return nil
}

func (m *memoryAudit) last(t *testing.T) auditdomain.Record {
	t.Helper()

	m.mu.Lock()
	defer m.mu.Unlock()
	if len(m.records) == 0 {
		t.Fatal("no audit records")
	}
	return m.records[len(m.records)-1]
}

type fixedInvocationIDs struct {
	ids []invocationdomain.InvocationID
}

func (f *fixedInvocationIDs) NewInvocationID() (invocationdomain.InvocationID, error) {
	if len(f.ids) == 0 {
		return "", errors.New("no invocation IDs remaining")
	}
	id := f.ids[0]
	f.ids = f.ids[1:]
	return id, nil
}

type monotonicClock struct {
	mu   sync.Mutex
	next time.Time
	step time.Duration
}

func (c *monotonicClock) Now() time.Time {
	c.mu.Lock()
	defer c.mu.Unlock()
	current := c.next
	c.next = c.next.Add(c.step)
	return current
}

func testTool(t *testing.T) catalogdomain.Tool {
	t.Helper()

	toolID, _ := catalogdomain.ParseToolID("tool_00000000000000000000000000000001")
	providerID, _ := catalogdomain.ParseProviderID("prov_00000000000000000000000000000001")
	schema, _ := catalogdomain.NewSchema([]byte(`{"type":"object"}`))
	tool, err := catalogdomain.NewTool(
		toolID,
		providerID,
		catalogdomain.ToolSpec{
			SourceKey:   "echo",
			Name:        "echo",
			Title:       "Echo",
			InputSchema: schema,
		},
		time.Date(2026, 9, 25, 10, 0, 0, 0, time.UTC),
	)
	if err != nil {
		t.Fatal(err)
	}
	return tool
}

func mustInvocationID(t *testing.T, raw string) invocationdomain.InvocationID {
	t.Helper()
	id, err := invocationdomain.ParseInvocationID(raw)
	if err != nil {
		t.Fatal(err)
	}
	return id
}

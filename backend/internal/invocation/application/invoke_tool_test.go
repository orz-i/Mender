package application

import (
	"bytes"
	"context"
	"errors"
	"slices"
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
		result     invocationdomain.Outcome
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
				Context:   catalogInvocationContext(t, "test"),
			})
			if err != nil {
				t.Fatalf("Execute() error = %v", err)
			}
			if response.Outcome.Kind() != tc.result.Kind() {
				t.Fatalf("Result kind = %q, want %q", response.Outcome.Kind(), tc.result.Kind())
			}
			record := fixture.audit.last(t)
			if record.Status() != tc.wantStatus || record.ResultKind() != tc.result.Kind() {
				t.Fatalf("Audit record = %#v", record)
			}
		})
	}
}

func TestInvokeToolRejectsMissingInvocationContextBeforeLookup(t *testing.T) {
	t.Parallel()

	fixture := newFixture(t)

	_, err := fixture.useCase.Execute(context.Background(), InvokeToolRequest{
		ToolID:    fixture.tool.ID(),
		Arguments: []byte(`{}`),
	})
	if !errors.Is(err, ErrInvalidInvocationContext) {
		t.Fatalf("Execute() error = %v, want ErrInvalidInvocationContext", err)
	}
	if fixture.tools.calls != 0 {
		t.Fatalf("tool lookup calls = %d, want 0", fixture.tools.calls)
	}
	if fixture.profileScopes.calls != 0 {
		t.Fatalf("profile scope calls = %d, want 0", fixture.profileScopes.calls)
	}
	if fixture.executor.calls != 0 {
		t.Fatalf("executor calls = %d, want 0", fixture.executor.calls)
	}
}

func TestCatalogScopeDoesNotResolveProfileMembership(t *testing.T) {
	t.Parallel()

	fixture := newFixture(t)

	_, err := fixture.useCase.Execute(context.Background(), InvokeToolRequest{
		ToolID:    fixture.tool.ID(),
		Arguments: []byte(`{}`),
		Context:   catalogInvocationContext(t, "catalog-test"),
	})
	if err != nil {
		t.Fatalf("Execute() error = %v", err)
	}
	if fixture.profileScopes.calls != 0 {
		t.Fatalf("profile scope calls = %d, want 0", fixture.profileScopes.calls)
	}
	if fixture.tools.calls != 1 || fixture.executor.calls != 1 {
		t.Fatalf("tool/executor calls = %d/%d, want 1/1", fixture.tools.calls, fixture.executor.calls)
	}
}

func TestInvalidJSONIsRejectedAfterToolResolutionBeforePolicy(t *testing.T) {
	t.Parallel()

	fixture := newFixture(t)

	response, err := fixture.useCase.Execute(context.Background(), InvokeToolRequest{
		ToolID:    fixture.tool.ID(),
		Arguments: []byte(`{`),
		Context:   catalogInvocationContext(t, ""),
	})
	if err != nil {
		t.Fatalf("Execute() error = %v", err)
	}
	if response.Outcome.Kind() != invocationdomain.OutcomeKindRejected ||
		response.Outcome.RejectionCode() != invocationdomain.RejectionInvalidArguments {
		t.Fatalf(
			"Outcome = kind %q code %q",
			response.Outcome.Kind(),
			response.Outcome.RejectionCode(),
		)
	}
	if fixture.tools.calls != 1 {
		t.Fatalf("tool lookup calls = %d, want 1", fixture.tools.calls)
	}
	if fixture.arguments.calls != 0 ||
		fixture.facts.calls != 0 ||
		fixture.policy.calls != 0 ||
		fixture.executor.calls != 0 {
		t.Fatalf(
			"preflight/policy/executor calls = validator %d facts %d policy %d executor %d",
			fixture.arguments.calls,
			fixture.facts.calls,
			fixture.policy.calls,
			fixture.executor.calls,
		)
	}

	record := fixture.audit.last(t)
	if record.ErrorClass() != auditdomain.ErrorInvalidArguments {
		t.Fatalf("ErrorClass() = %q", record.ErrorClass())
	}
}

func TestSchemaMismatchIsRejectedBeforeFactsPolicyAndExecution(t *testing.T) {
	t.Parallel()

	fixture := newFixture(t)
	fixture.arguments.valid = false

	response, err := fixture.useCase.Execute(context.Background(), InvokeToolRequest{
		ToolID:    fixture.tool.ID(),
		Arguments: []byte(`{}`),
		Context:   catalogInvocationContext(t, ""),
	})
	if err != nil {
		t.Fatalf("Execute() error = %v", err)
	}
	if response.Outcome.Kind() != invocationdomain.OutcomeKindRejected ||
		response.Outcome.RejectionCode() != invocationdomain.RejectionInvalidArguments {
		t.Fatalf(
			"Outcome = kind %q code %q",
			response.Outcome.Kind(),
			response.Outcome.RejectionCode(),
		)
	}
	if fixture.arguments.calls != 1 {
		t.Fatalf("validator calls = %d, want 1", fixture.arguments.calls)
	}
	if fixture.facts.calls != 0 || fixture.policy.calls != 0 || fixture.executor.calls != 0 {
		t.Fatalf(
			"facts/policy/executor calls = %d/%d/%d, want 0/0/0",
			fixture.facts.calls,
			fixture.policy.calls,
			fixture.executor.calls,
		)
	}
}

func TestSchemaValidatorFailureRemainsInternalFailure(t *testing.T) {
	t.Parallel()

	fixture := newFixture(t)
	fixture.arguments.err = errors.New("schema could not be resolved")

	response, err := fixture.useCase.Execute(context.Background(), InvokeToolRequest{
		ToolID:    fixture.tool.ID(),
		Arguments: []byte(`{}`),
		Context:   catalogInvocationContext(t, ""),
	})
	if err == nil {
		t.Fatal("Execute() expected internal validation error")
	}
	if response.Outcome.Kind() != "" {
		t.Fatalf("Outcome kind = %q, want zero", response.Outcome.Kind())
	}
	if fixture.facts.calls != 0 || fixture.policy.calls != 0 || fixture.executor.calls != 0 {
		t.Fatalf(
			"facts/policy/executor calls = %d/%d/%d, want 0/0/0",
			fixture.facts.calls,
			fixture.policy.calls,
			fixture.executor.calls,
		)
	}

	record := fixture.audit.last(t)
	if record.ErrorClass() != auditdomain.ErrorInternal {
		t.Fatalf("ErrorClass() = %q", record.ErrorClass())
	}
}

func TestValidatedGovernanceFactsReachPolicyWithoutRawArguments(t *testing.T) {
	t.Parallel()

	fixture := newFixture(t)
	resource, err := invocationdomain.NewResourceFact("network.host", "api.example.test")
	if err != nil {
		t.Fatal(err)
	}
	facts, err := invocationdomain.KnownGovernanceFacts(
		[]invocationdomain.EffectFact{"read"},
		[]invocationdomain.ResourceFact{resource},
	)
	if err != nil {
		t.Fatal(err)
	}
	fixture.facts.facts = facts

	response, err := fixture.useCase.Execute(context.Background(), InvokeToolRequest{
		ToolID:    fixture.tool.ID(),
		Arguments: []byte(`{"host":"api.example.test"}`),
		Context:   catalogInvocationContext(t, "facts-test"),
	})
	if err != nil {
		t.Fatalf("Execute() error = %v", err)
	}
	if response.Outcome.Kind() != invocationdomain.OutcomeKindComplete {
		t.Fatalf("Outcome kind = %q", response.Outcome.Kind())
	}
	if fixture.arguments.calls != 1 || fixture.facts.calls != 1 || fixture.policy.calls != 1 {
		t.Fatalf(
			"validator/facts/policy calls = %d/%d/%d, want 1/1/1",
			fixture.arguments.calls,
			fixture.facts.calls,
			fixture.policy.calls,
		)
	}
	if fixture.policy.last.Context.Caller() != "facts-test" ||
		fixture.policy.last.Context.Scope().Kind() != invocationdomain.ScopeCatalog {
		t.Fatalf("Policy context = %#v", fixture.policy.last.Context)
	}
	if fixture.policy.last.Facts.Knowledge() != invocationdomain.FactKnowledgeKnown {
		t.Fatalf("Policy facts knowledge = %q", fixture.policy.last.Facts.Knowledge())
	}
	resources := fixture.policy.last.Facts.Resources()
	if len(resources) != 1 || resources[0].Value() != "api.example.test" {
		t.Fatalf("Policy resources = %#v", resources)
	}
}

func TestGovernanceFactExtractorFailureRemainsInternalFailure(t *testing.T) {
	t.Parallel()

	fixture := newFixture(t)
	fixture.facts.err = errors.New("fact extraction failed")

	_, err := fixture.useCase.Execute(context.Background(), InvokeToolRequest{
		ToolID:    fixture.tool.ID(),
		Arguments: []byte(`{}`),
		Context:   catalogInvocationContext(t, ""),
	})
	if err == nil {
		t.Fatal("Execute() expected fact extraction error")
	}
	if fixture.policy.calls != 0 || fixture.executor.calls != 0 {
		t.Fatalf(
			"policy/executor calls = %d/%d, want 0/0",
			fixture.policy.calls,
			fixture.executor.calls,
		)
	}
	record := fixture.audit.last(t)
	if record.ErrorClass() != auditdomain.ErrorInternal {
		t.Fatalf("ErrorClass() = %q", record.ErrorClass())
	}
}

func TestProfileScopeAuthorizesBeforeToolLookup(t *testing.T) {
	t.Parallel()

	fixture := newFixture(t)
	const profileReference = "prof_00000000000000000000000000000001"

	_, err := fixture.useCase.Execute(context.Background(), InvokeToolRequest{
		ToolID:    fixture.tool.ID(),
		Arguments: []byte(`{}`),
		Context:   profileInvocationContext(t, "mcp:default", profileReference),
	})
	if err != nil {
		t.Fatalf("Execute() error = %v", err)
	}
	if fixture.profileScopes.calls != 1 {
		t.Fatalf("profile scope calls = %d, want 1", fixture.profileScopes.calls)
	}
	if fixture.profileScopes.reference != profileReference {
		t.Fatalf("profile reference = %q, want %q", fixture.profileScopes.reference, profileReference)
	}
	if fixture.profileScopes.toolID != fixture.tool.ID() {
		t.Fatalf("profile scope ToolID = %q, want %q", fixture.profileScopes.toolID, fixture.tool.ID())
	}
	if fixture.tools.calls != 1 || fixture.executor.calls != 1 {
		t.Fatalf("tool/executor calls = %d/%d, want 1/1", fixture.tools.calls, fixture.executor.calls)
	}
}

func TestProfileScopeDenialPreventsToolLookupAndExecution(t *testing.T) {
	t.Parallel()

	fixture := newFixture(t)
	fixture.profileScopes.allowed = false

	response, err := fixture.useCase.Execute(context.Background(), InvokeToolRequest{
		ToolID:    fixture.tool.ID(),
		Arguments: []byte(`{}`),
		Context: profileInvocationContext(
			t,
			"mcp:restricted",
			"prof_00000000000000000000000000000002",
		),
	})
	if err != nil {
		t.Fatalf("Execute() error = %v", err)
	}
	if response.Outcome.Kind() != invocationdomain.OutcomeKindRejected ||
		response.Outcome.RejectionCode() != invocationdomain.RejectionToolOutsideScope {
		t.Fatalf("Outcome = kind %q code %q", response.Outcome.Kind(), response.Outcome.RejectionCode())
	}
	if fixture.profileScopes.calls != 1 {
		t.Fatalf("profile scope calls = %d, want 1", fixture.profileScopes.calls)
	}
	if fixture.tools.calls != 0 {
		t.Fatalf("tool lookup calls = %d, want 0", fixture.tools.calls)
	}
	if fixture.executor.calls != 0 {
		t.Fatalf("executor calls = %d, want 0", fixture.executor.calls)
	}

	record := fixture.audit.last(t)
	if record.ProviderID() != nil {
		t.Fatalf("ProviderID() = %v, want nil for scope denial before Tool resolution", record.ProviderID())
	}
	if record.Status() != auditdomain.StatusDenied || record.ErrorClass() != auditdomain.ErrorPolicy {
		t.Fatalf("Audit record = %#v", record)
	}
}

func TestToolLookupFailureIsAuditedWithoutProvider(t *testing.T) {
	t.Parallel()

	fixture := newFixture(t)
	fixture.tools.err = catalogapplication.ErrToolNotFound

	response, err := fixture.useCase.Execute(context.Background(), InvokeToolRequest{
		ToolID:    fixture.tool.ID(),
		Arguments: []byte(`{}`),
		Context:   catalogInvocationContext(t, ""),
	})
	if err != nil {
		t.Fatalf("Execute() error = %v", err)
	}
	if response.Outcome.Kind() != invocationdomain.OutcomeKindRejected ||
		response.Outcome.RejectionCode() != invocationdomain.RejectionToolNotFound {
		t.Fatalf("Outcome = kind %q code %q", response.Outcome.Kind(), response.Outcome.RejectionCode())
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

	response, err := fixture.useCase.Execute(context.Background(), InvokeToolRequest{
		ToolID:    fixture.tool.ID(),
		Arguments: []byte(`{}`),
		Context:   catalogInvocationContext(t, ""),
	})
	if err != nil {
		t.Fatalf("Execute() error = %v", err)
	}
	if response.Outcome.Kind() != invocationdomain.OutcomeKindRejected ||
		response.Outcome.RejectionCode() != invocationdomain.RejectionPolicyDenied {
		t.Fatalf("Outcome = kind %q code %q", response.Outcome.Kind(), response.Outcome.RejectionCode())
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

	response, err := fixture.useCase.Execute(context.Background(), InvokeToolRequest{
		ToolID:    fixture.tool.ID(),
		Arguments: []byte(`{}`),
		Context:   catalogInvocationContext(t, "agent"),
	})
	if err != nil {
		t.Fatalf("Execute() error = %v", err)
	}
	if response.Outcome.Kind() != invocationdomain.OutcomeKindRejected ||
		response.Outcome.RejectionCode() != invocationdomain.RejectionApprovalDenied {
		t.Fatalf("Outcome = kind %q code %q", response.Outcome.Kind(), response.Outcome.RejectionCode())
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
		Context:   catalogInvocationContext(t, ""),
	})
	if err != nil {
		t.Fatalf("Execute() error = %v", err)
	}
	if response.Outcome.Kind() != invocationdomain.OutcomeKindComplete {
		t.Fatalf("Result kind = %q", response.Outcome.Kind())
	}
	if fixture.credentials.calls != 1 {
		t.Fatalf("credential resolver calls = %d, want 1", fixture.credentials.calls)
	}
	if fixture.policy.calls != 1 ||
		fixture.policy.last.Facts.Knowledge() != invocationdomain.FactKnowledgeUnknown {
		t.Fatalf("Policy request = %#v", fixture.policy.last)
	}

	record := fixture.audit.last(t)
	if bytes.Contains([]byte(record.Caller()), fixture.credentials.secret) ||
		bytes.Contains([]byte(record.PolicyReason()), fixture.credentials.secret) {
		t.Fatal("audit record exposed secret material")
	}
}

func TestExecutorUnavailableReturnsRuntimeFailureOutcome(t *testing.T) {
	t.Parallel()

	fixture := newFixture(t)
	fixture.useCase.router = fakeRouter{err: ErrExecutorNotFound}

	response, err := fixture.useCase.Execute(context.Background(), InvokeToolRequest{
		ToolID:    fixture.tool.ID(),
		Arguments: []byte(`{}`),
		Context:   catalogInvocationContext(t, ""),
	})
	if err != nil {
		t.Fatalf("Execute() error = %v", err)
	}
	if response.Outcome.Kind() != invocationdomain.OutcomeKindFailed ||
		response.Outcome.FailureSource() != invocationdomain.FailureSourceRuntime ||
		response.Outcome.FailureCode() != invocationdomain.FailureExecutorUnavailable {
		t.Fatalf(
			"Outcome = kind %q source %q code %q",
			response.Outcome.Kind(),
			response.Outcome.FailureSource(),
			response.Outcome.FailureCode(),
		)
	}
	if fixture.executor.calls != 0 {
		t.Fatalf("executor calls = %d, want 0", fixture.executor.calls)
	}

	record := fixture.audit.last(t)
	if record.Status() != auditdomain.StatusFailed ||
		record.ErrorClass() != auditdomain.ErrorExecutorNotFound {
		t.Fatalf("Audit record = %#v", record)
	}
}

func TestToolFailureOutcomeRemainsToolFailure(t *testing.T) {
	t.Parallel()

	fixture := newFixture(t)
	failed, err := invocationdomain.Failed(
		invocationdomain.FailureSourceTool,
		invocationdomain.FailureToolError,
		"tool rejected request token=supersecretvalue",
	)
	if err != nil {
		t.Fatal(err)
	}
	fixture.executor.result = failed

	response, err := fixture.useCase.Execute(context.Background(), InvokeToolRequest{
		ToolID:    fixture.tool.ID(),
		Arguments: []byte(`{}`),
		Context:   catalogInvocationContext(t, ""),
	})
	if err != nil {
		t.Fatalf("Execute() error = %v", err)
	}
	if response.Outcome.Kind() != invocationdomain.OutcomeKindFailed ||
		response.Outcome.FailureSource() != invocationdomain.FailureSourceTool ||
		response.Outcome.FailureCode() != invocationdomain.FailureToolError {
		t.Fatalf(
			"Outcome = kind %q source %q code %q",
			response.Outcome.Kind(),
			response.Outcome.FailureSource(),
			response.Outcome.FailureCode(),
		)
	}
	if response.Outcome.Message() != "tool rejected request token=supersecretvalue" {
		t.Fatalf("Outcome message = %q", response.Outcome.Message())
	}

	record := fixture.audit.last(t)
	if record.Status() != auditdomain.StatusFailed ||
		record.ErrorClass() != auditdomain.ErrorExecution {
		t.Fatalf("Audit record = %#v", record)
	}
	if record.ErrorSummary() != "tool rejected request token=[REDACTED]" {
		t.Fatalf("Audit ErrorSummary() = %q", record.ErrorSummary())
	}
}

func TestExecutionDiagnosticSummaryIsAuditedAndReturned(t *testing.T) {
	t.Parallel()

	fixture := newFixture(t)
	fixture.executor.err = testDiagnosticError{
		message: "provider rejected request token=supersecretvalue",
	}

	response, err := fixture.useCase.Execute(context.Background(), InvokeToolRequest{
		ToolID:    fixture.tool.ID(),
		Arguments: []byte(`{}`),
		Context:   catalogInvocationContext(t, ""),
	})
	if err != nil {
		t.Fatalf("Execute() error = %v", err)
	}
	if response.Outcome.Kind() != invocationdomain.OutcomeKindFailed ||
		response.Outcome.FailureSource() != invocationdomain.FailureSourceProvider ||
		response.Outcome.FailureCode() != invocationdomain.FailureProviderError {
		t.Fatalf("Outcome = kind %q source %q code %q", response.Outcome.Kind(), response.Outcome.FailureSource(), response.Outcome.FailureCode())
	}
	if response.Outcome.Message() != "provider rejected request token=[REDACTED]" {
		t.Fatalf("Outcome message = %q", response.Outcome.Message())
	}

	record := fixture.audit.last(t)
	if record.ErrorClass() != auditdomain.ErrorExecution {
		t.Fatalf("ErrorClass() = %q", record.ErrorClass())
	}
	if record.ErrorSummary() != "provider rejected request token=[REDACTED]" {
		t.Fatalf("ErrorSummary() = %q", record.ErrorSummary())
	}
}

func TestInvocationLifecycleEventsAndLateProgress(t *testing.T) {
	t.Parallel()

	fixture := newFixture(t)
	fixture.executor.progressMessage = "halfway"

	response, err := fixture.useCase.Execute(context.Background(), InvokeToolRequest{
		ToolID:    fixture.tool.ID(),
		Arguments: []byte("{}"),
		Context:   catalogInvocationContext(t, "events-test"),
	})
	if err != nil {
		t.Fatalf("Execute() error = %v", err)
	}
	if response.Outcome.Kind() != invocationdomain.OutcomeKindComplete {
		t.Fatalf("Outcome kind = %q", response.Outcome.Kind())
	}

	events := fixture.observer.snapshot()
	want := []invocationdomain.EventKind{
		invocationdomain.EventAccepted,
		invocationdomain.EventScopeResolved,
		invocationdomain.EventArgumentsValidated,
		invocationdomain.EventPolicyDecided,
		invocationdomain.EventExecutionWaiting,
		invocationdomain.EventExecutionStarted,
		invocationdomain.EventProgress,
		invocationdomain.EventTerminal,
	}
	if got := eventKinds(events); !slices.Equal(got, want) {
		t.Fatalf("event kinds = %v, want %v", got, want)
	}
	if events[6].Message() != "halfway" {
		t.Fatalf("progress message = %q", events[6].Message())
	}
	terminal := events[len(events)-1]
	if terminal.OutcomeKind() != invocationdomain.OutcomeKindComplete {
		t.Fatalf("terminal outcome kind = %q", terminal.OutcomeKind())
	}

	if fixture.executor.lastReporter == nil {
		t.Fatal("executor did not receive progress reporter")
	}
	late, err := invocationdomain.NewProgressUpdate("late")
	if err != nil {
		t.Fatal(err)
	}
	fixture.executor.lastReporter.Report(late)
	if got := len(fixture.observer.snapshot()); got != len(events) {
		t.Fatalf("late progress appended event: count = %d, want %d", got, len(events))
	}
}

func TestRejectedInvocationEmitsTerminalWithoutExecutionStarted(t *testing.T) {
	t.Parallel()

	fixture := newFixture(t)
	fixture.policy.decision, _ = policydomain.Deny("test.denied", "denied")

	response, err := fixture.useCase.Execute(context.Background(), InvokeToolRequest{
		ToolID:    fixture.tool.ID(),
		Arguments: []byte("{}"),
		Context:   catalogInvocationContext(t, "events-denied"),
	})
	if err != nil {
		t.Fatalf("Execute() error = %v", err)
	}
	if response.Outcome.Kind() != invocationdomain.OutcomeKindRejected {
		t.Fatalf("Outcome kind = %q", response.Outcome.Kind())
	}

	got := eventKinds(fixture.observer.snapshot())
	want := []invocationdomain.EventKind{
		invocationdomain.EventAccepted,
		invocationdomain.EventScopeResolved,
		invocationdomain.EventArgumentsValidated,
		invocationdomain.EventPolicyDecided,
		invocationdomain.EventTerminal,
	}
	if !slices.Equal(got, want) {
		t.Fatalf("event kinds = %v, want %v", got, want)
	}
}

func TestApprovalLifecycleEventPrecedesExecution(t *testing.T) {
	t.Parallel()

	fixture := newFixture(t)
	fixture.policy.decision, _ = policydomain.RequireApproval("risk.write", "approval required")
	fixture.approvals.approved = true

	_, err := fixture.useCase.Execute(context.Background(), InvokeToolRequest{
		ToolID:    fixture.tool.ID(),
		Arguments: []byte("{}"),
		Context:   catalogInvocationContext(t, "events-approval"),
	})
	if err != nil {
		t.Fatalf("Execute() error = %v", err)
	}

	got := eventKinds(fixture.observer.snapshot())
	want := []invocationdomain.EventKind{
		invocationdomain.EventAccepted,
		invocationdomain.EventScopeResolved,
		invocationdomain.EventArgumentsValidated,
		invocationdomain.EventPolicyDecided,
		invocationdomain.EventApprovalResolved,
		invocationdomain.EventExecutionWaiting,
		invocationdomain.EventExecutionStarted,
		invocationdomain.EventTerminal,
	}
	if !slices.Equal(got, want) {
		t.Fatalf("event kinds = %v, want %v", got, want)
	}
}

func TestObserverPanicDoesNotChangeOutcomeOrAudit(t *testing.T) {
	t.Parallel()

	fixture := newFixture(t)
	fixture.useCase.observer = panickingObserver{}

	response, err := fixture.useCase.Execute(context.Background(), InvokeToolRequest{
		ToolID:    fixture.tool.ID(),
		Arguments: []byte("{}"),
		Context:   catalogInvocationContext(t, "observer-panic"),
	})
	if err != nil {
		t.Fatalf("Execute() error = %v", err)
	}
	if response.Outcome.Kind() != invocationdomain.OutcomeKindComplete {
		t.Fatalf("Outcome kind = %q", response.Outcome.Kind())
	}
	if record := fixture.audit.last(t); record.Status() != auditdomain.StatusCompleted {
		t.Fatalf("Audit status = %q", record.Status())
	}
}

func TestExecutionGuardWaitUsesInvocationTimeout(t *testing.T) {
	t.Parallel()

	fixture := newFixture(t)
	fixture.executionGuard.blockUntilDone = true

	response, err := fixture.useCase.Execute(context.Background(), InvokeToolRequest{
		ToolID:    fixture.tool.ID(),
		Arguments: []byte(`{}`),
		Context:   catalogInvocationContext(t, ""),
		Timeout:   time.Millisecond,
	})
	if err != nil {
		t.Fatalf("Execute() error = %v", err)
	}
	if response.Outcome.Kind() != invocationdomain.OutcomeKindFailed ||
		response.Outcome.FailureSource() != invocationdomain.FailureSourceRuntime ||
		response.Outcome.FailureCode() != invocationdomain.FailureTimedOut {
		t.Fatalf(
			"Outcome = kind %q source %q code %q",
			response.Outcome.Kind(),
			response.Outcome.FailureSource(),
			response.Outcome.FailureCode(),
		)
	}
	if fixture.executionGuard.calls != 1 || fixture.executor.calls != 0 {
		t.Fatalf(
			"guard/executor calls = %d/%d, want 1/0",
			fixture.executionGuard.calls,
			fixture.executor.calls,
		)
	}
	if fixture.resultGuard.calls != 0 {
		t.Fatalf("result guard calls = %d, want 0", fixture.resultGuard.calls)
	}

	record := fixture.audit.last(t)
	if record.Status() != auditdomain.StatusTimedOut ||
		record.ErrorClass() != auditdomain.ErrorTimedOut {
		t.Fatalf("Audit record = %#v", record)
	}
}

func TestExecutionGuardReleasesAfterExecutorFailure(t *testing.T) {
	t.Parallel()

	fixture := newFixture(t)
	fixture.executor.err = errors.New("provider failed")

	response, err := fixture.useCase.Execute(context.Background(), InvokeToolRequest{
		ToolID:    fixture.tool.ID(),
		Arguments: []byte(`{}`),
		Context:   catalogInvocationContext(t, ""),
	})
	if err != nil {
		t.Fatalf("Execute() error = %v", err)
	}
	if response.Outcome.Kind() != invocationdomain.OutcomeKindFailed {
		t.Fatalf("Outcome kind = %q", response.Outcome.Kind())
	}
	if fixture.executionGuard.releases != 1 {
		t.Fatalf("guard releases = %d, want 1", fixture.executionGuard.releases)
	}
}

func TestResultGuardFailureOutcomeIsAuditedAsResultGuard(t *testing.T) {
	t.Parallel()

	fixture := newFixture(t)
	guarded, err := invocationdomain.Failed(
		invocationdomain.FailureSourceResultGuard,
		invocationdomain.FailureOutputSchemaMismatch,
		"tool result does not match the declared output schema",
	)
	if err != nil {
		t.Fatal(err)
	}
	fixture.resultGuard.outcome = guarded

	response, err := fixture.useCase.Execute(context.Background(), InvokeToolRequest{
		ToolID:    fixture.tool.ID(),
		Arguments: []byte(`{}`),
		Context:   catalogInvocationContext(t, ""),
	})
	if err != nil {
		t.Fatalf("Execute() error = %v", err)
	}
	if response.Outcome.FailureSource() != invocationdomain.FailureSourceResultGuard ||
		response.Outcome.FailureCode() != invocationdomain.FailureOutputSchemaMismatch {
		t.Fatalf(
			"Outcome = source %q code %q",
			response.Outcome.FailureSource(),
			response.Outcome.FailureCode(),
		)
	}
	if fixture.resultGuard.calls != 1 {
		t.Fatalf("result guard calls = %d, want 1", fixture.resultGuard.calls)
	}
	if fixture.executionGuard.releases != 1 {
		t.Fatalf("guard releases = %d, want 1", fixture.executionGuard.releases)
	}

	record := fixture.audit.last(t)
	if record.ErrorClass() != auditdomain.ErrorResultGuard {
		t.Fatalf("ErrorClass() = %q", record.ErrorClass())
	}
}

func TestResultGuardInternalErrorRemainsGoError(t *testing.T) {
	t.Parallel()

	fixture := newFixture(t)
	fixture.resultGuard.err = errors.New("output schema could not be evaluated")

	response, err := fixture.useCase.Execute(context.Background(), InvokeToolRequest{
		ToolID:    fixture.tool.ID(),
		Arguments: []byte(`{}`),
		Context:   catalogInvocationContext(t, ""),
	})
	if err == nil {
		t.Fatal("Execute() expected result guard internal error")
	}
	if response.Outcome.Kind() != "" {
		t.Fatalf("Outcome kind = %q, want zero", response.Outcome.Kind())
	}
	if fixture.executionGuard.releases != 1 {
		t.Fatalf("guard releases = %d, want 1", fixture.executionGuard.releases)
	}

	record := fixture.audit.last(t)
	if record.ErrorClass() != auditdomain.ErrorInternal {
		t.Fatalf("ErrorClass() = %q", record.ErrorClass())
	}
}

func TestParentCancellationIsAudited(t *testing.T) {
	t.Parallel()

	fixture := newFixture(t)
	fixture.executor.blockUntilDone = true

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	response, err := fixture.useCase.Execute(ctx, InvokeToolRequest{
		ToolID:    fixture.tool.ID(),
		Arguments: []byte(`{}`),
		Context:   catalogInvocationContext(t, ""),
	})
	if err != nil {
		t.Fatalf("Execute() error = %v", err)
	}
	if response.Outcome.Kind() != invocationdomain.OutcomeKindFailed ||
		response.Outcome.FailureSource() != invocationdomain.FailureSourceRuntime ||
		response.Outcome.FailureCode() != invocationdomain.FailureCancelled {
		t.Fatalf("Outcome = kind %q source %q code %q", response.Outcome.Kind(), response.Outcome.FailureSource(), response.Outcome.FailureCode())
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

	response, err := fixture.useCase.Execute(context.Background(), InvokeToolRequest{
		ToolID:    fixture.tool.ID(),
		Arguments: []byte(`{}`),
		Context:   catalogInvocationContext(t, ""),
		Timeout:   time.Millisecond,
	})
	if err != nil {
		t.Fatalf("Execute() error = %v", err)
	}
	if response.Outcome.Kind() != invocationdomain.OutcomeKindFailed ||
		response.Outcome.FailureSource() != invocationdomain.FailureSourceRuntime ||
		response.Outcome.FailureCode() != invocationdomain.FailureTimedOut {
		t.Fatalf("Outcome = kind %q source %q code %q", response.Outcome.Kind(), response.Outcome.FailureSource(), response.Outcome.FailureCode())
	}

	record := fixture.audit.last(t)
	if record.Status() != auditdomain.StatusTimedOut || record.ErrorClass() != auditdomain.ErrorTimedOut {
		t.Fatalf("Audit record = %#v", record)
	}
}

type fixture struct {
	useCase        InvokeTool
	tool           catalogdomain.Tool
	tools          *fakeToolLookup
	profileScopes  *fakeProfileScopes
	arguments      *fakeArgumentValidator
	facts          *fakeFactExtractor
	policy         *fakePolicy
	approvals      *fakeApproval
	executionGuard *fakeExecutionGuard
	resultGuard    *fakeResultGuard
	observer       *memoryObserver
	executor       *fakeExecutor
	credentials    *fakeCredentials
	audit          *memoryAudit
}

func newFixture(t *testing.T) fixture {
	t.Helper()

	tool := testTool(t)
	content, _ := invocationdomain.NewJSONValue([]byte(`{"ok":true}`))
	result, _ := invocationdomain.Complete(content)
	allow, _ := policydomain.Allow("local.default", "")

	tools := &fakeToolLookup{tool: tool}
	profileScopes := &fakeProfileScopes{allowed: true}
	arguments := &fakeArgumentValidator{valid: true}
	facts := &fakeFactExtractor{facts: invocationdomain.UnknownGovernanceFacts()}
	policy := &fakePolicy{decision: allow}
	approvals := &fakeApproval{approved: true}
	executionGuard := &fakeExecutionGuard{}
	resultGuard := &fakeResultGuard{}
	observer := &memoryObserver{}
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
		tools,
		profileScopes,
		arguments,
		facts,
		policy,
		approvals,
		executionGuard,
		resultGuard,
		observer,
		fakeRouter{executor: executor},
		credentials,
		audit,
		ids,
		clock,
		time.Second,
	)

	return fixture{
		useCase:        useCase,
		tool:           tool,
		tools:          tools,
		profileScopes:  profileScopes,
		arguments:      arguments,
		facts:          facts,
		policy:         policy,
		approvals:      approvals,
		executionGuard: executionGuard,
		resultGuard:    resultGuard,
		observer:       observer,
		executor:       executor,
		credentials:    credentials,
		audit:          audit,
	}
}

type fakeToolLookup struct {
	tool  catalogdomain.Tool
	err   error
	calls int
}

func (f *fakeToolLookup) GetTool(
	context.Context,
	catalogdomain.ToolID,
) (catalogdomain.Tool, error) {
	f.calls++
	return f.tool, f.err
}

type fakeProfileScopes struct {
	allowed   bool
	err       error
	calls     int
	reference string
	toolID    catalogdomain.ToolID
}

func (f *fakeProfileScopes) AllowsTool(
	_ context.Context,
	reference string,
	toolID catalogdomain.ToolID,
) (bool, error) {
	f.calls++
	f.reference = reference
	f.toolID = toolID
	return f.allowed, f.err
}

type fakeArgumentValidator struct {
	valid bool
	err   error
	calls int
}

func (f *fakeArgumentValidator) Validate(
	context.Context,
	catalogdomain.Schema,
	invocationdomain.JSONValue,
) (bool, error) {
	f.calls++
	return f.valid, f.err
}

type fakeFactExtractor struct {
	facts invocationdomain.GovernanceFacts
	err   error
	calls int
}

func (f *fakeFactExtractor) Extract(
	context.Context,
	catalogdomain.Tool,
	invocationdomain.JSONValue,
) (invocationdomain.GovernanceFacts, error) {
	f.calls++
	return f.facts, f.err
}

type fakePolicy struct {
	decision policydomain.Decision
	err      error
	calls    int
	last     PolicyRequest
}

func (f *fakePolicy) Evaluate(
	_ context.Context,
	request PolicyRequest,
) (policydomain.Decision, error) {
	f.calls++
	f.last = request
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

type fakeExecutionGuard struct {
	err            error
	calls          int
	releases       int
	blockUntilDone bool
}

func (f *fakeExecutionGuard) Acquire(
	ctx context.Context,
	_ ExecutionGuardRequest,
) (func(), error) {
	f.calls++
	if f.blockUntilDone {
		<-ctx.Done()
		return nil, ctx.Err()
	}
	if f.err != nil {
		return nil, f.err
	}
	return func() { f.releases++ }, nil
}

type fakeResultGuard struct {
	outcome invocationdomain.Outcome
	err     error
	calls   int
}

func (f *fakeResultGuard) Guard(
	_ context.Context,
	_ catalogdomain.Tool,
	outcome invocationdomain.Outcome,
) (invocationdomain.Outcome, error) {
	f.calls++
	if f.err != nil {
		return invocationdomain.Outcome{}, f.err
	}
	if f.outcome.Kind() != "" {
		return f.outcome, nil
	}
	return outcome, nil
}

type fakeRouter struct {
	executor ToolExecutor
	err      error
}

func (f fakeRouter) ExecutorFor(context.Context, catalogdomain.Tool) (ToolExecutor, error) {
	return f.executor, f.err
}

type fakeExecutor struct {
	result            invocationdomain.Outcome
	err               error
	calls             int
	resolveCredential bool
	blockUntilDone    bool
	progressMessage   string
	lastReporter      ProgressReporter
}

func (f *fakeExecutor) Execute(
	ctx context.Context,
	_ ExecutionRequest,
	credentials CredentialResolver,
	progress ProgressReporter,
) (invocationdomain.Outcome, error) {
	f.calls++
	f.lastReporter = progress
	if f.progressMessage != "" && progress != nil {
		update, err := invocationdomain.NewProgressUpdate(f.progressMessage)
		if err != nil {
			return invocationdomain.Outcome{}, err
		}
		progress.Report(update)
	}
	if f.blockUntilDone {
		<-ctx.Done()
		return invocationdomain.Outcome{}, ctx.Err()
	}
	if f.resolveCredential {
		ref, err := credentialdomain.ParseReference("credential://github/work")
		if err != nil {
			return invocationdomain.Outcome{}, err
		}
		secret, err := credentials.Resolve(ctx, ref)
		if err != nil {
			return invocationdomain.Outcome{}, err
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

type memoryObserver struct {
	mu     sync.Mutex
	events []invocationdomain.Event
}

func (m *memoryObserver) Observe(event invocationdomain.Event) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.events = append(m.events, event)
}

func (m *memoryObserver) snapshot() []invocationdomain.Event {
	m.mu.Lock()
	defer m.mu.Unlock()
	return slices.Clone(m.events)
}

type panickingObserver struct{}

func (panickingObserver) Observe(invocationdomain.Event) {
	panic("observer failure")
}

func eventKinds(events []invocationdomain.Event) []invocationdomain.EventKind {
	kinds := make([]invocationdomain.EventKind, 0, len(events))
	for _, event := range events {
		kinds = append(kinds, event.Kind())
	}
	return kinds
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

func catalogInvocationContext(
	t *testing.T,
	caller string,
) invocationdomain.InvocationContext {
	t.Helper()
	executionContext, err := invocationdomain.NewInvocationContext(
		invocationdomain.OriginInternal,
		caller,
		invocationdomain.CatalogExecutionScope(),
	)
	if err != nil {
		t.Fatal(err)
	}
	return executionContext
}

func profileInvocationContext(
	t *testing.T,
	caller string,
	profileReference string,
) invocationdomain.InvocationContext {
	t.Helper()
	scope, err := invocationdomain.ProfileExecutionScope(profileReference)
	if err != nil {
		t.Fatal(err)
	}
	executionContext, err := invocationdomain.NewInvocationContext(
		invocationdomain.OriginMCP,
		caller,
		scope,
	)
	if err != nil {
		t.Fatal(err)
	}
	return executionContext
}

func mustInvocationID(t *testing.T, raw string) invocationdomain.InvocationID {
	t.Helper()
	id, err := invocationdomain.ParseInvocationID(raw)
	if err != nil {
		t.Fatal(err)
	}
	return id
}

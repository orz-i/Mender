package delegationprovider

import (
	"context"
	"errors"
	"path/filepath"
	"strings"
	"testing"
	"time"

	agentapp "github.com/orz-i/Mender/backend/internal/agent/application"
	agentdomain "github.com/orz-i/Mender/backend/internal/agent/domain"
	agentsqlite "github.com/orz-i/Mender/backend/internal/agent/infrastructure/sqlite"
	catalogdomain "github.com/orz-i/Mender/backend/internal/catalog/domain"
	gov "github.com/orz-i/Mender/backend/internal/governance/domain"
	invocationapp "github.com/orz-i/Mender/backend/internal/invocation/application"
	invocationdomain "github.com/orz-i/Mender/backend/internal/invocation/domain"
	providerplugin "github.com/orz-i/Mender/backend/internal/providerplugin"
)

func TestUnsupportedNativeSessionContinuationReturnsSafeToolFailure(t *testing.T) {
	ctx := context.Background()
	store, err := agentsqlite.Open(ctx, filepath.Join(t.TempDir(), "agents.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = store.Close() }()
	principal, err := gov.NewPrincipal(gov.PrincipalLocalUser, "os:test")
	if err != nil {
		t.Fatal(err)
	}
	caller, err := gov.NewContext(gov.OriginInternal, principal, gov.CatalogScope())
	if err != nil {
		t.Fatal(err)
	}
	svc, err := agentapp.NewService(store, inertRunner{}, func(context.Context, gov.Context, agentdomain.Agent, gov.Action) error { return nil }, principal, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = svc.Close() }()
	agy, err := svc.Create(ctx, agentapp.CreateRequest{Name: "Agy bounded fixture", Backend: agentdomain.BackendAntigravity, WorkspaceRoot: t.TempDir(), Isolation: "host"}, caller)
	if err != nil {
		t.Fatal(err)
	}
	other, err := svc.Create(ctx, agentapp.CreateRequest{Name: "Other bounded fixture", Backend: agentdomain.BackendGrok, WorkspaceRoot: t.TempDir(), Isolation: "host"}, caller)
	if err != nil {
		t.Fatal(err)
	}
	original, err := svc.Submit(ctx, agy.ID(), "fixture prior prompt", caller)
	if err != nil {
		t.Fatal(err)
	}
	notOwned, err := svc.Submit(ctx, other.ID(), "other fixture", caller)
	if err != nil {
		t.Fatal(err)
	}
	provider := New(svc, principal)
	inst, err := provider.Open(ctx, providerplugin.OpenRequest{ConfigVersion: ConfigVersion, Config: []byte(`{"agentId":"` + string(agy.ID()) + `"}`)})
	if err != nil {
		t.Fatal(err)
	}
	inputs, err := catalogdomain.NewSchema(resumeInput)
	if err != nil {
		t.Fatal(err)
	}
	outputs, err := catalogdomain.NewSchema(resumeOutput)
	if err != nil {
		t.Fatal(err)
	}
	pid, _ := catalogdomain.ParseProviderID("prov_00000000000000000000000000000001")
	tid, _ := catalogdomain.ParseToolID("tool_00000000000000000000000000000001")
	tool, err := catalogdomain.NewTool(tid, pid, catalogdomain.ToolSpec{
		SourceKey: sourceResume, Name: "resume", Title: "Resume", Description: "Fixture",
		InputSchema: inputs, OutputSchema: &outputs,
	}, time.Now())
	if err != nil {
		t.Fatal(err)
	}
	call := func(taskID agentdomain.TaskID) (invocationdomain.Outcome, error) {
		args, err := invocationdomain.NewJSONValue([]byte(`{"taskId":"` + string(taskID) + `","prompt":"do not run previous prompt"}`))
		if err != nil {
			t.Fatal(err)
		}
		return inst.Execute(ctx, invocationapp.ExecutionRequest{Tool: tool, Arguments: args, Context: caller}, nil, nil)
	}
	before, err := svc.Tasks(ctx, agy.ID(), caller)
	if err != nil {
		t.Fatal(err)
	}
	outcome, err := call(original.ID)
	if err != nil {
		t.Fatal(err)
	}
	if outcome.Kind() != invocationdomain.OutcomeKindFailed ||
		outcome.FailureSource() != invocationdomain.FailureSourceTool ||
		outcome.FailureCode() != invocationdomain.FailureToolError ||
		!strings.Contains(outcome.Message(), "No new task was created") {
		t.Fatalf("unsafe resume result: kind=%s, source=%s, code=%s, message=%q",
			outcome.Kind(), outcome.FailureSource(), outcome.FailureCode(), outcome.Message())
	}
	after, err := svc.Tasks(ctx, agy.ID(), caller)
	if err != nil || len(after) != len(before) {
		t.Fatalf("resume created Task: before=%d after=%d err=%v", len(before), len(after), err)
	}
	// Forbidden tasks from other Agents must never disclose whether their
	// native Session is recoverable.
	_, err = call(notOwned.ID)
	if !errors.Is(err, agentdomain.ErrForbidden) {
		t.Fatalf("cross-Agent TaskID changed its authorization guard: %v", err)
	}
}

package application

import (
	"context"
	"errors"
	"path/filepath"
	"testing"
	"time"

	agentdomain "github.com/orz-i/Mender/backend/internal/agent/domain"
	agentsqlite "github.com/orz-i/Mender/backend/internal/agent/infrastructure/sqlite"
	gov "github.com/orz-i/Mender/backend/internal/governance/domain"
)

type fakeExecutor struct {
	started chan struct{}
	release chan struct{}
}

func (x fakeExecutor) ExecuteConfigured(ctx context.Context, _ agentdomain.Agent, _ string, config RunConfig) (string, error) {
	close(x.started)
	select {
	case <-x.release:
		if err := observeFixtureTerminal(config); err != nil {
			return "", err
		}
		return "fixture result", nil
	case <-ctx.Done():
		return "", ctx.Err()
	}
}

// Test runners that claim a completed native turn must persist the same
// terminal/process evidence as real backends. No Session ID is required for
// Task completion: Session context and a single Task result are independent.
func observeFixtureTerminal(cfg RunConfig) error {
	if cfg.Observe == nil {
		return nil
	}
	for _, event := range []agentdomain.ExecutionEvent{
		{Kind: agentdomain.EventProcessStarted, ProcessPID: 77001},
		{Kind: agentdomain.EventPromptSent},
		{Kind: agentdomain.EventTerminalSeen},
		{Kind: agentdomain.EventLeaderReaped},
	} {
		if err := cfg.Observe(event); err != nil {
			return err
		}
	}
	return nil
}
func owner(t *testing.T) gov.Context {
	t.Helper()
	p, err := gov.NewPrincipal(gov.PrincipalLocalUser, "os:501")
	if err != nil {
		t.Fatal(err)
	}
	c, err := gov.NewContext(gov.OriginCLI, p, gov.CatalogScope())
	if err != nil {
		t.Fatal(err)
	}
	return c
}
func profile(t *testing.T, id string) gov.Context {
	t.Helper()
	p, err := gov.NewPrincipal(gov.PrincipalService, "profile:"+id)
	if err != nil {
		t.Fatal(err)
	}
	scope, err := gov.ProfileScope(id)
	if err != nil {
		t.Fatal(err)
	}
	c, err := gov.NewContext(gov.OriginMCP, p, scope)
	if err != nil {
		t.Fatal(err)
	}
	return c
}

const goodProfileID = "prof_00000000000000000000000000000001"
const badProfileID = "prof_00000000000000000000000000000002"
const otherProfileID = "prof_00000000000000000000000000000003"

func newFixtureService(t *testing.T, run Executor) (*Service, gov.Context) {
	t.Helper()
	dbPath := filepath.Join(t.TempDir(), "agents.db")
	requireApprovalFixturePolicy(t, dbPath)
	s, err := agentsqlite.Open(context.Background(), dbPath)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = s.Close() })
	o := owner(t)
	service, err := NewService(s, run, func(context.Context, gov.Context, agentdomain.Agent, gov.Action) error { return nil },
		o.Principal(), func(_ context.Context, id string) error {
			if id == goodProfileID {
				return nil
			}
			return errors.New("profile missing")
		})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = service.Close() })
	return service, o
}
func waitTask(t *testing.T, s *Service, id agentdomain.TaskID, o gov.Context, status agentdomain.TaskStatus) agentdomain.Task {
	t.Helper()
	deadline := time.After(5 * time.Second)
	for {
		item, err := s.Task(context.Background(), id, o)
		if err != nil {
			t.Fatal(err)
		}
		if item.Status == status {
			return item
		}
		select {
		case <-deadline:
			t.Fatalf("Task stuck in %s; want %s", item.Status, status)
		case <-time.After(10 * time.Millisecond):
		}
	}
}
func TestServiceRejectsUntrustedProfileAndCancelsFixture(t *testing.T) {
	x := fakeExecutor{started: make(chan struct{}), release: make(chan struct{})}
	s, o := newFixtureService(t, x)
	ctx := context.Background()
	a, err := s.Create(ctx, CreateRequest{Name: "Local", Backend: agentdomain.BackendGrok,
		WorkspaceRoot: t.TempDir(), Isolation: "restricted_host"}, o)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := s.Submit(ctx, a.ID(), "inspect", profile(t, goodProfileID)); !errors.Is(err, agentdomain.ErrForbidden) {
		t.Fatalf("ungranted Profile accepted: %v", err)
	}
	if err := s.Grant(ctx, a.ID(), goodProfileID, o); err != nil {
		t.Fatal(err)
	}
	if err := s.Grant(ctx, a.ID(), badProfileID, o); err == nil {
		t.Fatal("nonexistent Profile granted")
	}
	task, err := s.Submit(ctx, a.ID(), "inspect", profile(t, goodProfileID))
	if err != nil {
		t.Fatal(err)
	}
	select {
	case <-x.started:
	case <-time.After(5 * time.Second):
		t.Fatal("executor never started")
	}
	if _, err := s.Task(ctx, task.ID, profile(t, otherProfileID)); !errors.Is(err, agentdomain.ErrForbidden) {
		t.Fatal("cross-Profile Task leaked", err)
	}
	if err := s.Cancel(ctx, task.ID, profile(t, goodProfileID)); err != nil {
		t.Fatal(err)
	}
	after := waitTask(t, s, task.ID, o, agentdomain.TaskCancelled)
	if !after.CancelRequested {
		t.Fatal("cancel intent missing")
	}
	if err := s.Revoke(ctx, a.ID(), goodProfileID, o); err != nil {
		t.Fatal(err)
	}
	if _, err := s.Task(ctx, task.ID, profile(t, goodProfileID)); !errors.Is(err, agentdomain.ErrForbidden) {
		t.Fatal("revoked Profile can still read", err)
	}
}
func TestServiceCompletionAndPolicyPreflight(t *testing.T) {
	x := fakeExecutor{started: make(chan struct{}), release: make(chan struct{})}
	s, o := newFixtureService(t, x)
	ctx := context.Background()
	a, err := s.Create(ctx, CreateRequest{Name: "Local", Backend: agentdomain.BackendCodex, WorkspaceRoot: t.TempDir()}, o)
	if err != nil {
		t.Fatal(err)
	}
	task, err := s.Submit(ctx, a.ID(), "review", o)
	if err != nil {
		t.Fatal(err)
	}
	<-x.started
	close(x.release)
	done := waitTask(t, s, task.ID, o, agentdomain.TaskCompleted)
	if done.Result != "fixture result" {
		t.Fatalf("result = %+v", done)
	}
	if err := s.Cancel(ctx, task.ID, o); !errors.Is(err, agentdomain.ErrConflict) {
		t.Fatal("cancelled terminal task", err)
	}
}
func TestServiceDeniedPolicyNeverStartsRunner(t *testing.T) {
	store, err := agentsqlite.Open(context.Background(), filepath.Join(t.TempDir(), "agents.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = store.Close() }()
	o := owner(t)
	x := fakeExecutor{started: make(chan struct{}), release: make(chan struct{})}
	s, err := NewService(store, x, func(_ context.Context, _ gov.Context, _ agentdomain.Agent, _ gov.Action) error {
		return agentdomain.ErrForbidden
	}, o.Principal(), nil)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = s.Close() }()
	a, err := s.Create(context.Background(), CreateRequest{Name: "Reader", Backend: agentdomain.BackendCodex, WorkspaceRoot: t.TempDir()}, o)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := s.Submit(context.Background(), a.ID(), "task", o); !errors.Is(err, agentdomain.ErrForbidden) {
		t.Fatalf("unauthorized submit %v", err)
	}
	select {
	case <-x.started:
		t.Fatal("process started before admission")
	default:
	}
	tasks, err := s.Tasks(context.Background(), a.ID(), o)
	if err != nil || len(tasks) > 0 {
		t.Fatal("Task persisted before admission", err)
	}
}

func TestRuntimeShutdownDoesNotClaimCallerCancellation(t *testing.T) {
	ctx := context.Background()
	executor := fakeExecutor{started: make(chan struct{}), release: make(chan struct{})}
	service, ownerCtx := newFixtureService(t, executor)
	agent, err := service.Create(ctx, CreateRequest{
		Name: "Shutdown safety fixture", Backend: agentdomain.BackendGrok,
		WorkspaceRoot: t.TempDir(),
	}, ownerCtx)
	if err != nil {
		t.Fatal(err)
	}
	task, err := service.Submit(ctx, agent.ID(), "read-only fixture prompt", ownerCtx)
	if err != nil {
		t.Fatal(err)
	}
	select {
	case <-executor.started:
	case <-time.After(5 * time.Second):
		t.Fatal("fixture task not running")
	}
	if err := service.Close(); err != nil {
		t.Fatal(err)
	}
	observed, err := service.Task(ctx, task.ID, ownerCtx)
	if err != nil {
		t.Fatal(err)
	}
	if observed.Status != agentdomain.TaskUnknown || observed.CancelRequested {
		t.Fatalf("runtime shutdown incorrectly claimed confirmed cancellation: %+v", observed)
	}
}

func TestGovernedToolSupplyFailureNeverLaunchesAgentAndIsExplicit(t *testing.T) {
	executor := fakeExecutor{started: make(chan struct{}), release: make(chan struct{})}
	service, ownerCtx := newFixtureService(t, executor)
	if err := service.ConfigureToolSupplyResolver(func(context.Context, agentdomain.Task) (ToolSupply, func(), error) {
		return ToolSupply{}, nil, agentdomain.ErrUnavailable
	}); err != nil {
		t.Fatal(err)
	}
	agent, err := service.Create(context.Background(), CreateRequest{
		Name: "Unavailable governed placement", Backend: agentdomain.BackendGrok, WorkspaceRoot: t.TempDir(),
	}, ownerCtx)
	if err != nil {
		t.Fatal(err)
	}
	task, err := service.Submit(context.Background(), agent.ID(), "fixture no process", ownerCtx)
	if err != nil {
		t.Fatal(err)
	}
	terminal := waitTask(t, service, task.ID, ownerCtx, agentdomain.TaskFailed)
	if terminal.Diagnostic != "Governed tool execution unavailable" {
		t.Fatalf("resolver error was hidden: %+v", terminal)
	}
	select {
	case <-executor.started:
		t.Fatal("Agent process started despite invalid governed placement")
	default:
	}
}
func TestCancelDuringGovernedToolResolutionRetainsCancelledLifecycle(t *testing.T) {
	executor := fakeExecutor{started: make(chan struct{}), release: make(chan struct{})}
	service, ownerCtx := newFixtureService(t, executor)
	entering := make(chan struct{})
	if err := service.ConfigureToolSupplyResolver(func(ctx context.Context, _ agentdomain.Task) (ToolSupply, func(), error) {
		close(entering)
		<-ctx.Done()
		return ToolSupply{}, nil, ctx.Err()
	}); err != nil {
		t.Fatal(err)
	}
	agent, err := service.Create(context.Background(), CreateRequest{
		Name: "Cancelled while resolving", Backend: agentdomain.BackendGrok, WorkspaceRoot: t.TempDir(),
	}, ownerCtx)
	if err != nil {
		t.Fatal(err)
	}
	task, err := service.Submit(context.Background(), agent.ID(), "fixture cancellation", ownerCtx)
	if err != nil {
		t.Fatal(err)
	}
	select {
	case <-entering:
	case <-time.After(4 * time.Second):
		t.Fatal("Task never entered governed resolver")
	}
	if err := service.Cancel(context.Background(), task.ID, ownerCtx); err != nil {
		t.Fatal(err)
	}
	after := waitTask(t, service, task.ID, ownerCtx, agentdomain.TaskCancelled)
	if after.Diagnostic != "Task cancelled" || !after.CancelRequested {
		t.Fatalf("resolver cancellation silently became a failure: %+v", after)
	}
	select {
	case <-executor.started:
		t.Fatal("Agent started after cancellation")
	default:
	}
}

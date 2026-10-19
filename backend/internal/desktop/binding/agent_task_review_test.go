package binding

import (
	"context"
	"errors"
	"path/filepath"
	"testing"
	"time"

	agentapp "github.com/orz-i/Mender/backend/internal/agent/application"
	agentdomain "github.com/orz-i/Mender/backend/internal/agent/domain"
	"github.com/orz-i/Mender/backend/internal/bootstrap"
	gov "github.com/orz-i/Mender/backend/internal/governance/domain"
)

type dispositionDesktopFixture struct{}

func (dispositionDesktopFixture) ExecuteConfigured(_ context.Context, _ agentdomain.Agent, _ string, config agentapp.RunConfig) (string, error) {
	for _, event := range []agentdomain.ExecutionEvent{
		{Kind: agentdomain.EventProcessStarted, ProcessPID: 1337},
		{Kind: agentdomain.EventSessionReady, NativeSession: "demo-native", CanResume: true},
		{Kind: agentdomain.EventPromptSent},
		{Kind: agentdomain.EventLeaderReaped},
	} {
		if err := config.Observe(event); err != nil {
			return "", err
		}
	}
	return "", agentdomain.ErrOutcomeUncertain
}

func TestWailsUnknownDispositionOwnerOnlyWithoutAgentReplay(t *testing.T) {
	ctx := context.Background()
	r, err := bootstrap.NewRuntime(ctx, bootstrap.RuntimeOptions{
		DatabasePath: filepath.Join(t.TempDir(), "desktop-review.db"), AgentExecutor: dispositionDesktopFixture{},
	})
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = r.Close() }()
	d := NewService(ctx, r)
	agent, err := d.CreateAgent(CreateAgentInput{Name: "unknown demo",
		Backend: "opencode", Isolation: "host", WorkspaceRoot: t.TempDir()})
	if err != nil {
		t.Fatal(err)
	}
	aid, err := agentdomain.ParseID(agent.AgentID)
	if err != nil {
		t.Fatal(err)
	}
	owner, err := gov.NewContext(gov.OriginDesktop, r.OwnerPrincipal(), gov.CatalogScope())
	if err != nil {
		t.Fatal(err)
	}
	task, err := r.Agents.Submit(ctx, aid, "fixture only", owner)
	if err != nil {
		t.Fatal(err)
	}
	var before AgentTask
	for deadline := time.Now().Add(4 * time.Second); time.Now().Before(deadline); {
		before, err = d.GetAgentTask(string(task.ID))
		if err != nil {
			t.Fatal(err)
		}
		if before.Status == "unknown" {
			break
		}
		time.Sleep(10 * time.Millisecond)
	}
	if before.Status != "unknown" {
		t.Fatalf("unknown fixture not terminal %+v", before)
	}
	if before.ResumeAvailability == nil || before.ResumeAvailability.Eligible || before.ResumeAvailability.Reason != "unknown_outcome" {
		t.Fatalf("Wails exposed unsafe resume %+v", before.ResumeAvailability)
	}
	handled, err := d.ReviewUnknownAgentTask(before.TaskID, agentdomain.UnknownHandledElsewhere)
	if err != nil || handled.Status != "unknown" || handled.Disposition == nil ||
		handled.Disposition.Handling != agentdomain.UnknownHandledElsewhere {
		t.Fatalf("Wails work disposition %+v %v", handled, err)
	}
	if _, err := d.ResumeAgentTask(before.TaskID, "NEVER EXECUTE"); !errors.Is(err, agentdomain.ErrResumeUnavailable) {
		t.Fatalf("Wails resumed unknown old work: %v", err)
	}
	tasks, err := d.ListAgentTasks(agent.AgentID)
	if err != nil || len(tasks.Tasks) != 1 || tasks.Tasks[0].Status != "unknown" ||
		tasks.Tasks[0].Disposition == nil {
		t.Fatalf("Wails review created new task or concealed unknown %+v %v", tasks, err)
	}
}

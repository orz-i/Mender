package application

import (
	"context"
	"errors"
	"strings"
	"sync"
	"testing"

	agentdomain "github.com/orz-i/Mender/backend/internal/agent/domain"
)

type nativeResumeFixtureExecutor struct {
	mu         sync.Mutex
	newPrompts []string
}

func (f *nativeResumeFixtureExecutor) ExecuteConfigured(_ context.Context, _ agentdomain.Agent, prompt string, cfg RunConfig) (string, error) {
	f.mu.Lock()
	f.newPrompts = append(f.newPrompts, prompt)
	f.mu.Unlock()
	session := agentdomain.ExecutionEvent{Kind: agentdomain.EventSessionReady,
		NativeSession: "fixture-native-host-session", CanLoad: true}
	if cfg.Resume != nil {
		session.ExistingSessionRef = cfg.Resume.SessionRef
		session.NativeSession = cfg.Resume.NativeSessionID
	}
	for _, event := range []agentdomain.ExecutionEvent{
		{Kind: agentdomain.EventProcessStarted, ProcessPID: 8888},
		session,
		{Kind: agentdomain.EventPromptSent},
		{Kind: agentdomain.EventTerminalSeen},
		{Kind: agentdomain.EventLeaderReaped},
	} {
		if err := cfg.Observe(event); err != nil {
			return "", err
		}
	}
	return "done", nil
}

func TestServiceNativeResumeCreatesNewTaskKeepsOriginalAndNoImplicitRetry(t *testing.T) {
	ctx := context.Background()
	executor := &nativeResumeFixtureExecutor{}
	svc, owner := newFixtureService(t, executor)
	agent, err := svc.Create(ctx, CreateRequest{Name: "Continuable OpenCode", Backend: agentdomain.BackendOpenCode,
		WorkspaceRoot: t.TempDir(), Isolation: "host"}, owner)
	if err != nil {
		t.Fatal(err)
	}
	first, err := svc.Submit(ctx, agent.ID(), "original instruction", owner)
	if err != nil {
		t.Fatal(err)
	}
	original := waitTask(t, svc, first.ID, owner, agentdomain.TaskCompleted)
	resumed, err := svc.Resume(ctx, original.ID, "NEW instruction not previous work", agent.ID(), owner)
	if err != nil {
		t.Fatal(err)
	}
	if resumed.ID == original.ID || resumed.ContinuedFromTaskID != original.ID {
		t.Fatalf("continuation identity drift %+v", resumed)
	}
	after := waitTask(t, svc, resumed.ID, owner, agentdomain.TaskCompleted)
	if after.ContinuedFromTaskID != original.ID || after.Observation == nil ||
		!after.Observation.SessionRecorded || after.Result != "done" {
		t.Fatalf("continuation outcome %+v", after)
	}
	old, err := svc.Task(ctx, original.ID, owner)
	if err != nil || old.Status != agentdomain.TaskCompleted || old.ContinuedFromTaskID != "" {
		t.Fatalf("original modified by continuation %+v %v", old, err)
	}
	if _, err := svc.Resume(ctx, original.ID, "duplicate", agent.ID(), owner); !errors.Is(err, agentdomain.ErrResumeUnavailable) {
		t.Fatalf("duplicate continued same source: %v", err)
	}
	// A subsequent explicit new prompt can continue from the newly
	// completed Task while retaining exactly the original native Session.
	last, err := svc.Resume(ctx, after.ID, "THIRD instruction", agent.ID(), owner)
	if err != nil {
		t.Fatal(err)
	}
	lastTask := waitTask(t, svc, last.ID, owner, agentdomain.TaskCompleted)
	if lastTask.ContinuedFromTaskID != after.ID {
		t.Fatalf("third turn lineage %+v", lastTask)
	}
	executor.mu.Lock()
	got := append([]string(nil), executor.newPrompts...)
	executor.mu.Unlock()
	if len(got) != 3 || got[0] != "original instruction" || got[1] != "NEW instruction not previous work" || got[2] != "THIRD instruction" {
		t.Fatalf("original task was silently replayed: %v", got)
	}
}

func TestServiceNativeResumeRequiresOriginalCallerGrantsAndExactAgent(t *testing.T) {
	ctx := context.Background()
	svc, owner := newFixtureService(t, &nativeResumeFixtureExecutor{})
	agent, err := svc.Create(ctx, CreateRequest{Name: "Profile-only continuation", Backend: agentdomain.BackendOpenCode,
		WorkspaceRoot: t.TempDir(), Isolation: "host"}, owner)
	if err != nil {
		t.Fatal(err)
	}
	if err := svc.Grant(ctx, agent.ID(), goodProfileID, owner); err != nil {
		t.Fatal(err)
	}
	origCaller := profile(t, goodProfileID)
	initial, err := svc.Submit(ctx, agent.ID(), "safe", origCaller)
	if err != nil {
		t.Fatal(err)
	}
	complete := waitTask(t, svc, initial.ID, owner, agentdomain.TaskCompleted)
	if _, err := svc.Resume(ctx, complete.ID, "owner takeover", agent.ID(), owner); !errors.Is(err, agentdomain.ErrForbidden) {
		t.Fatalf("owner impersonated Profile native Session: %v", err)
	}
	other, err := svc.Create(ctx, CreateRequest{Name: "Other Agent", Backend: agentdomain.BackendOpenCode,
		WorkspaceRoot: t.TempDir(), Isolation: "host"}, owner)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := svc.Resume(ctx, complete.ID, "wrong Agent", other.ID(), origCaller); !errors.Is(err, agentdomain.ErrForbidden) {
		t.Fatalf("cross-Agent TaskID accepted: %v", err)
	}
	if _, err := svc.Resume(ctx, complete.ID, "wrong Profile", agent.ID(), profile(t, otherProfileID)); !errors.Is(err, agentdomain.ErrForbidden) {
		t.Fatalf("ungranted Profile accepted: %v", err)
	}
	if err := svc.Revoke(ctx, agent.ID(), goodProfileID, owner); err != nil {
		t.Fatal(err)
	}
	if _, err := svc.Resume(ctx, complete.ID, "revoked Profile", agent.ID(), origCaller); !errors.Is(err, agentdomain.ErrForbidden) {
		t.Fatalf("revoked AgentGrant bypassed: %v", err)
	}
	if err := svc.Grant(ctx, agent.ID(), goodProfileID, owner); err != nil {
		t.Fatal(err)
	}
	next, err := svc.Resume(ctx, complete.ID, "authorized new prompt", agent.ID(), origCaller)
	if err != nil {
		t.Fatal(err)
	}
	waitTask(t, svc, next.ID, owner, agentdomain.TaskCompleted)
}

func TestServiceNativeResumeFailsClosedForRestrictedAndUnknownHistory(t *testing.T) {
	ctx := context.Background()
	svc, owner := newFixtureService(t, &nativeResumeFixtureExecutor{})
	restricted, err := svc.Create(ctx, CreateRequest{Name: "Private HOME", Backend: agentdomain.BackendOpenCode,
		WorkspaceRoot: t.TempDir(), Isolation: "restricted_host"}, owner)
	if err != nil {
		t.Fatal(err)
	}
	first, err := svc.Submit(ctx, restricted.ID(), "original", owner)
	if err != nil {
		t.Fatal(err)
	}
	waitTask(t, svc, first.ID, owner, agentdomain.TaskCompleted)
	if _, err := svc.Resume(ctx, first.ID, "new", restricted.ID(), owner); !errors.Is(err, agentdomain.ErrResumeUnavailable) {
		t.Fatalf("ephemeral restricted HOME misrepresented as persisted: %v", err)
	}
	if _, err := svc.Resume(ctx, first.ID, strings.Repeat("X", agentdomain.MaxPromptBytes+1), restricted.ID(), owner); !errors.Is(err, agentdomain.ErrInvalid) {
		t.Fatalf("unbounded continuation prompt accepted: %v", err)
	}
	// A synthetic unknown historical outcome must never be loaded as a
	// source for new filesystem effects; no guessed native association.
	id, err := agentdomain.NewTaskID()
	if err != nil {
		t.Fatal(err)
	}
	if _, err := svc.Resume(ctx, id, "new", restricted.ID(), owner); !errors.Is(err, agentdomain.ErrNotFound) {
		t.Fatalf("nonexistent TaskIDs guessed: %v", err)
	}
}

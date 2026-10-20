package bootstrap

import (
	"context"
	"encoding/json"
	"fmt"
	"path/filepath"
	"strings"
	"testing"

	agentapp "github.com/orz-i/Mender/backend/internal/agent/application"
	agentdomain "github.com/orz-i/Mender/backend/internal/agent/domain"
	catalogdomain "github.com/orz-i/Mender/backend/internal/catalog/domain"
	invocationdomain "github.com/orz-i/Mender/backend/internal/invocation/domain"
	profileapp "github.com/orz-i/Mender/backend/internal/profile/application"
	profiledomain "github.com/orz-i/Mender/backend/internal/profile/domain"
)

const bootstrapAgyNativeSession = "dfe3c2a6-211b-48dc-bfa8-a5d2ccf51b85"

// This tests canonical Mender admission, SQLite evidence and the same native
// SessionRef lineage. The CLI's pre-prompt identity and fallback handling are
// tested separately by the real antigravity.Runner and launcher tests.
type agySessionFixtureExecutor struct{}

func (agySessionFixtureExecutor) ExecuteConfigured(_ context.Context, _ agentdomain.Agent,
	prompt string, cfg agentapp.RunConfig) (string, error) {
	if cfg.Resume != nil &&
		(cfg.Resume.NativeSessionID != bootstrapAgyNativeSession || !cfg.Resume.Valid()) {
		return "", fmt.Errorf("%w: mismatched native conversation", agentdomain.ErrOutcomeUncertain)
	}
	event := agentdomain.ExecutionEvent{Kind: agentdomain.EventSessionReady,
		NativeSession: bootstrapAgyNativeSession, CanLoad: true, CanResume: true}
	if cfg.Resume != nil {
		event.ExistingSessionRef = cfg.Resume.SessionRef
	}
	for _, e := range []agentdomain.ExecutionEvent{
		{Kind: agentdomain.EventProcessStarted, ProcessPID: 20788},
		event,
		{Kind: agentdomain.EventPromptSent},
		{Kind: agentdomain.EventTerminalSeen},
		{Kind: agentdomain.EventLeaderReaped},
	} {
		if err := cfg.Observe(e); err != nil {
			return "", err
		}
	}
	return "agy fixture: " + prompt, nil
}

func TestCanonicalAntigravitySessionContinuationPreservesGrantAndLineage(t *testing.T) {
	ctx := context.Background()
	r, err := NewRuntime(ctx, RuntimeOptions{DatabasePath: filepath.Join(t.TempDir(), "agy-resume.db"),
		AgentExecutor: agySessionFixtureExecutor{}})
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = r.Close() }()
	owner := agentOwnerContext(t, r)
	agent, err := r.CreateAgentWithTools(ctx, agentapp.CreateRequest{
		Name: "Temporary host Agy", Backend: agentdomain.BackendAntigravity,
		WorkspaceRoot: t.TempDir(), Isolation: "host",
	}, owner)
	if err != nil {
		t.Fatal(err)
	}
	suite, err := r.AgentTools(ctx, agent.ID(), owner)
	if err != nil || len(suite.Tools) != 5 {
		t.Fatalf("incomplete canonical suite %+v %v", suite, err)
	}
	ids := map[string]catalogdomain.ToolID{}
	for _, item := range suite.Tools {
		id, err := catalogdomain.ParseToolID(item.ToolID)
		if err != nil {
			t.Fatal(err)
		}
		ids[item.SourceKey] = id
	}
	profile, err := r.Profiles.Create.Execute(ctx, profileapp.CreateProfileRequest{
		Slug: "agy-resume", DisplayName: "Agy Continuation", Mode: profiledomain.ExposureDynamic,
	})
	if err != nil {
		t.Fatal(err)
	}
	for _, key := range []string{"agent.delegate", "agent.result"} {
		profile, err = r.Profiles.AddTool.Execute(ctx, profileapp.AddToolRequest{
			ProfileSlug: profile.Slug(), ToolID: ids[key],
		})
		if err != nil {
			t.Fatal(err)
		}
	}
	if err := r.Agents.Grant(ctx, agent.ID(), profile.ID().String(), owner); err != nil {
		t.Fatal(err)
	}
	caller := taskServicePrincipal(t, profile.ID().String())
	submitted, err := r.CallConfiguredTool(ctx, ids["agent.delegate"], []byte(`{"prompt":"first"}`), caller)
	if err != nil || submitted.Outcome.Kind() != invocationdomain.OutcomeKindComplete {
		t.Fatalf("original submission failed: %+v %v", submitted, err)
	}
	var receipt struct {
		TaskID string `json:"taskId"`
	}
	if err := json.Unmarshal(submitted.Outcome.Content().Bytes(), &receipt); err != nil || receipt.TaskID == "" {
		t.Fatalf("missing original Task: %+v %v", receipt, err)
	}
	waitCanonicalAgentOutcome(t, r, ids["agent.result"], caller, receipt.TaskID, "completed")
	originalID, err := agentdomain.ParseTaskID(receipt.TaskID)
	if err != nil {
		t.Fatal(err)
	}
	originalPlan, err := r.agentStore.ResumePlan(ctx, originalID)
	if err != nil || !originalPlan.Valid() || originalPlan.NativeSessionID != bootstrapAgyNativeSession {
		t.Fatalf("unverified native session: %+v %v", originalPlan, err)
	}
	prompt := []byte(`{"taskId":"` + receipt.TaskID + `","prompt":"new prompt only"}`)
	// A visible canonical Tool definition must never silently grant Resume.
	beforeGrant, err := r.CallConfiguredTool(ctx, ids["agent.resume"], prompt, caller)
	if err == nil && beforeGrant.Outcome.Kind() == invocationdomain.OutcomeKindComplete {
		t.Fatal("unbound Agent resume was executed")
	}
	profile, err = r.Profiles.AddTool.Execute(ctx, profileapp.AddToolRequest{
		ProfileSlug: profile.Slug(), ToolID: ids["agent.resume"],
	})
	if err != nil {
		t.Fatal(err)
	}
	resumed, err := r.CallConfiguredTool(ctx, ids["agent.resume"], prompt, caller)
	if err != nil || resumed.Outcome.Kind() != invocationdomain.OutcomeKindComplete {
		t.Fatalf("Agy native conversation continuation failed: %+v %v", resumed, err)
	}
	var receipt2 struct {
		TaskID, ContinuedFromTaskID, Status string
	}
	if err := json.Unmarshal(resumed.Outcome.Content().Bytes(), &receipt2); err != nil ||
		receipt2.TaskID == "" || receipt2.TaskID == receipt.TaskID ||
		receipt2.ContinuedFromTaskID != receipt.TaskID {
		t.Fatalf("not an explicit new child Task %+v %v", receipt2, err)
	}
	continued := waitCanonicalAgentOutcome(t, r, ids["agent.result"], caller, receipt2.TaskID, "completed")
	if continued.Result != "agy fixture: new prompt only" || continued.ContinuedFromTaskID != receipt.TaskID {
		t.Fatalf("new prompt continuation failed %+v", continued)
	}
	childID, err := agentdomain.ParseTaskID(receipt2.TaskID)
	if err != nil {
		t.Fatal(err)
	}
	childPlan, err := r.agentStore.ResumePlan(ctx, childID)
	if err != nil || childPlan.SessionRef != originalPlan.SessionRef ||
		childPlan.NativeSessionID != originalPlan.NativeSessionID {
		t.Fatalf("native session lineage was replaced: %+v err=%v", childPlan, err)
	}
	// Original completed Task remains immutable; a parent cannot be resumed
	// twice even if it carries the same native SessionRef.
	old := waitCanonicalAgentOutcome(t, r, ids["agent.result"], caller, receipt.TaskID, "completed")
	if old.Result != "agy fixture: first" || old.ContinuedFromTaskID != "" {
		t.Fatalf("original was rewritten: %+v", old)
	}
	double, err := r.CallConfiguredTool(ctx, ids["agent.resume"], prompt, caller)
	if err != nil || double.Outcome.Kind() != invocationdomain.OutcomeKindFailed ||
		double.Outcome.FailureCode() != invocationdomain.FailureToolError {
		t.Fatalf("duplicate native continuation was accepted: %+v %v", double, err)
	}
	if strings.Contains(string(submitted.Outcome.Content().Bytes()), bootstrapAgyNativeSession) ||
		strings.Contains(string(resumed.Outcome.Content().Bytes()), bootstrapAgyNativeSession) {
		t.Fatal("native SessionID leaked through canonical Agent facade")
	}
	if len(suite.Tools) != 5 || profile.Revision() == 0 {
		t.Fatal("tool identity or profile revision unexpectedly changed")
	}
}

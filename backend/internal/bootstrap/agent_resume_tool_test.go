package bootstrap

import (
	"context"
	"encoding/json"
	"path/filepath"
	"strings"
	"testing"
	"time"

	agentapp "github.com/orz-i/Mender/backend/internal/agent/application"
	agentdomain "github.com/orz-i/Mender/backend/internal/agent/domain"
	catalogdomain "github.com/orz-i/Mender/backend/internal/catalog/domain"
	gov "github.com/orz-i/Mender/backend/internal/governance/domain"
	invocationdomain "github.com/orz-i/Mender/backend/internal/invocation/domain"
	policydomain "github.com/orz-i/Mender/backend/internal/policy/domain"
	profileapp "github.com/orz-i/Mender/backend/internal/profile/application"
	profiledomain "github.com/orz-i/Mender/backend/internal/profile/domain"
)

// This test executor emits protocol-shaped evidence, never inference, shell
// access, native SDK authentication or reads against the owner's workspace.
type canonicalResumeFixture struct{}

func (canonicalResumeFixture) ExecuteConfigured(_ context.Context, _ agentdomain.Agent, prompt string,
	cfg agentapp.RunConfig) (string, error) {
	event := agentdomain.ExecutionEvent{Kind: agentdomain.EventSessionReady,
		NativeSession: "private-native-protocol-id", CanLoad: true, CanResume: true}
	if cfg.Resume != nil {
		event.NativeSession = cfg.Resume.NativeSessionID
		event.ExistingSessionRef = cfg.Resume.SessionRef
	}
	for _, e := range []agentdomain.ExecutionEvent{
		{Kind: agentdomain.EventProcessStarted, ProcessPID: 16789},
		event,
		{Kind: agentdomain.EventPromptSent},
		{Kind: agentdomain.EventTerminalSeen},
		{Kind: agentdomain.EventLeaderReaped},
	} {
		if err := cfg.Observe(e); err != nil {
			return "", err
		}
	}
	return "fixture: " + prompt, nil
}

func TestAgentResumeCanonicalToolRequiresExplicitProfileBindingAndExactLineage(t *testing.T) {
	ctx := context.Background()
	r, err := NewRuntime(ctx, RuntimeOptions{
		DatabasePath:  filepath.Join(t.TempDir(), "agent-resume.sqlite"),
		AgentExecutor: canonicalResumeFixture{},
	})
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = r.Close() }()
	owner := agentOwnerContext(t, r)
	agent, err := r.CreateAgentWithTools(ctx, agentapp.CreateRequest{
		Name: "Temporary host Agent", Backend: agentdomain.BackendOpenCode,
		WorkspaceRoot: t.TempDir(), Isolation: "host",
	}, owner)
	if err != nil {
		t.Fatal(err)
	}
	suite, err := r.AgentTools(ctx, agent.ID(), owner)
	if err != nil || suite.Status != "ready" || len(suite.Tools) != 5 {
		t.Fatalf("suite %+v %v", suite, err)
	}
	ids := map[string]catalogdomain.ToolID{}
	for _, tool := range suite.Tools {
		id, err := catalogdomain.ParseToolID(tool.ToolID)
		if err != nil {
			t.Fatal(err)
		}
		ids[tool.SourceKey] = id
	}
	for _, key := range []string{"agent.delegate", "agent.status", "agent.result", "agent.cancel", "agent.resume"} {
		if ids[key] == "" {
			t.Fatalf("missing canonical %s", key)
		}
	}
	profile, err := r.Profiles.Create.Execute(ctx, profileapp.CreateProfileRequest{
		Slug: "resume-profile", DisplayName: "Resume", Mode: profiledomain.ExposureDynamic,
	})
	if err != nil {
		t.Fatal(err)
	}
	for _, key := range []string{"agent.delegate", "agent.status", "agent.result", "agent.cancel"} {
		profile, err = r.Profiles.AddTool.Execute(ctx, profileapp.AddToolRequest{
			ProfileSlug: profile.Slug(), ToolID: ids[key],
		})
		if err != nil {
			t.Fatal(err)
		}
	}
	caller := taskServicePrincipal(t, profile.ID().String())
	if err := r.Agents.Grant(ctx, agent.ID(), profile.ID().String(), owner); err != nil {
		t.Fatal(err)
	}
	start, err := r.CallConfiguredTool(ctx, ids["agent.delegate"], []byte(`{"prompt":"original"}`), caller)
	if err != nil {
		t.Fatal(err)
	}
	var receipt struct {
		TaskID string `json:"taskId"`
	}
	if err := json.Unmarshal(start.Outcome.Content().Bytes(), &receipt); err != nil {
		t.Fatal(err)
	}
	if receipt.TaskID == "" {
		t.Fatal("no TaskID")
	}
	waitCanonicalAgentOutcome(t, r, ids["agent.result"], caller, receipt.TaskID, "completed")
	args := []byte(`{"taskId":"` + receipt.TaskID + `","prompt":"fresh"}`)
	unbound, err := r.CallConfiguredTool(ctx, ids["agent.resume"], args, caller)
	if err == nil && unbound.Outcome.Kind() == invocationdomain.OutcomeKindComplete {
		t.Fatal("new agent.resume tool auto-granted to existing Profile")
	}
	// Owner refresh must preserve the four old ToolIDs and original Profile
	// Tools; a newly discovered source is a candidate, never auto-exposed.
	refreshed, err := r.EnsureAgentTools(ctx, agent.ID(), owner)
	if err != nil {
		t.Fatal(err)
	}
	for _, item := range refreshed.Tools {
		if ids[item.SourceKey].String() != item.ToolID {
			t.Fatalf("rotated ToolID %s", item.SourceKey)
		}
	}
	_, err = r.Profiles.AddTool.Execute(ctx, profileapp.AddToolRequest{
		ProfileSlug: profile.Slug(), ToolID: ids["agent.resume"],
	})
	if err != nil {
		t.Fatal(err)
	}
	next, err := r.CallConfiguredTool(ctx, ids["agent.resume"], args, caller)
	if err != nil {
		t.Fatal(err)
	}
	if next.Outcome.Kind() != invocationdomain.OutcomeKindComplete {
		t.Fatal("no continuation receipt")
	}
	var continued struct{ TaskID, Status, ContinuedFromTaskID string }
	if err := json.Unmarshal(next.Outcome.Content().Bytes(), &continued); err != nil {
		t.Fatal(err)
	}
	if continued.TaskID == receipt.TaskID || continued.TaskID == "" ||
		continued.ContinuedFromTaskID != receipt.TaskID {
		t.Fatalf("invalid continuation %+v", continued)
	}
	output := waitCanonicalAgentOutcome(t, r, ids["agent.result"], caller, continued.TaskID, "completed")
	if output.Result != "fixture: fresh" || output.ContinuedFromTaskID != receipt.TaskID {
		t.Fatalf("wrong resumed Task %+v", output)
	}
	old := waitCanonicalAgentOutcome(t, r, ids["agent.result"], caller, receipt.TaskID, "completed")
	if old.Result != "fixture: original" || old.ContinuedFromTaskID != "" {
		t.Fatalf("original changed %+v", old)
	}
	if bytes, _ := json.Marshal(output); strings.Contains(string(bytes), "private-native-protocol-id") {
		t.Fatal("native SessionID leaked into returned Tool result")
	}
	duplicate, err := r.CallConfiguredTool(ctx, ids["agent.resume"], args, caller)
	if err == nil && duplicate.Outcome.Kind() == invocationdomain.OutcomeKindComplete {
		t.Fatal("original Task reused in a second continuation")
	}
	if _, err := r.Policy.SetDefault.Execute(ctx, policydomain.DecisionDeny); err != nil {
		t.Fatal(err)
	}
	denied, err := r.CallConfiguredTool(ctx, ids["agent.resume"], []byte(`{"taskId":"`+
		continued.TaskID+`","prompt":"denied by Policy"}`), caller)
	if err == nil && denied.Outcome.Kind() == invocationdomain.OutcomeKindComplete {
		t.Fatal("new resume bypassed canonical Policy")
	}
}

type canonicalAgentResult struct {
	Status              string `json:"status"`
	Result              string `json:"result"`
	ContinuedFromTaskID string `json:"continuedFromTaskId"`
}

func waitCanonicalAgentOutcome(t *testing.T, r *Runtime, id catalogdomain.ToolID,
	caller gov.Context, taskID, want string) canonicalAgentResult {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	var result canonicalAgentResult
	for time.Now().Before(deadline) {
		got, err := r.CallConfiguredTool(context.Background(), id, []byte(`{"taskId":"`+taskID+`"}`), caller)
		if err != nil {
			t.Fatal(err)
		}
		if got.Outcome.Kind() != invocationdomain.OutcomeKindComplete {
			t.Fatal("no canonical result")
		}
		if err := json.Unmarshal(got.Outcome.Content().Bytes(), &result); err != nil {
			t.Fatal(err)
		}
		if result.Status == want {
			return result
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatalf("task %s remained %s not %s", taskID, result.Status, want)
	return result
}

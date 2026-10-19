package httpapi

import (
	"context"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"
	"time"

	agentapp "github.com/orz-i/Mender/backend/internal/agent/application"
	agentdomain "github.com/orz-i/Mender/backend/internal/agent/domain"
	"github.com/orz-i/Mender/backend/internal/bootstrap"
	gov "github.com/orz-i/Mender/backend/internal/governance/domain"
)

type unknownWorkFixture struct{}

func (unknownWorkFixture) ExecuteConfigured(_ context.Context, _ agentdomain.Agent, _ string, config agentapp.RunConfig) (string, error) {
	for _, event := range []agentdomain.ExecutionEvent{
		{Kind: agentdomain.EventProcessStarted, ProcessPID: 1234},
		{Kind: agentdomain.EventSessionReady, NativeSession: "fixture-session", CanLoad: true},
		{Kind: agentdomain.EventPromptSent},
		{Kind: agentdomain.EventLeaderReaped},
	} {
		if err := config.Observe(event); err != nil {
			return "", err
		}
	}
	return "", agentdomain.ErrOutcomeUncertain
}

func TestOwnerHTTPUnknownHandlingNeverResumesOrRewrites(t *testing.T) {
	ctx := context.Background()
	r, err := bootstrap.NewRuntime(ctx, bootstrap.RuntimeOptions{
		DatabasePath: filepath.Join(t.TempDir(), "review-http.db"), AgentExecutor: unknownWorkFixture{},
	})
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = r.Close() }()
	server := httptest.NewServer(New(r))
	defer server.Close()
	base := server.URL + "/api/v1"
	agent := requestJSON(t, server.Client(), http.MethodPost, base+"/agents", map[string]any{
		"name": "Historical outcome fixture", "backend": "opencode", "workspaceRoot": t.TempDir(), "isolation": "host",
	}, http.StatusCreated)
	agentID, err := agentdomain.ParseID(agent["agentId"].(string))
	if err != nil {
		t.Fatal(err)
	}
	caller, err := gov.NewContext(gov.OriginManagementHTTP, r.OwnerPrincipal(), gov.CatalogScope())
	if err != nil {
		t.Fatal(err)
	}
	started, err := r.Agents.Submit(ctx, agentID, "synthetic no real work", caller)
	if err != nil {
		t.Fatal(err)
	}
	taskID := string(started.ID)
	deadline := time.Now().Add(4 * time.Second)
	var first map[string]any
	for time.Now().Before(deadline) {
		first = requestJSON(t, server.Client(), http.MethodGet, base+"/agent-tasks/"+taskID, nil, http.StatusOK)
		if first["status"] == "unknown" {
			break
		}
		time.Sleep(10 * time.Millisecond)
	}
	if first["status"] != "unknown" {
		t.Fatalf("fixture not settled: %+v", first)
	}
	if _, ok := first["disposition"]; ok {
		t.Fatal("old unknown was automatically marked handled")
	}
	requestJSON(t, server.Client(), http.MethodPost, base+"/agent-tasks/"+taskID+"/resume",
		map[string]any{"prompt": "must never be delivered"}, http.StatusConflict)
	handled := requestJSON(t, server.Client(), http.MethodPut, base+"/agent-tasks/"+taskID+"/disposition",
		map[string]any{"handling": "handled_elsewhere"}, http.StatusOK)
	if handled["status"] != "unknown" || handled["diagnostic"] != first["diagnostic"] {
		t.Fatalf("owner metadata changed original result: %+v", handled)
	}
	disposition, ok := handled["disposition"].(map[string]any)
	if !ok || disposition["handling"] != "handled_elsewhere" {
		t.Fatalf("handling missing %+v", handled)
	}
	requestJSON(t, server.Client(), http.MethodPost, base+"/agent-tasks/"+taskID+"/resume",
		map[string]any{"prompt": "still forbidden"}, http.StatusConflict)
	requestJSON(t, server.Client(), http.MethodPut, base+"/agent-tasks/"+taskID+"/disposition",
		map[string]any{"handling": "arbitrary_completed"}, http.StatusBadRequest)
	requestJSON(t, server.Client(), http.MethodPut, base+"/agent-tasks/"+taskID+"/disposition",
		map[string]any{"handling": "handled_elsewhere"}, http.StatusConflict)
	reopened := requestJSON(t, server.Client(), http.MethodPut, base+"/agent-tasks/"+taskID+"/disposition",
		map[string]any{"handling": "needs_review"}, http.StatusOK)
	if reopened["status"] != "unknown" || reopened["disposition"].(map[string]any)["handling"] != "needs_review" {
		t.Fatalf("reopening review implied success %+v", reopened)
	}
	tasks := requestJSON(t, server.Client(), http.MethodGet, base+"/agents/"+string(agentID)+"/tasks", nil, http.StatusOK)
	items := tasks["tasks"].([]any)
	if len(items) != 1 || items[0].(map[string]any)["status"] != "unknown" {
		t.Fatalf("owner acknowledgement launched Agent or minted Task %+v", tasks)
	}
}

type completedWorkFixture struct{}

func (completedWorkFixture) ExecuteConfigured(_ context.Context, _ agentdomain.Agent, prompt string, config agentapp.RunConfig) (string, error) {
	event := agentdomain.ExecutionEvent{Kind: agentdomain.EventSessionReady, NativeSession: "completed-native-id", CanLoad: true, CanResume: true}
	if config.Resume != nil {
		if config.Resume.NativeSessionID != "completed-native-id" {
			return "", agentdomain.ErrResumeUnavailable
		}
		event.ExistingSessionRef = config.Resume.SessionRef
	}
	for _, e := range []agentdomain.ExecutionEvent{
		{Kind: agentdomain.EventProcessStarted, ProcessPID: 4321},
		event,
		{Kind: agentdomain.EventPromptSent},
		{Kind: agentdomain.EventTerminalSeen},
		{Kind: agentdomain.EventLeaderReaped},
	} {
		if err := config.Observe(e); err != nil {
			return "", err
		}
	}
	return "finished: " + prompt, nil
}

func TestOwnerHTTPResumeRequiresNewPromptAndNeverReplaysOriginal(t *testing.T) {
	ctx := context.Background()
	r, err := bootstrap.NewRuntime(ctx, bootstrap.RuntimeOptions{
		DatabasePath: filepath.Join(t.TempDir(), "resume-owner-http.db"), AgentExecutor: completedWorkFixture{},
	})
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = r.Close() }()
	server := httptest.NewServer(New(r))
	defer server.Close()
	base := server.URL + "/api/v1"
	agent := requestJSON(t, server.Client(), http.MethodPost, base+"/agents", map[string]any{
		"name": "Owner ACP completed source", "backend": "opencode", "workspaceRoot": t.TempDir(), "isolation": "host",
	}, http.StatusCreated)
	agentID, err := agentdomain.ParseID(agent["agentId"].(string))
	if err != nil {
		t.Fatal(err)
	}
	caller, err := gov.NewContext(gov.OriginManagementHTTP, r.OwnerPrincipal(), gov.CatalogScope())
	if err != nil {
		t.Fatal(err)
	}
	source, err := r.Agents.Submit(ctx, agentID, "ORIGINAL", caller)
	if err != nil {
		t.Fatal(err)
	}
	id := string(source.ID)
	deadline := time.Now().Add(4 * time.Second)
	var completed map[string]any
	for time.Now().Before(deadline) {
		completed = requestJSON(t, server.Client(), http.MethodGet, base+"/agent-tasks/"+id, nil, http.StatusOK)
		if completed["status"] == "completed" {
			break
		}
		time.Sleep(10 * time.Millisecond)
	}
	if completed["status"] != "completed" {
		t.Fatalf("source did not finish %+v", completed)
	}
	if availability, ok := completed["resumeAvailability"].(map[string]any); !ok || availability["eligible"] != true {
		t.Fatalf("missing owner-only preview %+v", completed)
	}
	requestJSON(t, server.Client(), http.MethodPost, base+"/agent-tasks/"+id+"/resume", map[string]any{"prompt": ""}, http.StatusBadRequest)
	resumed := requestJSON(t, server.Client(), http.MethodPost, base+"/agent-tasks/"+id+"/resume",
		map[string]any{"prompt": "NEW ONLY"}, http.StatusAccepted)
	nextID, ok := resumed["taskId"].(string)
	if !ok || nextID == id || resumed["continuedFromTaskId"] != id {
		t.Fatalf("unsafe lineage %+v", resumed)
	}
	for time.Now().Before(deadline.Add(4 * time.Second)) {
		resumed = requestJSON(t, server.Client(), http.MethodGet, base+"/agent-tasks/"+nextID, nil, http.StatusOK)
		if resumed["status"] == "completed" {
			break
		}
		time.Sleep(10 * time.Millisecond)
	}
	if resumed["status"] != "completed" || resumed["result"] != "finished: NEW ONLY" {
		t.Fatalf("failed explicit fresh turn %+v", resumed)
	}
	sourceAfter := requestJSON(t, server.Client(), http.MethodGet, base+"/agent-tasks/"+id, nil, http.StatusOK)
	if sourceAfter["status"] != "completed" || sourceAfter["result"] != "finished: ORIGINAL" {
		t.Fatalf("original Task rewritten %+v", sourceAfter)
	}
	requestJSON(t, server.Client(), http.MethodPost, base+"/agent-tasks/"+id+"/resume",
		map[string]any{"prompt": "DO NOT DUPLICATE"}, http.StatusConflict)
}

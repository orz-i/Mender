package httpapi

import (
	"context"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"sync/atomic"
	"testing"

	agentapp "github.com/orz-i/Mender/backend/internal/agent/application"
	agentdomain "github.com/orz-i/Mender/backend/internal/agent/domain"
	"github.com/orz-i/Mender/backend/internal/bootstrap"
)

type modelInspectionFixture struct{ calls atomic.Int32 }

func (*modelInspectionFixture) Execute(context.Context, agentdomain.Agent, string) (string, error) {
	return "", nil
}
func (x *modelInspectionFixture) InspectModels(_ context.Context, _ agentdomain.Agent) (agentapp.ModelChoices, error) {
	x.calls.Add(1)
	return agentapp.ModelChoices{Current: "fixture-one", Models: []agentapp.ModelChoice{
		{ID: "fixture-one", Name: "Fixture One"}, {ID: "fixture-two", Name: "Fixture Two"},
	}, ReasoningEffort: &agentapp.OptionChoices{Current: "high", Choices: []agentapp.ModelChoice{{ID: "high", Name: "High"}, {ID: "xhigh", Name: "Extra high"}}}}, nil
}

func TestAgentModelInspectionRequiresExplicitOwnerActionWithoutTask(t *testing.T) {
	ctx := context.Background()
	inspector := &modelInspectionFixture{}
	runtime, err := bootstrap.NewRuntime(ctx, bootstrap.RuntimeOptions{
		DatabasePath: filepath.Join(t.TempDir(), "model-inspection.db"), AgentExecutor: inspector,
	})
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = runtime.Close() }()
	server := httptest.NewServer(New(runtime))
	defer server.Close()
	base := server.URL + "/api/v1"
	agent := requestJSON(t, server.Client(), http.MethodPost, base+"/agents", map[string]any{
		"name": "ACP Models Fixture", "backend": "grok", "workspaceRoot": t.TempDir(), "isolation": "host",
	}, http.StatusCreated)
	id := agent["agentId"].(string)
	requestJSON(t, server.Client(), http.MethodGet, base+"/agents", nil, http.StatusOK)
	requestJSON(t, server.Client(), http.MethodGet, base+"/agents/"+id, nil, http.StatusOK)
	if inspector.calls.Load() != 0 {
		t.Fatal("passive Agent reads launched ACP inspection")
	}
	models := requestJSON(t, server.Client(), http.MethodPost, base+"/agents/"+id+"/models/inspect", nil, http.StatusOK)
	if models["current"] != "fixture-one" || len(models["models"].([]any)) != 2 || models["reasoningEffort"].(map[string]any)["current"] != "high" {
		t.Fatalf("unexpected model projection: %+v", models)
	}
	if inspector.calls.Load() != 1 {
		t.Fatal("explicit probe was not called exactly once")
	}
	before := requestJSON(t, server.Client(), http.MethodGet, base+"/agents/"+id, nil, http.StatusOK)
	if before["defaultModel"] != "" {
		t.Fatal("capability inspection rewrote persisted default model")
	}
	tasks := requestJSON(t, server.Client(), http.MethodGet, base+"/agents/"+id+"/tasks", nil, http.StatusOK)
	if len(tasks["tasks"].([]any)) != 0 {
		t.Fatal("inspection created an Agent Task")
	}
}

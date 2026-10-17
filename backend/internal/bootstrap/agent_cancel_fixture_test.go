package bootstrap

import (
	"context"
	"encoding/json"
	"path/filepath"
	"testing"
	"time"

	agentapplication "github.com/orz-i/Mender/backend/internal/agent/application"
	agentdomain "github.com/orz-i/Mender/backend/internal/agent/domain"
	agentdelegation "github.com/orz-i/Mender/backend/internal/agent/infrastructure/delegationprovider"
	catalogapplication "github.com/orz-i/Mender/backend/internal/catalog/application"
	catalogdomain "github.com/orz-i/Mender/backend/internal/catalog/domain"
	invocationdomain "github.com/orz-i/Mender/backend/internal/invocation/domain"
	profileapplication "github.com/orz-i/Mender/backend/internal/profile/application"
	profiledomain "github.com/orz-i/Mender/backend/internal/profile/domain"
)

type waitingAgentExecutor struct{ started chan struct{} }

func (e waitingAgentExecutor) ExecuteConfigured(ctx context.Context, _ agentdomain.Agent, _ string, _ agentapplication.RunConfig) (string, error) {
	close(e.started)
	<-ctx.Done()
	return "", ctx.Err()
}

func TestAgentCanonicalTaskCancellationFixture(t *testing.T) {
	ctx := context.Background()
	executor := waitingAgentExecutor{started: make(chan struct{})}
	r, err := NewRuntime(ctx, RuntimeOptions{
		DatabasePath: filepath.Join(t.TempDir(), "cancel-fixture.db"), AgentExecutor: executor,
	})
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = r.Close() }()
	owner := agentOwnerContext(t, r)
	agent, err := r.Agents.Create(ctx, agentapplication.CreateRequest{
		Name: "Cancelable Fixture", Backend: agentdomain.BackendGrok, WorkspaceRoot: t.TempDir(),
	}, owner)
	if err != nil {
		t.Fatal(err)
	}
	config, err := json.Marshal(map[string]string{"agentId": string(agent.ID())})
	if err != nil {
		t.Fatal(err)
	}
	provider, err := registerConfiguredProviderTest(t, r, ctx, catalogapplication.RegisterProviderRequest{
		Namespace: "cancel-agent", DisplayName: "Cancelable Fixture",
	}, agentdelegation.ID, config)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := syncConfiguredProviderTest(t, r, ctx, provider.ID()); err != nil {
		t.Fatal(err)
	}
	tools, err := r.Catalog.store.ListProviderTools(ctx, provider.ID())
	if err != nil {
		t.Fatal(err)
	}
	ids := map[string]catalogdomain.ToolID{}
	for _, tool := range tools {
		ids[tool.SourceKey()] = tool.ID()
	}
	profile, err := r.Profiles.Create.Execute(ctx, profileapplication.CreateProfileRequest{
		Slug: "agent-cancel", DisplayName: "Cancelable Fixture", Mode: profiledomain.ExposureDynamic,
	})
	if err != nil {
		t.Fatal(err)
	}
	for _, key := range []string{"agent.delegate", "agent.status", "agent.cancel"} {
		profile, err = r.Profiles.AddTool.Execute(ctx, profileapplication.AddToolRequest{
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
	start, err := r.CallConfiguredTool(ctx, ids["agent.delegate"], []byte(`{"prompt":"fixture only"}`), caller)
	if err != nil || start.Outcome.Kind() != invocationdomain.OutcomeKindComplete {
		t.Fatalf("canonical task dispatch failed: %v %#v", err, start.Outcome)
	}
	var receipt struct {
		TaskID string `json:"taskId"`
	}
	if err := json.Unmarshal(start.Outcome.Content().Bytes(), &receipt); err != nil {
		t.Fatal(err)
	}
	select {
	case <-executor.started:
	case <-time.After(5 * time.Second):
		t.Fatal("fake Agent was not invoked")
	}
	stopped, err := r.CallConfiguredTool(ctx, ids["agent.cancel"], []byte(`{"taskId":"`+receipt.TaskID+`"}`), caller)
	if err != nil || stopped.Outcome.Kind() != invocationdomain.OutcomeKindComplete {
		t.Fatalf("canonical cancel refused: %v %#v", err, stopped.Outcome)
	}
	deadline := time.Now().Add(5 * time.Second)
	for {
		task, err := r.Agents.Task(ctx, agentdomain.TaskID(receipt.TaskID), owner)
		if err != nil {
			t.Fatal(err)
		}
		if task.Status == agentdomain.TaskCancelled {
			if !task.CancelRequested {
				t.Fatal("cancel bit was not persisted")
			}
			break
		}
		if time.Now().After(deadline) {
			t.Fatalf("task did not confirm cancellation: %+v", task)
		}
		time.Sleep(10 * time.Millisecond)
	}
	events, err := r.agentStore.ListAdmissions(ctx, agent.ID(), 20)
	if err != nil {
		t.Fatal(err)
	}
	var canceled bool
	for _, event := range events {
		if event.Action == "agent.task.cancel" && event.Status == "admitted" {
			canceled = true
		}
	}
	if !canceled {
		t.Fatalf("task cancel governance was not recorded: %+v", events)
	}
}

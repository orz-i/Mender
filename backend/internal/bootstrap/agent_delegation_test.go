package bootstrap

import (
	"context"
	"encoding/json"
	"errors"
	"path/filepath"
	"testing"
	"time"

	agentapplication "github.com/orz-i/Mender/backend/internal/agent/application"
	agentdomain "github.com/orz-i/Mender/backend/internal/agent/domain"
	agentdelegation "github.com/orz-i/Mender/backend/internal/agent/infrastructure/delegationprovider"
	catalogapplication "github.com/orz-i/Mender/backend/internal/catalog/application"
	catalogdomain "github.com/orz-i/Mender/backend/internal/catalog/domain"
	gov "github.com/orz-i/Mender/backend/internal/governance/domain"
	invocationdomain "github.com/orz-i/Mender/backend/internal/invocation/domain"
	profileapplication "github.com/orz-i/Mender/backend/internal/profile/application"
	profiledomain "github.com/orz-i/Mender/backend/internal/profile/domain"
)

type fixtureAgentExecutor struct{}

func (fixtureAgentExecutor) Execute(_ context.Context, _ agentdomain.Agent, prompt string) (string, error) {
	return "fixture: " + prompt, nil
}
func agentOwnerContext(t *testing.T, r *Runtime) gov.Context {
	t.Helper()
	owner, err := gov.NewContext(gov.OriginCLI, r.OwnerPrincipal(), gov.CatalogScope())
	if err != nil {
		t.Fatal(err)
	}
	return owner
}
func taskServicePrincipal(t *testing.T, profileID string) gov.Context {
	t.Helper()
	p, err := gov.NewPrincipal(gov.PrincipalService, "profile:"+profileID)
	if err != nil {
		t.Fatal(err)
	}
	scope, err := gov.ProfileScope(profileID)
	if err != nil {
		t.Fatal(err)
	}
	c, err := gov.NewContext(gov.OriginMCP, p, scope)
	if err != nil {
		t.Fatal(err)
	}
	return c
}

func TestGovernedCanonicalAgentDelegationToolFixture(t *testing.T) {
	ctx := context.Background()
	r, err := NewRuntime(ctx, RuntimeOptions{
		DatabasePath:  filepath.Join(t.TempDir(), "mender.sqlite"),
		AgentExecutor: fixtureAgentExecutor{},
	})
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = r.Close() }()
	owner := agentOwnerContext(t, r)
	a, err := r.Agents.Create(ctx, agentapplication.CreateRequest{
		Name: "Code Reviewer", Backend: agentdomain.BackendCodex, WorkspaceRoot: t.TempDir(),
	}, owner)
	if err != nil {
		t.Fatal(err)
	}
	config, err := json.Marshal(map[string]string{"agentId": string(a.ID())})
	if err != nil {
		t.Fatal(err)
	}
	provider, err := registerConfiguredProviderTest(t, r, ctx, catalogapplication.RegisterProviderRequest{
		Namespace: "agent-reviewer", DisplayName: "Code Reviewer",
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
	if len(ids) != 4 || ids["agent.delegate"] == "" || ids["agent.status"] == "" {
		t.Fatalf("Agent facade Tools = %v", ids)
	}

	p, err := r.Profiles.Create.Execute(ctx, profileapplication.CreateProfileRequest{
		Slug: "agent-test-a", DisplayName: "Agent Test A", Mode: profiledomain.ExposureDynamic,
	})
	if err != nil {
		t.Fatal(err)
	}
	for _, key := range []string{"agent.delegate", "agent.status", "agent.result", "agent.cancel"} {
		p, err = r.Profiles.AddTool.Execute(ctx, profileapplication.AddToolRequest{ProfileSlug: p.Slug(), ToolID: ids[key]})
		if err != nil {
			t.Fatal(err)
		}
	}
	caller := taskServicePrincipal(t, p.ID().String())
	if _, err := r.Agents.Submit(ctx, a.ID(), "must be denied", caller); !errors.Is(err, agentdomain.ErrForbidden) {
		t.Fatalf("missing Agent grant allowed submission: %v", err)
	}
	// Profile membership by itself is insufficient. Add an exact grant.
	if err := r.Agents.Grant(ctx, a.ID(), p.ID().String(), owner); err != nil {
		t.Fatal(err)
	}
	resp, err := r.CallConfiguredTool(ctx, ids["agent.delegate"], []byte(`{"prompt":"review code"}`), caller)
	if err != nil {
		t.Fatal(err)
	}
	if resp.Outcome.Kind() != invocationdomain.OutcomeKindComplete {
		t.Fatalf("unexpected async Task outcome: %v", resp.Outcome.Kind())
	}
	var receipt struct {
		TaskID string `json:"taskId"`
		Status string `json:"status"`
	}
	if err := json.Unmarshal(resp.Outcome.Content().Bytes(), &receipt); err != nil {
		t.Fatal(err)
	}
	if _, err := agentdomain.ParseTaskID(receipt.TaskID); err != nil {
		t.Fatal(err)
	}
	var last struct {
		Status string `json:"status"`
		Result string `json:"result"`
	}
	deadline := time.Now().Add(4 * time.Second)
	for time.Now().Before(deadline) {
		got, err := r.CallConfiguredTool(ctx, ids["agent.result"], []byte(`{"taskId":"`+receipt.TaskID+`"}`), caller)
		if err != nil {
			t.Fatal(err)
		}
		if got.Outcome.Kind() != invocationdomain.OutcomeKindComplete {
			t.Fatalf("result outcome: %v", got.Outcome.Kind())
		}
		if err := json.Unmarshal(got.Outcome.Content().Bytes(), &last); err != nil {
			t.Fatal(err)
		}
		if last.Status == "completed" {
			break
		}
		time.Sleep(10 * time.Millisecond)
	}
	if last.Status != "completed" || last.Result != "fixture: review code" {
		t.Fatalf("delegation result: %+v", last)
	}
	// Same TaskID under another Profile service must not reveal results.
	p2, err := r.Profiles.Create.Execute(ctx, profileapplication.CreateProfileRequest{
		Slug: "agent-test-b", DisplayName: "Agent Test B", Mode: profiledomain.ExposureDynamic,
	})
	if err != nil {
		t.Fatal(err)
	}
	p2, err = r.Profiles.AddTool.Execute(ctx, profileapplication.AddToolRequest{
		ProfileSlug: p2.Slug(), ToolID: ids["agent.result"]})
	if err != nil {
		t.Fatal(err)
	}
	outsider := taskServicePrincipal(t, p2.ID().String())
	reply, err := r.CallConfiguredTool(ctx, ids["agent.result"], []byte(`{"taskId":"`+receipt.TaskID+`"}`), outsider)
	if err == nil && reply.Outcome.Kind() == invocationdomain.OutcomeKindComplete {
		t.Fatal("cross-Profile Task disclosed through canonical Tool")
	}
	if err := r.Agents.Revoke(ctx, a.ID(), p.ID().String(), owner); err != nil {
		t.Fatal(err)
	}
	denied, err := r.CallConfiguredTool(ctx, ids["agent.delegate"], []byte(`{"prompt":"denied after revocation"}`), caller)
	if err == nil && denied.Outcome.Kind() == invocationdomain.OutcomeKindComplete {
		t.Fatal("Agent task submitted after grant revocation")
	}
}

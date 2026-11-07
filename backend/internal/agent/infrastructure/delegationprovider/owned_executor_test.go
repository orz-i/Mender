package delegationprovider

import (
	"context"
	"errors"
	"path/filepath"
	"testing"
	"time"

	agentapp "github.com/orz-i/Mender/backend/internal/agent/application"
	agent "github.com/orz-i/Mender/backend/internal/agent/domain"
	agentworkspace "github.com/orz-i/Mender/backend/internal/agent/infrastructure/localworkspace"
	agentsqlite "github.com/orz-i/Mender/backend/internal/agent/infrastructure/sqlite"
	catalog "github.com/orz-i/Mender/backend/internal/catalog/domain"
	gov "github.com/orz-i/Mender/backend/internal/governance/domain"
	invocationapp "github.com/orz-i/Mender/backend/internal/invocation/application"
	invocation "github.com/orz-i/Mender/backend/internal/invocation/domain"
)

func TestOwnedExecutorDoesNotRouteAcrossAgentSources(t *testing.T) {
	ctx := context.Background()
	store, err := agentsqlite.Open(ctx, filepath.Join(t.TempDir(), "mender.sqlite"))
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = store.Close() }()
	owner, err := gov.NewPrincipal(gov.PrincipalLocalUser, "native-agent-tool-test")
	if err != nil {
		t.Fatal(err)
	}
	caller, err := gov.NewContext(gov.OriginInternal, owner, gov.CatalogScope())
	if err != nil {
		t.Fatal(err)
	}
	service, err := agentapp.NewService(store, inertRunner{}, func(context.Context, gov.Context, agent.Agent, gov.Action) error { return nil },
		owner, nil, agentworkspace.New())
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = service.Close() }()
	first, err := service.Create(ctx, agentapp.CreateRequest{Name: "First", Backend: agent.BackendCodex, WorkspaceRoot: t.TempDir(), Isolation: "host"}, caller)
	if err != nil {
		t.Fatal(err)
	}
	second, err := service.Create(ctx, agentapp.CreateRequest{Name: "Second", Backend: agent.BackendCodex, WorkspaceRoot: t.TempDir(), Isolation: "host"}, caller)
	if err != nil {
		t.Fatal(err)
	}
	executor, err := NewOwnedExecutor(service, first.ID())
	if err != nil {
		t.Fatal(err)
	}
	if _, err := NewOwnedExecutor(nil, first.ID()); err == nil {
		t.Fatal("nil Task service allowed")
	}
	if _, err := NewOwnedExecutor(service, agent.ID("invalid")); err == nil {
		t.Fatal("unverified AgentID allowed")
	}
	create := func(id agent.ID) catalog.Tool {
		t.Helper()
		source, err := catalog.AgentSource(string(id))
		if err != nil {
			t.Fatal(err)
		}
		schema, err := catalog.NewSchema(submitInput)
		if err != nil {
			t.Fatal(err)
		}
		key := "tool_aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"
		if id == second.ID() {
			key = "tool_bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb"
		}
		toolID, err := catalog.ParseToolID(key)
		if err != nil {
			t.Fatal(err)
		}
		tool, err := catalog.NewToolFromSource(toolID, source,
			catalog.ToolSpec{SourceKey: sourceDelegate, Name: "delegate", InputSchema: schema}, time.Now().UTC())
		if err != nil {
			t.Fatal(err)
		}
		return tool
	}
	args, err := invocation.NewJSONValue([]byte(`{"prompt":"check facts only"}`))
	if err != nil {
		t.Fatal(err)
	}
	req := invocationapp.ExecutionRequest{Tool: create(second.ID()), Context: caller, Arguments: args}
	if _, err := executor.Execute(ctx, req, nil, nil); !errors.Is(err, agent.ErrForbidden) {
		t.Fatalf("Agent B Tool could execute as Agent A: %v", err)
	}
	if _, err := executor.Extract(ctx, req.Tool, req.Arguments, caller); !errors.Is(err, agent.ErrForbidden) {
		t.Fatalf("Agent B Policy facts accepted as Agent A: %v", err)
	}
	req.Tool = create(first.ID())
	reviewed, err := executor.Extract(ctx, req.Tool, req.Arguments, caller)
	if err != nil || reviewed.Target != nil || reviewed.Facts.Knowledge() != invocation.FactKnowledgeKnown ||
		len(reviewed.Facts.Resources()) != 1 || reviewed.Facts.Resources()[0].Value() != string(first.ID()) ||
		len(reviewed.Facts.Effects()) != 1 || string(reviewed.Facts.Effects()[0]) != sourceDelegate {
		t.Fatalf("owned Agent as Tool Policy facts lost Agent identity: %+v %v", reviewed, err)
	}
	out, err := executor.Execute(ctx, req, nil, nil)
	if err != nil || out.Kind() != invocation.OutcomeKindComplete {
		t.Fatalf("owned Agent as Tool not delegated: result=%+v err=%v", out, err)
	}
}

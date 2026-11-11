package delegationprovider

import (
	"bytes"
	"context"
	"encoding/json"
	"path/filepath"
	"testing"
	"time"

	agentapp "github.com/orz-i/Mender/backend/internal/agent/application"
	agentdomain "github.com/orz-i/Mender/backend/internal/agent/domain"
	agentworkspace "github.com/orz-i/Mender/backend/internal/agent/infrastructure/localworkspace"
	catalogdomain "github.com/orz-i/Mender/backend/internal/catalog/domain"
	gov "github.com/orz-i/Mender/backend/internal/governance/domain"
	invocationapp "github.com/orz-i/Mender/backend/internal/invocation/application"
	invocationdomain "github.com/orz-i/Mender/backend/internal/invocation/domain"
	providerplugin "github.com/orz-i/Mender/backend/internal/providerplugin"
)

func TestDelegateExplicitTaskTitleEndToEndWithoutPersistingPrompt(t *testing.T) {
	ctx := context.Background()
	store, err := openAgentV3(t, ctx, filepath.Join(t.TempDir(), "agent.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = store.Close() }()
	principal, err := gov.NewPrincipal(gov.PrincipalLocalUser, "os:fixture")
	if err != nil {
		t.Fatal(err)
	}
	caller, err := gov.NewContext(gov.OriginInternal, principal, gov.CatalogScope())
	if err != nil {
		t.Fatal(err)
	}
	svc, err := agentapp.NewService(store, inertRunner{},
		func(context.Context, gov.Context, agentdomain.Agent, gov.Action) error { return nil }, principal, nil, agentworkspace.New())
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = svc.Close() }()
	agent, err := svc.Create(ctx, agentapp.CreateRequest{
		Name: "Label test", Backend: agentdomain.BackendGrok, WorkspaceRoot: t.TempDir(), Isolation: "host",
	}, caller)
	if err != nil {
		t.Fatal(err)
	}
	instance, err := New(svc, principal).Open(ctx, providerplugin.OpenRequest{
		ConfigVersion: ConfigVersion, Config: []byte(`{"agentId":"` + string(agent.ID()) + `"}`),
	})
	if err != nil {
		t.Fatal(err)
	}
	input, err := catalogdomain.NewSchema(submitInput)
	if err != nil {
		t.Fatal(err)
	}
	output, err := catalogdomain.NewSchema(submitOutput)
	if err != nil {
		t.Fatal(err)
	}
	source, _ := catalogdomain.AgentSource("agent_00000000000000000000000000000002")
	tid, _ := catalogdomain.ParseToolID("tool_00000000000000000000000000000002")
	tool, err := catalogdomain.NewToolFromSource(tid, source, catalogdomain.ToolSpec{
		SourceKey: sourceDelegate, Name: "delegate", Title: "Delegate", Description: "Fixture",
		InputSchema: input, OutputSchema: &output,
	}, time.Now())
	if err != nil {
		t.Fatal(err)
	}
	const prompt = "PRIVATE_TOOL_PROMPT_NO_PERSISTENCE"
	const title = "验证任务标题显示"
	arguments, err := invocationdomain.NewJSONValue([]byte(`{"prompt":"` + prompt + `","title":"` + title + `"}`))
	if err != nil {
		t.Fatal(err)
	}
	outcome, err := instance.Execute(ctx, invocationapp.ExecutionRequest{
		Tool: tool, Arguments: arguments, Context: caller,
	}, nil, nil)
	if err != nil || outcome.Kind() != invocationdomain.OutcomeKindComplete {
		t.Fatalf("delegate rejected explicit title: %+v %v", outcome, err)
	}
	tasks, err := svc.Tasks(ctx, agent.ID(), caller)
	if err != nil || len(tasks) != 1 || tasks[0].Title != title {
		t.Fatalf("title not present in Owner Task inventory: %+v %v", tasks, err)
	}
	b, err := json.Marshal(tasks[0])
	if err != nil || !json.Valid(b) || string(b) == "" {
		t.Fatalf("invalid Task JSON %s %v", b, err)
	}
	if json.Valid(b) && (bytes.Contains(b, []byte(prompt)) || !bytes.Contains(b, []byte(title))) {
		t.Fatalf("Task JSON exposed input prompt or lost title: %s", b)
	}
}

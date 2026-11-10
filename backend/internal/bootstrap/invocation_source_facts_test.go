package bootstrap

import (
	"context"
	"errors"
	"testing"
	"time"

	catalog "github.com/orz-i/Mender/backend/internal/catalog/domain"
	governance "github.com/orz-i/Mender/backend/internal/governance/domain"
	invocation "github.com/orz-i/Mender/backend/internal/invocation/application"
	invocationdomain "github.com/orz-i/Mender/backend/internal/invocation/domain"
	router "github.com/orz-i/Mender/backend/internal/invocation/infrastructure/router"
	providerplugin "github.com/orz-i/Mender/backend/internal/providerplugin"
	providerconfig "github.com/orz-i/Mender/backend/internal/providerplugin/configstore"
	v3fixture "github.com/orz-i/Mender/backend/internal/testfixtures/v3"
)

type reviewedAgentFactExecutor struct{ calls int }

func (*reviewedAgentFactExecutor) Execute(context.Context, invocation.ExecutionRequest,
	invocation.CredentialResolver, invocation.ProgressReporter,
) (invocationdomain.Outcome, error) {
	return invocationdomain.Outcome{}, errors.New("fact extraction must not execute a Tool")
}
func (e *reviewedAgentFactExecutor) Extract(_ context.Context, tool catalog.Tool,
	_ invocationdomain.JSONValue, caller governance.Context,
) (invocation.GovernanceFactExtraction, error) {
	if err := caller.Validate(); err != nil {
		return invocation.GovernanceFactExtraction{}, err
	}
	e.calls++
	resource, err := invocationdomain.NewResourceFact("agent_id", tool.SourceRef().ID())
	if err != nil {
		return invocation.GovernanceFactExtraction{}, err
	}
	facts, err := invocationdomain.KnownGovernanceFacts(
		[]invocationdomain.EffectFact{"agent.delegate"}, []invocationdomain.ResourceFact{resource},
	)
	return invocation.GovernanceFactExtraction{Facts: facts}, err
}

func TestHostSourceFactRoutingUsesOnlyRegisteredAgentAndNotProviderExecutor(t *testing.T) {
	ctx := context.Background()
	store, err := providerconfig.OpenTyped(ctx, v3fixture.EmptyDatabase(t))
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = store.Close() }()
	executors := router.New()
	agentA, _ := catalog.AgentSource("agent_11111111111111111111111111111111")
	agentB, _ := catalog.AgentSource("agent_22222222222222222222222222222222")
	providerID, _ := catalog.ParseProviderID("prov_33333333333333333333333333333333")
	providerSource, _ := catalog.ProviderSource(providerID)
	schema, err := catalog.NewSchema([]byte(`{"type":"object"}`))
	if err != nil {
		t.Fatal(err)
	}
	makeTool := func(id string, src catalog.SourceRef, key string) catalog.Tool {
		t.Helper()
		tid, err := catalog.ParseToolID(id)
		if err != nil {
			t.Fatal(err)
		}
		tool, err := catalog.NewToolFromSource(tid, src,
			catalog.ToolSpec{SourceKey: key, Name: key, InputSchema: schema}, time.Now().UTC())
		if err != nil {
			t.Fatal(err)
		}
		return tool
	}
	first := makeTool("tool_11111111111111111111111111111111", agentA, "agent.delegate")
	second := makeTool("tool_22222222222222222222222222222222", agentB, "agent.delegate")
	external := makeTool("tool_33333333333333333333333333333333", providerSource, "external.call")
	principal, err := governance.NewPrincipal(governance.PrincipalLocalUser, "reviewer")
	if err != nil {
		t.Fatal(err)
	}
	caller, err := governance.NewContext(governance.OriginCLI, principal, governance.CatalogScope())
	if err != nil {
		t.Fatal(err)
	}
	args, _ := invocationdomain.NewJSONValue([]byte("{}"))
	executor := &reviewedAgentFactExecutor{}
	if err := executors.RegisterSource(agentA, executor); err != nil {
		t.Fatal(err)
	}
	if err := executors.RegisterSource(providerSource, executor); err != nil {
		t.Fatal(err)
	}
	extractor := newInvocationGovernanceFactExtractor(store, providerplugin.NewRegistry(), executors)
	got, err := extractor.Extract(ctx, first, args, caller)
	if err != nil || got.Facts.Knowledge() != invocationdomain.FactKnowledgeKnown ||
		got.Facts.Resources()[0].Value() != agentA.ID() || executor.calls != 1 {
		t.Fatalf("registered Agent fact route failed: %+v err=%v calls=%d", got, err, executor.calls)
	}
	if _, err := extractor.Extract(ctx, second, args, caller); !errors.Is(err, invocation.ErrExecutorNotFound) {
		t.Fatalf("Agent B borrowed Agent A fact extractor: %v", err)
	}
	// Even an executor registered under a Provider SourceRef may not forge
	// policy facts: Provider facts require host-reviewed Plugin configuration.
	providerFacts, err := extractor.Extract(ctx, external, args, caller)
	if err != nil || providerFacts.Facts.Knowledge() != invocationdomain.FactKnowledgeUnknown || executor.calls != 1 {
		t.Fatalf("Provider facts bypassed host config: %+v err=%v calls=%d", providerFacts, err, executor.calls)
	}
}

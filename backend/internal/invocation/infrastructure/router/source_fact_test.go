package router

import (
	"context"
	"errors"
	"testing"

	catalog "github.com/orz-i/Mender/backend/internal/catalog/domain"
	governance "github.com/orz-i/Mender/backend/internal/governance/domain"
	invocationapp "github.com/orz-i/Mender/backend/internal/invocation/application"
	invocation "github.com/orz-i/Mender/backend/internal/invocation/domain"
)

type factExecutor struct {
	labelledExecutor
	calls int
}

func (e *factExecutor) Extract(_ context.Context, tool catalog.Tool, _ invocation.JSONValue,
	caller governance.Context,
) (invocationapp.GovernanceFactExtraction, error) {
	e.calls++
	if caller.Validate() != nil {
		return invocationapp.GovernanceFactExtraction{}, errors.New("invalid caller")
	}
	fact, err := invocation.NewResourceFact("agent_id", tool.SourceRef().ID())
	if err != nil {
		return invocationapp.GovernanceFactExtraction{}, err
	}
	facts, err := invocation.KnownGovernanceFacts([]invocation.EffectFact{"agent.delegate"}, []invocation.ResourceFact{fact})
	return invocationapp.GovernanceFactExtraction{Facts: facts}, err
}

func TestRouterFactExtractionUsesExactRegisteredSourceExecutor(t *testing.T) {
	first, err := catalog.AgentSource("agent_11111111111111111111111111111111")
	if err != nil {
		t.Fatal(err)
	}
	second, err := catalog.AgentSource("agent_22222222222222222222222222222222")
	if err != nil {
		t.Fatal(err)
	}
	toolA := sourceTool(t, "tool_aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa", first, "agent.delegate")
	toolB := sourceTool(t, "tool_bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb", second, "agent.delegate")
	principal, err := governance.NewPrincipal(governance.PrincipalLocalUser, "test")
	if err != nil {
		t.Fatal(err)
	}
	caller, err := governance.NewContext(governance.OriginCLI, principal, governance.CatalogScope())
	if err != nil {
		t.Fatal(err)
	}
	args, err := invocation.NewJSONValue([]byte(`{}`))
	if err != nil {
		t.Fatal(err)
	}
	r := New()
	agentA := &factExecutor{}
	if err := r.RegisterSource(first, agentA); err != nil {
		t.Fatal(err)
	}
	if _, err := r.Extract(context.Background(), toolB, args, caller); !errors.Is(err, invocationapp.ErrExecutorNotFound) {
		t.Fatalf("other Agent borrowed facts provider: %v", err)
	}
	if agentA.calls != 0 {
		t.Fatal("mismatched source executed fact extractor")
	}
	projected, err := r.Extract(context.Background(), toolA, args, caller)
	if err != nil || projected.Facts.Knowledge() != invocation.FactKnowledgeKnown || agentA.calls != 1 ||
		projected.Facts.Resources()[0].Value() != first.ID() {
		t.Fatalf("Agent source facts mismatch: facts=%+v err=%v calls=%d", projected, err, agentA.calls)
	}
	provider, err := catalog.ProviderSource(catalog.ProviderID("prov_33333333333333333333333333333333"))
	if err != nil {
		t.Fatal(err)
	}
	providerTool := sourceTool(t, "tool_33333333333333333333333333333333", provider, "external.call")
	if err := r.RegisterSource(provider, &factExecutor{}); err != nil {
		t.Fatal(err)
	}
	if _, err := r.Extract(context.Background(), providerTool, args, caller); err == nil {
		t.Fatal("external Provider executor forged trusted policy facts")
	}
	if err := r.RegisterSource(second, &labelledExecutor{id: "unreviewed"}); err != nil {
		t.Fatal(err)
	}
	if _, err := r.Extract(context.Background(), toolB, args, caller); err == nil {
		t.Fatal("executor without a trusted fact extractor forged Agent Policy facts")
	}
}

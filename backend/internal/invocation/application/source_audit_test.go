package application

import (
	"context"
	"testing"
	"time"

	catalog "github.com/orz-i/Mender/backend/internal/catalog/domain"
	invocation "github.com/orz-i/Mender/backend/internal/invocation/domain"
)

type auditedTarget struct{ facts invocation.GovernanceFacts }

func (t auditedTarget) Facts() invocation.GovernanceFacts { return t.facts }

func TestAgentAndNativeInvocationAuditNeverFabricatesProvider(t *testing.T) {
	cases := []struct {
		kind     string
		sourceID string
		key      string
		toolID   string
	}{
		{"agent", "agent_aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa", "agent.delegate", "tool_aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"},
		{"native", "workspace", "workspace.read", "tool_882fb9c25db58744f34da684f7f7ab5f"},
	}
	for _, tc := range cases {
		t.Run(tc.kind, func(t *testing.T) {
			f := newFixture(t)
			source, err := catalog.NewSourceRef(catalog.SourceKind(tc.kind), tc.sourceID)
			if err != nil {
				t.Fatal(err)
			}
			id, err := catalog.ParseToolID(tc.toolID)
			if err != nil {
				t.Fatal(err)
			}
			item, err := catalog.NewToolFromSource(id, source, catalog.ToolSpec{
				SourceKey: tc.key, Name: "invoke", InputSchema: f.tool.InputSchema(),
			}, time.Now().UTC())
			if err != nil {
				t.Fatal(err)
			}
			f.tools.tool = item
			if tc.kind == "native" {
				resources, err := invocation.NewResourceFact("workspace_path", "a")
				if err != nil {
					t.Fatal(err)
				}
				facts, err := invocation.KnownGovernanceFacts([]invocation.EffectFact{"filesystem.read"}, []invocation.ResourceFact{resources})
				if err != nil {
					t.Fatal(err)
				}
				f.facts.facts = facts
				f.facts.target = auditedTarget{facts}
			}
			response, err := f.useCase.Execute(context.Background(), InvokeToolRequest{
				ToolID: item.ID(), Arguments: []byte(`{}`),
				Context: catalogInvocationContext(t, "typed-audit"),
			})
			if err != nil || response.Outcome.Kind() != invocation.OutcomeKindComplete {
				t.Fatalf("typed invocation was not audited: %+v %v", response, err)
			}
			record := f.audit.last(t)
			if record.ProviderID() != nil || record.ToolID() != item.ID() {
				t.Fatalf("non-Provider Tool fabricated a Provider audit: provider=%v tool=%v", record.ProviderID(), record.ToolID())
			}
		})
	}
}

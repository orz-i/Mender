package application

import (
	"context"
	"testing"
	"time"

	catalog "github.com/orz-i/Mender/backend/internal/catalog/domain"
	invocation "github.com/orz-i/Mender/backend/internal/invocation/domain"
)

func TestNativeExecutionTargetIsRequiredAndMatchesPolicyFacts(t *testing.T) {
	for _, tc := range []struct {
		name         string
		sourceKind   catalog.SourceKind
		sourceID     string
		target       bool
		mismatch     bool
		shouldPermit bool
	}{
		{"native missing target", catalog.SourceNative, "workspace", false, false, false},
		{"native mismatched facts", catalog.SourceNative, "workspace", true, true, false},
		{"native exact frozen facts", catalog.SourceNative, "workspace", true, false, true},
		{"provider target injected", catalog.SourceProvider, "prov_aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa", true, false, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			f := newFixture(t)
			source, err := catalog.NewSourceRef(tc.sourceKind, tc.sourceID)
			if err != nil {
				t.Fatal(err)
			}
			id, err := catalog.ParseToolID("tool_882fb9c25db58744f34da684f7f7ab5f")
			if err != nil {
				t.Fatal(err)
			}
			tool, err := catalog.NewToolFromSource(id, source,
				catalog.ToolSpec{SourceKey: "workspace.read", Name: "read", InputSchema: f.tool.InputSchema()},
				time.Now().UTC(),
			)
			if err != nil {
				t.Fatal(err)
			}
			f.tools.tool = tool
			resource, err := invocation.NewResourceFact("workspace_id", "wrk_aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa")
			if err != nil {
				t.Fatal(err)
			}
			facts, err := invocation.KnownGovernanceFacts(
				[]invocation.EffectFact{"filesystem.read"}, []invocation.ResourceFact{resource},
			)
			if err != nil {
				t.Fatal(err)
			}
			f.facts.facts = facts
			if tc.target {
				targetFacts := facts
				if tc.mismatch {
					different, err := invocation.NewResourceFact("workspace_id", "wrk_bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb")
					if err != nil {
						t.Fatal(err)
					}
					targetFacts, err = invocation.KnownGovernanceFacts(
						[]invocation.EffectFact{"filesystem.read"}, []invocation.ResourceFact{different},
					)
					if err != nil {
						t.Fatal(err)
					}
				}
				f.facts.target = &auditedTarget{facts: targetFacts}
			}
			response, err := f.useCase.Execute(context.Background(), InvokeToolRequest{
				ToolID: tool.ID(), Arguments: []byte("{}"), Context: catalogInvocationContext(t, "native-target"),
			})
			if tc.shouldPermit {
				if err != nil || response.Outcome.Kind() != invocation.OutcomeKindComplete ||
					f.executor.calls != 1 {
					t.Fatalf("expected execution: response=%+v err=%v calls=%d", response, err, f.executor.calls)
				}
				if f.executor.last.Target != f.facts.target {
					t.Fatal("executor lost the same pre-Policy frozen target")
				}
			} else {
				if err == nil || f.executor.calls != 0 || f.policy.calls != 0 ||
					f.runtimeBudget.calls != 0 {
					t.Fatalf("unverified target reached admission/execution: response=%+v err=%v policy=%d budget=%d executor=%d",
						response, err, f.policy.calls, f.runtimeBudget.calls, f.executor.calls)
				}
			}
		})
	}
}

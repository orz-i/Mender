package application

import (
	"context"
	"strings"
	"testing"
	"time"

	policydomain "github.com/orz-i/Mender/backend/internal/policy/domain"
)

func TestEvaluateMapsBoundedRequestIntoConfiguredPolicy(t *testing.T) {
	t.Parallel()

	repository := newMemoryRepository(t)
	resource, err := policydomain.NewResourceMatcher(
		"path",
		policydomain.ResourcePrefix,
		"/workspace/release",
	)
	if err != nil {
		t.Fatal(err)
	}
	match, err := policydomain.NewMatch(policydomain.MatchSpec{
		Origins:       []policydomain.Origin{policydomain.OriginMCP},
		FactKnowledge: policydomain.FactKnown,
		EffectsAny:    []string{"filesystem.write"},
		ResourcesAny:  []policydomain.ResourceMatcher{resource},
	})
	if err != nil {
		t.Fatal(err)
	}
	rule, err := policydomain.NewRule(
		mustRuleID(t, "rule_00000000000000000000000000000001"),
		"Release writes",
		true,
		policydomain.DecisionDeny,
		match,
		time.Date(2026, 9, 27, 1, 0, 0, 0, time.UTC),
	)
	if err != nil {
		t.Fatal(err)
	}
	if err := repository.CreateRule(context.Background(), rule); err != nil {
		t.Fatal(err)
	}

	decision, err := NewEvaluate(repository).Execute(
		context.Background(),
		EvaluateRequest{
			Origin:        policydomain.OriginMCP,
			ScopeKind:     policydomain.ScopeCatalog,
			ProviderID:    "prov_00000000000000000000000000000001",
			ToolID:        "tool_00000000000000000000000000000001",
			FactKnowledge: policydomain.FactKnown,
			Effects:       []string{"filesystem.write"},
			Resources: []ResourceFact{
				{Kind: "path", Value: "/workspace/release/v1"},
			},
		},
	)
	if err != nil {
		t.Fatal(err)
	}
	if decision.Kind() != policydomain.DecisionDeny ||
		decision.ReasonCode() != "policy.rule.00000000000000000000000000000001" {
		t.Fatalf("decision = %#v", decision)
	}
}

func TestEvaluateFailsClosedOnInvalidExecutionMetadata(t *testing.T) {
	t.Parallel()

	repository := newMemoryRepository(t)
	_, err := NewEvaluate(repository).Execute(
		context.Background(),
		EvaluateRequest{
			Origin:        policydomain.Origin("unknown"),
			ScopeKind:     policydomain.ScopeCatalog,
			ProviderID:    "prov_00000000000000000000000000000001",
			ToolID:        "tool_00000000000000000000000000000001",
			FactKnowledge: policydomain.FactUnknown,
		},
	)
	if err == nil || !strings.Contains(err.Error(), "evaluation.origin") {
		t.Fatalf("Evaluate.Execute() error = %v", err)
	}
}

func TestEvaluateRejectsValuesAttachedToUnknownFacts(t *testing.T) {
	t.Parallel()

	repository := newMemoryRepository(t)
	_, err := NewEvaluate(repository).Execute(
		context.Background(),
		EvaluateRequest{
			Origin:        policydomain.OriginCLI,
			ScopeKind:     policydomain.ScopeCatalog,
			ProviderID:    "prov_00000000000000000000000000000001",
			ToolID:        "tool_00000000000000000000000000000001",
			FactKnowledge: policydomain.FactUnknown,
			Effects:       []string{"filesystem.write"},
		},
	)
	if err == nil || !strings.Contains(err.Error(), "unknown facts") {
		t.Fatalf("Evaluate.Execute() error = %v", err)
	}
}

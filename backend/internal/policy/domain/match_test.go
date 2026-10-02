package domain

import (
	governancedomain "github.com/orz-i/Mender/backend/internal/governance/domain"
	"testing"
)

func TestMatchSemantics(t *testing.T) {
	t.Parallel()

	exact, err := NewResourceMatcher("path", ResourceExact, "/workspace/release")
	if err != nil {
		t.Fatal(err)
	}
	prefix, err := NewResourceMatcher("host", ResourcePrefix, "api.example.com/")
	if err != nil {
		t.Fatal(err)
	}
	match, err := NewMatch(MatchSpec{
		Actions:       []governancedomain.Action{governancedomain.ActionToolInvoke},
		Origins:       []governancedomain.Origin{governancedomain.OriginMCP},
		ScopeKinds:    []governancedomain.ScopeKind{governancedomain.ScopeProfile},
		ProfileRefs:   []string{"prof_00000000000000000000000000000001"},
		ProviderIDs:   []string{"prov_00000000000000000000000000000001"},
		ToolIDs:       []string{"tool_00000000000000000000000000000001"},
		FactKnowledge: FactKnown,
		EffectsAny:    []string{"filesystem.write"},
		ResourcesAny:  []ResourceMatcher{exact, prefix},
	})
	if err != nil {
		t.Fatal(err)
	}

	input := EvaluationInput{
		Action:             governancedomain.ActionToolInvoke,
		PrincipalKind:      governancedomain.PrincipalLocalUser,
		PrincipalReference: "test:user",
		Origin:             governancedomain.OriginMCP,
		ScopeKind:          governancedomain.ScopeProfile,
		ScopeReference:     "prof_00000000000000000000000000000001",
		ProviderID:         "prov_00000000000000000000000000000001",
		ToolID:             "tool_00000000000000000000000000000001",
		FactKnowledge:      FactKnown,
		Effects:            []string{"filesystem.write"},
		Resources: []ResourceFact{
			{Kind: "path", Value: "/workspace/release"},
		},
	}
	if !match.Matches(input) {
		t.Fatal("expected complete match")
	}

	input.FactKnowledge = FactUnknown
	input.Effects = nil
	input.Resources = nil
	if match.Matches(input) {
		t.Fatal("effect/resource match unexpectedly accepted unknown facts")
	}
}

func TestUnknownFactRuleAndCrossFieldValidation(t *testing.T) {
	t.Parallel()

	unknown, err := NewMatch(MatchSpec{
		Actions: []governancedomain.Action{governancedomain.ActionToolInvoke}, FactKnowledge: FactUnknown})
	if err != nil {
		t.Fatal(err)
	}
	if !unknown.Matches(EvaluationInput{
		Action:             governancedomain.ActionToolInvoke,
		PrincipalKind:      governancedomain.PrincipalLocalUser,
		PrincipalReference: "test:user", FactKnowledge: FactUnknown}) {
		t.Fatal("explicit unknown-fact match did not match unknown facts")
	}
	if unknown.Matches(EvaluationInput{
		Action:             governancedomain.ActionToolInvoke,
		PrincipalKind:      governancedomain.PrincipalLocalUser,
		PrincipalReference: "test:user", FactKnowledge: FactKnown}) {
		t.Fatal("unknown-fact match accepted known facts")
	}

	if _, err := NewMatch(MatchSpec{
		Actions:       []governancedomain.Action{governancedomain.ActionToolInvoke},
		FactKnowledge: FactUnknown,
		EffectsAny:    []string{"network.write"},
	}); err == nil {
		t.Fatal("unknown FactKnowledge accepted effect predicate")
	}

	if _, err := NewMatch(MatchSpec{
		Actions:     []governancedomain.Action{governancedomain.ActionToolInvoke},
		ScopeKinds:  []governancedomain.ScopeKind{governancedomain.ScopeCatalog},
		ProfileRefs: []string{"prof_00000000000000000000000000000001"},
	}); err == nil {
		t.Fatal("catalog-only match accepted ProfileRefs")
	}
}

func TestMatchNormalizesDuplicatesAndWildcard(t *testing.T) {
	t.Parallel()

	match, err := NewMatch(MatchSpec{
		Actions:       []governancedomain.Action{governancedomain.ActionToolInvoke},
		Origins:       []governancedomain.Origin{governancedomain.OriginMCP, governancedomain.OriginMCP},
		FactKnowledge: "",
		EffectsAny:    []string{"network.read", "network.read"},
	})
	if err != nil {
		t.Fatal(err)
	}
	if got := len(match.Origins()); got != 1 {
		t.Fatalf("Origins() len = %d, want 1", got)
	}
	if got := len(match.EffectsAny()); got != 1 {
		t.Fatalf("EffectsAny() len = %d, want 1", got)
	}
	if match.FactKnowledge() != FactAny {
		t.Fatalf("FactKnowledge() = %q", match.FactKnowledge())
	}

	wildcard, err := NewMatch(MatchSpec{
		Actions: []governancedomain.Action{governancedomain.ActionToolInvoke}})
	if err != nil {
		t.Fatal(err)
	}
	if !wildcard.IsWildcard() {
		t.Fatal("empty MatchSpec did not produce wildcard")
	}
}

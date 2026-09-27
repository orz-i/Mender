package domain

import (
	"testing"
	"time"
)

func TestPolicySetUsesDefaultOnlyWhenNoRuleMatches(t *testing.T) {
	t.Parallel()

	denyDefault, err := NewPolicySet(DecisionDeny, nil)
	if err != nil {
		t.Fatal(err)
	}
	decision, err := denyDefault.Evaluate(validEvaluationInput(OriginCLI, FactUnknown))
	if err != nil {
		t.Fatal(err)
	}
	if decision.Kind() != DecisionDeny || decision.ReasonCode() != "policy.default.deny" {
		t.Fatalf("default decision = %#v", decision)
	}

	allowRule := mustPolicyRule(
		t,
		"rule_00000000000000000000000000000002",
		"Allow MCP",
		DecisionAllow,
		MatchSpec{Origins: []Origin{OriginMCP}},
	)
	allowList, err := NewPolicySet(DecisionDeny, []Rule{allowRule})
	if err != nil {
		t.Fatal(err)
	}
	decision, err = allowList.Evaluate(validEvaluationInput(OriginMCP, FactUnknown))
	if err != nil {
		t.Fatal(err)
	}
	if decision.Kind() != DecisionAllow {
		t.Fatalf("matching allow over default deny = %q", decision.Kind())
	}
}

func TestPolicySetMatchingRulesUseMostRestrictiveDecisionWithoutOrder(t *testing.T) {
	t.Parallel()

	allow := mustPolicyRule(
		t,
		"rule_00000000000000000000000000000003",
		"Allow",
		DecisionAllow,
		MatchSpec{Origins: []Origin{OriginMCP}},
	)
	approval := mustPolicyRule(
		t,
		"rule_00000000000000000000000000000002",
		"Approval",
		DecisionRequireApproval,
		MatchSpec{Origins: []Origin{OriginMCP}},
	)
	deny := mustPolicyRule(
		t,
		"rule_00000000000000000000000000000001",
		"Deny",
		DecisionDeny,
		MatchSpec{Origins: []Origin{OriginMCP}},
	)

	for _, rules := range [][]Rule{
		{allow, approval, deny},
		{deny, allow, approval},
		{approval, deny, allow},
	} {
		policy, err := NewPolicySet(DecisionAllow, rules)
		if err != nil {
			t.Fatal(err)
		}
		decision, err := policy.Evaluate(validEvaluationInput(OriginMCP, FactUnknown))
		if err != nil {
			t.Fatal(err)
		}
		if decision.Kind() != DecisionDeny {
			t.Fatalf("decision kind = %q, want deny", decision.Kind())
		}
		if decision.ReasonCode() != "policy.rule.00000000000000000000000000000001" {
			t.Fatalf("reason = %q", decision.ReasonCode())
		}
	}
}

func TestPolicySetSameDecisionTieBreaksByRuleID(t *testing.T) {
	t.Parallel()

	first := mustPolicyRule(
		t,
		"rule_00000000000000000000000000000001",
		"First",
		DecisionDeny,
		MatchSpec{},
	)
	second := mustPolicyRule(
		t,
		"rule_00000000000000000000000000000002",
		"Second",
		DecisionDeny,
		MatchSpec{},
	)
	policy, err := NewPolicySet(DecisionAllow, []Rule{second, first})
	if err != nil {
		t.Fatal(err)
	}
	decision, err := policy.Evaluate(validEvaluationInput(OriginCLI, FactUnknown))
	if err != nil {
		t.Fatal(err)
	}
	if decision.ReasonCode() != "policy.rule.00000000000000000000000000000001" {
		t.Fatalf("reason = %q", decision.ReasonCode())
	}
}

func TestDisabledRuleDoesNotMatch(t *testing.T) {
	t.Parallel()

	rule := mustPolicyRule(
		t,
		"rule_00000000000000000000000000000001",
		"Disabled deny",
		DecisionDeny,
		MatchSpec{},
	)
	rule, err := rule.SetEnabled(false, time.Date(2026, 9, 27, 1, 0, 0, 0, time.UTC))
	if err != nil {
		t.Fatal(err)
	}
	policy, err := NewPolicySet(DecisionAllow, []Rule{rule})
	if err != nil {
		t.Fatal(err)
	}
	decision, err := policy.Evaluate(validEvaluationInput(OriginCLI, FactUnknown))
	if err != nil {
		t.Fatal(err)
	}
	if decision.Kind() != DecisionAllow || decision.ReasonCode() != "policy.default.allow" {
		t.Fatalf("decision = %#v", decision)
	}
}

func mustPolicyRule(
	t *testing.T,
	rawID string,
	name string,
	decision DecisionKind,
	matchSpec MatchSpec,
) Rule {
	t.Helper()
	id, err := ParseRuleID(rawID)
	if err != nil {
		t.Fatal(err)
	}
	match, err := NewMatch(matchSpec)
	if err != nil {
		t.Fatal(err)
	}
	rule, err := NewRule(
		id,
		name,
		true,
		decision,
		match,
		time.Date(2026, 9, 27, 0, 0, 0, 0, time.UTC),
	)
	if err != nil {
		t.Fatal(err)
	}
	return rule
}

func validEvaluationInput(origin Origin, knowledge FactKnowledge) EvaluationInput {
	return EvaluationInput{
		Origin:        origin,
		ScopeKind:     ScopeCatalog,
		ProviderID:    "prov_00000000000000000000000000000001",
		ToolID:        "tool_00000000000000000000000000000001",
		FactKnowledge: knowledge,
	}
}

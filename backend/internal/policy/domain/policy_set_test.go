package domain

import (
	"testing"
	"time"

	governancedomain "github.com/orz-i/Mender/backend/internal/governance/domain"
)

func TestPolicySetUsesDefaultOnlyWhenNoRuleMatches(t *testing.T) {
	t.Parallel()

	denyDefault, err := NewPolicySet(DecisionDeny, nil)
	if err != nil {
		t.Fatal(err)
	}
	decision, err := denyDefault.Evaluate(validEvaluationInput(governancedomain.OriginCLI, FactUnknown))
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
		MatchSpec{
			Actions: []governancedomain.Action{governancedomain.ActionToolInvoke}, Origins: []governancedomain.Origin{governancedomain.OriginMCP}},
	)
	allowList, err := NewPolicySet(DecisionDeny, []Rule{allowRule})
	if err != nil {
		t.Fatal(err)
	}
	decision, err = allowList.Evaluate(validEvaluationInput(governancedomain.OriginMCP, FactUnknown))
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
		MatchSpec{
			Actions: []governancedomain.Action{governancedomain.ActionToolInvoke}, Origins: []governancedomain.Origin{governancedomain.OriginMCP}},
	)
	approval := mustPolicyRule(
		t,
		"rule_00000000000000000000000000000002",
		"Approval",
		DecisionRequireApproval,
		MatchSpec{
			Actions: []governancedomain.Action{governancedomain.ActionToolInvoke}, Origins: []governancedomain.Origin{governancedomain.OriginMCP}},
	)
	deny := mustPolicyRule(
		t,
		"rule_00000000000000000000000000000001",
		"Deny",
		DecisionDeny,
		MatchSpec{
			Actions: []governancedomain.Action{governancedomain.ActionToolInvoke}, Origins: []governancedomain.Origin{governancedomain.OriginMCP}},
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
		decision, err := policy.Evaluate(validEvaluationInput(governancedomain.OriginMCP, FactUnknown))
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
		MatchSpec{
			Actions: []governancedomain.Action{governancedomain.ActionToolInvoke}},
	)
	second := mustPolicyRule(
		t,
		"rule_00000000000000000000000000000002",
		"Second",
		DecisionDeny,
		MatchSpec{
			Actions: []governancedomain.Action{governancedomain.ActionToolInvoke}},
	)
	policy, err := NewPolicySet(DecisionAllow, []Rule{second, first})
	if err != nil {
		t.Fatal(err)
	}
	decision, err := policy.Evaluate(validEvaluationInput(governancedomain.OriginCLI, FactUnknown))
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
		MatchSpec{
			Actions: []governancedomain.Action{governancedomain.ActionToolInvoke}},
	)
	rule, err := rule.SetEnabled(false, time.Date(2026, 9, 27, 1, 0, 0, 0, time.UTC))
	if err != nil {
		t.Fatal(err)
	}
	policy, err := NewPolicySet(DecisionAllow, []Rule{rule})
	if err != nil {
		t.Fatal(err)
	}
	decision, err := policy.Evaluate(validEvaluationInput(governancedomain.OriginCLI, FactUnknown))
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

func validEvaluationInput(origin governancedomain.Origin, knowledge FactKnowledge) EvaluationInput {
	return EvaluationInput{
		Action:             governancedomain.ActionToolInvoke,
		PrincipalKind:      governancedomain.PrincipalLocalUser,
		PrincipalReference: "test:user",
		Origin:             origin,
		ScopeKind:          governancedomain.ScopeCatalog,
		ProviderID:         "prov_00000000000000000000000000000001",
		ToolID:             "tool_00000000000000000000000000000001",
		FactKnowledge:      knowledge,
	}
}

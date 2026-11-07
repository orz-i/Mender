package domain

import (
	"strings"
	"testing"
	"time"

	gov "github.com/orz-i/Mender/backend/internal/governance/domain"
)

func sourceEvaluation(source, tool string) EvaluationInput {
	return EvaluationInput{
		Action: gov.ActionToolInvoke, Origin: gov.OriginMCP,
		PrincipalKind: gov.PrincipalService, PrincipalReference: "profile:prof_aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa",
		ScopeKind: gov.ScopeProfile, ScopeReference: "prof_aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa",
		SourceRef: source, ToolID: tool, FactKnowledge: FactKnown,
		Effects:   []string{"filesystem.read"},
		Resources: []ResourceFact{{Kind: "workspace_path", Value: "README.md"}},
	}
}

func TestToolPolicyUsesExactTypedSourceAndRetainsProviderSelectorBoundary(t *testing.T) {
	providerID := "prov_11111111111111111111111111111111"
	toolID := "tool_22222222222222222222222222222222"
	otherID := "tool_33333333333333333333333333333333"
	for _, source := range []string{
		"provider:" + providerID,
		"agent:agent_aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa",
		"native:workspace",
		"native:jobs",
	} {
		if err := sourceEvaluation(source, toolID).Validate(); err != nil {
			t.Fatalf("valid source %q rejected: %v", source, err)
		}
	}
	for _, source := range []string{
		"", "provider", "provider:agent_aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa",
		"agent:" + providerID, "native:browser", "native:workspace:extra",
		"provider:prov_AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA", "native:workspace ",
	} {
		if err := sourceEvaluation(source, toolID).Validate(); err == nil {
			t.Fatalf("unverified Tool SourceRef accepted: %q", source)
		}
	}
	providerRule, err := NewMatch(MatchSpec{
		Actions:     []gov.Action{gov.ActionToolInvoke},
		ProviderIDs: []string{providerID},
	})
	if err != nil {
		t.Fatal(err)
	}
	if !providerRule.Matches(sourceEvaluation("provider:"+providerID, toolID)) {
		t.Fatal("real Provider Tool lost its scoped Policy")
	}
	for _, source := range []string{
		"agent:agent_aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa", "native:workspace", "native:jobs",
	} {
		if providerRule.Matches(sourceEvaluation(source, toolID)) {
			t.Fatalf("non-Provider source %q inherited Provider-only selector", source)
		}
	}
	agentOnlyRule, err := NewMatch(MatchSpec{
		Actions: []gov.Action{gov.ActionToolInvoke}, ToolIDs: []string{toolID},
	})
	if err != nil {
		t.Fatal(err)
	}
	if !agentOnlyRule.Matches(sourceEvaluation("agent:agent_aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa", toolID)) ||
		agentOnlyRule.Matches(sourceEvaluation("agent:agent_aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa", otherID)) {
		t.Fatal("Agent-owned ToolID selector was not exact")
	}
}

func TestToolPolicySourceCannotForgeProviderOrResourceAction(t *testing.T) {
	input := sourceEvaluation("native:workspace", "tool_22222222222222222222222222222222")
	input.ProviderID = "prov_11111111111111111111111111111111"
	if err := input.Validate(); err == nil {
		t.Fatal("Native Tool invoked with a forged ProviderID")
	}
	input.ProviderID = ""
	input.SourceRef = "native:workspace"
	input.Action = gov.ActionProviderActivate
	if err := input.Validate(); err == nil {
		t.Fatal("Native SourceRef impersonated provider.activate")
	}
	input.SourceRef = ""
	input.ProviderID = "prov_11111111111111111111111111111111"
	input.ToolID = ""
	input.FactKnowledge = ""
	input.Effects = nil
	input.Resources = nil
	if err := input.Validate(); err != nil {
		t.Fatalf("real Provider activation rejected: %v", err)
	}
	input.ProviderID = "prov_invalid"
	if err := input.Validate(); err == nil {
		t.Fatal("malformed Provider activation accepted")
	}
}

func TestPolicySetEvaluatesNativeToolByExactToolID(t *testing.T) {
	nativeToolID := "tool_22222222222222222222222222222222"
	match, err := NewMatch(MatchSpec{
		Actions:       []gov.Action{gov.ActionToolInvoke},
		ToolIDs:       []string{nativeToolID},
		FactKnowledge: FactKnown,
	})
	if err != nil {
		t.Fatal(err)
	}
	ruleID, err := ParseRuleID("rule_99999999999999999999999999999999")
	if err != nil {
		t.Fatal(err)
	}
	rule, err := NewRule(ruleID, "Native denial", true, DecisionDeny, match, time.Now().UTC())
	if err != nil {
		t.Fatal(err)
	}
	policy, err := NewPolicySet(DecisionAllow, []Rule{rule})
	if err != nil {
		t.Fatal(err)
	}
	decision, err := policy.Evaluate(sourceEvaluation("native:workspace", nativeToolID))
	if err != nil || decision.Kind() != DecisionDeny {
		t.Fatalf("native ToolID-specific deny was lost: %v %v", decision, err)
	}
	other, err := policy.Evaluate(sourceEvaluation("native:jobs", "tool_33333333333333333333333333333333"))
	if err != nil || other.Kind() != DecisionAllow {
		t.Fatalf("different Native operation reused another deny: %v %v", other, err)
	}
	if !strings.HasPrefix(decision.ReasonCode(), "policy.rule.") {
		t.Fatal("policy rule explanation lost")
	}
}

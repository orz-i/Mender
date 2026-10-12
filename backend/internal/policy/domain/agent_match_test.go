package domain

import (
	"testing"

	gov "github.com/orz-i/Mender/backend/internal/governance/domain"
)

func TestAgentActionsRequireTypedResourceFacts(t *testing.T) {
	match, err := NewMatch(MatchSpec{Actions: []gov.Action{gov.ActionAgentTaskSubmit}})
	if err != nil {
		t.Fatal(err)
	}
	valid := EvaluationInput{
		Action: gov.ActionAgentTaskSubmit, Origin: gov.OriginMCP, PrincipalKind: gov.PrincipalService,
		PrincipalReference: "profile:p1", ScopeKind: gov.ScopeProfile, ScopeReference: "p1",
		FactKnowledge: FactKnown,
		Resources:     []ResourceFact{{Kind: "agent_id", Value: "agent_0123456789abcdef0123456789abcdef"}},
	}
	if err := valid.Validate(); err != nil {
		t.Fatal(err)
	}
	if !match.Matches(valid) {
		t.Fatal("Agent rule failed to match")
	}
	valid.Resources = nil
	if err := valid.Validate(); err == nil {
		t.Fatal("unscoped Agent action accepted")
	}
	if _, err := NewMatch(MatchSpec{Actions: []gov.Action{gov.ActionAgentTaskSubmit},
		ProviderIDs: []string{"provider-a"}}); err == nil {
		t.Fatal("Provider selectors accepted for Agent action")
	}
	if _, err := NewMatch(MatchSpec{Actions: []gov.Action{gov.ActionAgentTaskSubmit, gov.ActionToolInvoke}}); err == nil {
		t.Fatal("Tool invocation joined Agent admission rule")
	}
}

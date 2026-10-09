package domain

import (
	"testing"

	gov "github.com/orz-i/Mender/backend/internal/governance/domain"
)

func TestSandboxActionsCannotInheritToolOrProviderRules(t *testing.T) {
	toolRule, err := NewMatch(MatchSpec{Actions: []gov.Action{gov.ActionToolInvoke}})
	if err != nil {
		t.Fatal(err)
	}
	sandboxRule, err := NewMatch(MatchSpec{Actions: []gov.Action{gov.ActionSandboxAcquire}, FactKnowledge: FactKnown, ResourcesAny: []ResourceMatcher{mustSandboxResource(t)}})
	if err != nil {
		t.Fatal(err)
	}
	sandbox := sandboxInput(gov.ActionSandboxAcquire)
	if err := sandbox.Validate(); err != nil {
		t.Fatalf("sandbox input: %v", err)
	}
	if toolRule.Matches(sandbox) {
		t.Fatal("tool rule inherited sandbox action")
	}
	if !sandboxRule.Matches(sandbox) {
		t.Fatal("sandbox selector did not match")
	}
	input := EvaluationInput{Action: gov.ActionToolInvoke, Origin: gov.OriginCLI, PrincipalKind: gov.PrincipalLocalUser, PrincipalReference: "owner", ScopeKind: gov.ScopeCatalog, ProviderID: "provider-test", ToolID: "tool-test", FactKnowledge: FactKnown, Resources: []ResourceFact{{Kind: "sandbox_id", Value: "sbx_0123456789abcdef0123456789abcdef"}}}
	if sandboxRule.Matches(input) {
		t.Fatal("sandbox rule leaked into tool invocation")
	}
}
func TestSandboxRuleCannotSelectProviderOrMixActions(t *testing.T) {
	cases := []MatchSpec{
		{Actions: []gov.Action{gov.ActionSandboxAcquire, gov.ActionToolInvoke}},
		{Actions: []gov.Action{gov.ActionSandboxAcquire}, ProviderIDs: []string{"provider-id"}},
		{Actions: []gov.Action{gov.ActionSandboxAcquire}, ToolIDs: []string{"tool-id"}},
	}
	for _, spec := range cases {
		if _, err := NewMatch(spec); err == nil {
			t.Fatalf("accepted unsafe rule %+v", spec)
		}
	}
}
func TestSandboxActionRequiresReviewedIdentity(t *testing.T) {
	input := sandboxInput(gov.ActionSandboxAcquire)
	for _, test := range []struct {
		name   string
		mutate func(*EvaluationInput)
	}{
		{"unknown fact", func(v *EvaluationInput) { v.FactKnowledge = FactUnknown }},
		{"no id", func(v *EvaluationInput) { v.Resources = nil }},
		{"invalid id", func(v *EvaluationInput) { v.Resources[0].Value = "sbx_0123456789abcdef0123456789abcdeg" }},
		{"provider spoof", func(v *EvaluationInput) { v.ProviderID = "provider" }},
		{"tool spoof", func(v *EvaluationInput) { v.ToolID = "tool" }},
	} {
		t.Run(test.name, func(t *testing.T) {
			v := input
			v.Resources = append([]ResourceFact(nil), input.Resources...)
			test.mutate(&v)
			if err := v.Validate(); err == nil {
				t.Fatal("invalid sandbox action was admitted")
			}
		})
	}
}
func sandboxInput(action gov.Action) EvaluationInput {
	return EvaluationInput{Action: action, Origin: gov.OriginCLI, PrincipalKind: gov.PrincipalLocalUser, PrincipalReference: "owner", ScopeKind: gov.ScopeCatalog, FactKnowledge: FactKnown, Resources: []ResourceFact{{Kind: "sandbox_id", Value: "sbx_0123456789abcdef0123456789abcdef"}, {Kind: "sandbox_backend", Value: "local_workspace"}, {Kind: "sandbox_isolation", Value: "host"}}}
}
func mustSandboxResource(t *testing.T) ResourceMatcher {
	t.Helper()
	r, err := NewResourceMatcher("sandbox_id", ResourceExact, "sbx_0123456789abcdef0123456789abcdef")
	if err != nil {
		t.Fatal(err)
	}
	return r
}

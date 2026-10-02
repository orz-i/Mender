package application

import (
	"context"
	"errors"
	"testing"
	"time"

	governancedomain "github.com/orz-i/Mender/backend/internal/governance/domain"
	policydomain "github.com/orz-i/Mender/backend/internal/policy/domain"
)

func TestPolicyRuleLifecycleAndReferenceValidation(t *testing.T) {
	t.Parallel()

	repository := newMemoryRepository(t)
	ids := &fixedRuleIDs{ids: []policydomain.RuleID{
		mustRuleID(t, "rule_00000000000000000000000000000001"),
	}}
	clock := fixedClock{now: time.Date(2026, 9, 27, 1, 0, 0, 0, time.UTC)}
	references := fakeReferences{
		profiles:  map[string]bool{"prof_00000000000000000000000000000001": true},
		providers: map[string]bool{"prov_00000000000000000000000000000001": true},
		tools:     map[string]bool{"tool_00000000000000000000000000000001": true},
	}

	create := NewCreateRule(repository, ids, clock, references)
	rule, err := create.Execute(context.Background(), RuleInput{
		DisplayName: "Guard MCP",
		Enabled:     true,
		Decision:    policydomain.DecisionRequireApproval,
		Match: policydomain.MatchSpec{
			Actions:     []governancedomain.Action{governancedomain.ActionToolInvoke},
			Origins:     []governancedomain.Origin{governancedomain.OriginMCP},
			ProfileRefs: []string{"prof_00000000000000000000000000000001"},
			ProviderIDs: []string{"prov_00000000000000000000000000000001"},
			ToolIDs:     []string{"tool_00000000000000000000000000000001"},
		},
	})
	if err != nil {
		t.Fatalf("CreateRule.Execute() error = %v", err)
	}
	if rule.ID() != mustRuleID(t, "rule_00000000000000000000000000000001") {
		t.Fatalf("RuleID = %q", rule.ID())
	}

	update := NewUpdateRule(repository, clock, references)
	updated, err := update.Execute(context.Background(), rule.ID(), RuleInput{
		DisplayName: "Deny MCP",
		Enabled:     false,
		Decision:    policydomain.DecisionDeny,
		Match: policydomain.MatchSpec{
			Actions: []governancedomain.Action{governancedomain.ActionToolInvoke},
			Origins: []governancedomain.Origin{governancedomain.OriginMCP},
		},
	})
	if err != nil {
		t.Fatalf("UpdateRule.Execute() error = %v", err)
	}
	if updated.ID() != rule.ID() || updated.DisplayName() != "Deny MCP" ||
		updated.Decision() != policydomain.DecisionDeny || updated.Enabled() {
		t.Fatalf("updated Rule = %#v", updated)
	}

	enable := NewSetRuleEnabled(repository, clock)
	enabled, err := enable.Execute(context.Background(), rule.ID(), true)
	if err != nil {
		t.Fatalf("SetRuleEnabled.Execute() error = %v", err)
	}
	if !enabled.Enabled() {
		t.Fatal("Rule remained disabled")
	}

	if err := NewDeleteRule(repository).Execute(context.Background(), rule.ID()); err != nil {
		t.Fatalf("DeleteRule.Execute() error = %v", err)
	}
	if _, err := repository.GetRule(context.Background(), rule.ID()); !errors.Is(err, ErrRuleNotFound) {
		t.Fatalf("GetRule(deleted) error = %v", err)
	}
}

func TestCreateRuleRejectsMissingReferenceBeforePersistence(t *testing.T) {
	t.Parallel()

	repository := newMemoryRepository(t)
	create := NewCreateRule(
		repository,
		&fixedRuleIDs{ids: []policydomain.RuleID{
			mustRuleID(t, "rule_00000000000000000000000000000001"),
		}},
		fixedClock{now: time.Date(2026, 9, 27, 1, 0, 0, 0, time.UTC)},
		fakeReferences{},
	)

	_, err := create.Execute(context.Background(), RuleInput{
		DisplayName: "Missing Tool",
		Enabled:     true,
		Decision:    policydomain.DecisionDeny,
		Match: policydomain.MatchSpec{
			Actions: []governancedomain.Action{governancedomain.ActionToolInvoke},
			ToolIDs: []string{"tool_00000000000000000000000000000001"},
		},
	})
	if !errors.Is(err, ErrToolReferenceNotFound) {
		t.Fatalf("CreateRule.Execute() error = %v", err)
	}
	policy, err := repository.GetPolicySet(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(policy.Rules()) != 0 {
		t.Fatalf("failed create persisted Rules = %#v", policy.Rules())
	}
}

func TestSetDefaultAndEvaluateUseCurrentPolicySet(t *testing.T) {
	t.Parallel()

	repository := newMemoryRepository(t)
	clock := fixedClock{now: time.Date(2026, 9, 27, 1, 0, 0, 0, time.UTC)}

	policy, err := NewSetDefaultDecision(repository, clock).Execute(
		context.Background(),
		policydomain.DecisionDeny,
	)
	if err != nil {
		t.Fatal(err)
	}
	if policy.DefaultDecision() != policydomain.DecisionDeny {
		t.Fatalf("DefaultDecision() = %q", policy.DefaultDecision())
	}

	decision, err := NewEvaluate(repository).Execute(
		context.Background(),
		EvaluateRequest{
			Action:             governancedomain.ActionToolInvoke,
			PrincipalKind:      governancedomain.PrincipalLocalUser,
			PrincipalReference: "test:user",
			Origin:             governancedomain.OriginCLI,
			ScopeKind:          governancedomain.ScopeCatalog,
			ProviderID:         "prov_00000000000000000000000000000001",
			ToolID:             "tool_00000000000000000000000000000001",
			FactKnowledge:      policydomain.FactUnknown,
		},
	)
	if err != nil {
		t.Fatal(err)
	}
	if decision.Kind() != policydomain.DecisionDeny ||
		decision.ReasonCode() != "policy.default.deny" {
		t.Fatalf("decision = %#v", decision)
	}
}

type memoryRepository struct {
	defaultDecision policydomain.DecisionKind
	rules           map[policydomain.RuleID]policydomain.Rule
}

func newMemoryRepository(t *testing.T) *memoryRepository {
	t.Helper()
	return &memoryRepository{
		defaultDecision: policydomain.DecisionAllow,
		rules:           make(map[policydomain.RuleID]policydomain.Rule),
	}
}

func (m *memoryRepository) GetPolicySet(context.Context) (policydomain.PolicySet, error) {
	rules := make([]policydomain.Rule, 0, len(m.rules))
	for _, rule := range m.rules {
		rules = append(rules, rule)
	}
	return policydomain.NewPolicySet(m.defaultDecision, rules)
}

func (m *memoryRepository) SetDefaultDecision(
	_ context.Context,
	decision policydomain.DecisionKind,
	_ time.Time,
) error {
	m.defaultDecision = decision
	return nil
}

func (m *memoryRepository) CreateRule(_ context.Context, rule policydomain.Rule) error {
	if _, exists := m.rules[rule.ID()]; exists {
		return ErrRuleConflict
	}
	m.rules[rule.ID()] = rule
	return nil
}

func (m *memoryRepository) GetRule(
	_ context.Context,
	id policydomain.RuleID,
) (policydomain.Rule, error) {
	rule, exists := m.rules[id]
	if !exists {
		return policydomain.Rule{}, ErrRuleNotFound
	}
	return rule, nil
}

func (m *memoryRepository) UpdateRule(_ context.Context, rule policydomain.Rule) error {
	if _, exists := m.rules[rule.ID()]; !exists {
		return ErrRuleNotFound
	}
	m.rules[rule.ID()] = rule
	return nil
}

func (m *memoryRepository) DeleteRule(_ context.Context, id policydomain.RuleID) error {
	if _, exists := m.rules[id]; !exists {
		return ErrRuleNotFound
	}
	delete(m.rules, id)
	return nil
}

type fixedRuleIDs struct {
	ids []policydomain.RuleID
}

func (f *fixedRuleIDs) NewRuleID() (policydomain.RuleID, error) {
	if len(f.ids) == 0 {
		return "", errors.New("no RuleIDs remaining")
	}
	id := f.ids[0]
	f.ids = f.ids[1:]
	return id, nil
}

type fixedClock struct {
	now time.Time
}

func (f fixedClock) Now() time.Time {
	return f.now
}

type fakeReferences struct {
	profiles  map[string]bool
	providers map[string]bool
	tools     map[string]bool
}

func (f fakeReferences) ProfileExists(_ context.Context, ref string) (bool, error) {
	return f.profiles[ref], nil
}

func (f fakeReferences) ProviderExists(_ context.Context, id string) (bool, error) {
	return f.providers[id], nil
}

func (f fakeReferences) ToolExists(_ context.Context, id string) (bool, error) {
	return f.tools[id], nil
}

func mustRuleID(t *testing.T, value string) policydomain.RuleID {
	t.Helper()
	id, err := policydomain.ParseRuleID(value)
	if err != nil {
		t.Fatal(err)
	}
	return id
}

package domain

import (
	"fmt"
	"strings"
	"time"
)

const maxRuleDisplayNameLength = 160

type Rule struct {
	id          RuleID
	displayName string
	enabled     bool
	decision    DecisionKind
	match       Match
	createdAt   time.Time
	updatedAt   time.Time
}

func NewRule(
	id RuleID,
	displayName string,
	enabled bool,
	decision DecisionKind,
	match Match,
	now time.Time,
) (Rule, error) {
	if _, err := ParseRuleID(id.String()); err != nil {
		return Rule{}, err
	}
	if err := ValidateDecisionKind(decision); err != nil {
		return Rule{}, err
	}
	displayName = strings.TrimSpace(displayName)
	if displayName == "" {
		return Rule{}, fmt.Errorf("display_name: must not be empty")
	}
	if len(displayName) > maxRuleDisplayNameLength {
		return Rule{}, fmt.Errorf("display_name: is too long")
	}
	if now.IsZero() {
		return Rule{}, fmt.Errorf("now: must not be zero")
	}
	now = now.UTC()
	return Rule{
		id:          id,
		displayName: displayName,
		enabled:     enabled,
		decision:    decision,
		match:       cloneMatch(match),
		createdAt:   now,
		updatedAt:   now,
	}, nil
}

func RestoreRule(
	id RuleID,
	displayName string,
	enabled bool,
	decision DecisionKind,
	match Match,
	createdAt time.Time,
	updatedAt time.Time,
) (Rule, error) {
	if createdAt.IsZero() || updatedAt.IsZero() {
		return Rule{}, fmt.Errorf("timestamps: must not be zero")
	}
	if updatedAt.Before(createdAt) {
		return Rule{}, fmt.Errorf("updated_at: must not be before created_at")
	}
	rule, err := NewRule(id, displayName, enabled, decision, match, createdAt)
	if err != nil {
		return Rule{}, err
	}
	rule.updatedAt = updatedAt.UTC()
	return rule, nil
}

func (r Rule) Update(
	displayName string,
	decision DecisionKind,
	match Match,
	now time.Time,
) (Rule, error) {
	if now.IsZero() {
		return Rule{}, fmt.Errorf("now: must not be zero")
	}
	if now.Before(r.createdAt) {
		return Rule{}, fmt.Errorf("now: must not be before created_at")
	}
	updated, err := NewRule(
		r.id,
		displayName,
		r.enabled,
		decision,
		match,
		r.createdAt,
	)
	if err != nil {
		return Rule{}, err
	}
	updated.updatedAt = now.UTC()
	return updated, nil
}

func (r Rule) SetEnabled(enabled bool, now time.Time) (Rule, error) {
	if now.IsZero() {
		return Rule{}, fmt.Errorf("now: must not be zero")
	}
	if now.Before(r.createdAt) {
		return Rule{}, fmt.Errorf("now: must not be before created_at")
	}
	r.enabled = enabled
	r.updatedAt = now.UTC()
	return r, nil
}

func (r Rule) ID() RuleID {
	return r.id
}

func (r Rule) DisplayName() string {
	return r.displayName
}

func (r Rule) Enabled() bool {
	return r.enabled
}

func (r Rule) Decision() DecisionKind {
	return r.decision
}

func (r Rule) Match() Match {
	return cloneMatch(r.match)
}

func (r Rule) CreatedAt() time.Time {
	return r.createdAt
}

func (r Rule) UpdatedAt() time.Time {
	return r.updatedAt
}

func cloneMatch(match Match) Match {
	cloned, err := NewMatch(MatchSpec{
		Origins:       match.Origins(),
		ScopeKinds:    match.ScopeKinds(),
		ProfileRefs:   match.ProfileRefs(),
		ProviderIDs:   match.ProviderIDs(),
		ToolIDs:       match.ToolIDs(),
		FactKnowledge: match.FactKnowledge(),
		EffectsAny:    match.EffectsAny(),
		ResourcesAny:  match.ResourcesAny(),
	})
	if err != nil {
		panic("policy domain: clone validated match: " + err.Error())
	}
	return cloned
}

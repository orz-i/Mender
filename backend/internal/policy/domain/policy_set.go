package domain

import (
	"fmt"
	"slices"
	"strings"
)

type PolicySet struct {
	defaultDecision DecisionKind
	rules           []Rule
}

func NewPolicySet(
	defaultDecision DecisionKind,
	rules []Rule,
) (PolicySet, error) {
	if err := ValidateDecisionKind(defaultDecision); err != nil {
		return PolicySet{}, err
	}
	normalized := make([]Rule, 0, len(rules))
	seen := make(map[RuleID]struct{}, len(rules))
	for _, rule := range rules {
		if _, err := ParseRuleID(rule.ID().String()); err != nil {
			return PolicySet{}, err
		}
		if _, exists := seen[rule.ID()]; exists {
			return PolicySet{}, fmt.Errorf("policy_set: duplicate RuleID %s", rule.ID())
		}
		seen[rule.ID()] = struct{}{}
		normalized = append(normalized, rule)
	}
	slices.SortFunc(normalized, func(a, b Rule) int {
		return strings.Compare(a.ID().String(), b.ID().String())
	})
	return PolicySet{
		defaultDecision: defaultDecision,
		rules:           normalized,
	}, nil
}

func (p PolicySet) DefaultDecision() DecisionKind {
	return p.defaultDecision
}

func (p PolicySet) Rules() []Rule {
	return slices.Clone(p.rules)
}

func (p PolicySet) Evaluate(input EvaluationInput) (Decision, error) {
	if err := input.Validate(); err != nil {
		return Decision{}, err
	}
	var winning *Rule
	winningSeverity := 0

	for i := range p.rules {
		rule := p.rules[i]
		if !rule.Enabled() || !rule.Match().Matches(input) {
			continue
		}
		severity := decisionSeverity(rule.Decision())
		if winning == nil ||
			severity > winningSeverity ||
			(severity == winningSeverity &&
				rule.ID().String() < winning.ID().String()) {
			ruleCopy := rule
			winning = &ruleCopy
			winningSeverity = severity
		}
	}

	if winning == nil {
		return decisionForKind(
			p.defaultDecision,
			"policy.default."+string(p.defaultDecision),
			"default Policy decision",
		)
	}

	suffix := strings.TrimPrefix(winning.ID().String(), ruleIDPrefix)
	return decisionForKind(
		winning.Decision(),
		"policy.rule."+suffix,
		"Policy rule "+winning.DisplayName(),
	)
}

func decisionSeverity(kind DecisionKind) int {
	switch kind {
	case DecisionAllow:
		return 1
	case DecisionRequireApproval:
		return 2
	case DecisionDeny:
		return 3
	default:
		return 0
	}
}

func decisionForKind(
	kind DecisionKind,
	reasonCode string,
	explanation string,
) (Decision, error) {
	switch kind {
	case DecisionAllow:
		return Allow(reasonCode, explanation)
	case DecisionRequireApproval:
		return RequireApproval(reasonCode, explanation)
	case DecisionDeny:
		return Deny(reasonCode, explanation)
	default:
		return Decision{}, fmt.Errorf("decision: unknown value %q", kind)
	}
}

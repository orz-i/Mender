package application

import (
	"context"
	"fmt"

	policydomain "github.com/orz-i/Mender/backend/internal/policy/domain"
)

type GetPolicy struct {
	repository Repository
}

func NewGetPolicy(repository Repository) GetPolicy {
	return GetPolicy{repository: repository}
}

func (uc GetPolicy) Execute(ctx context.Context) (policydomain.PolicySet, error) {
	policy, err := uc.repository.GetPolicySet(ctx)
	if err != nil {
		return policydomain.PolicySet{}, fmt.Errorf("get policy: %w", err)
	}
	return policy, nil
}

type SetDefaultDecision struct {
	repository Repository
	clock      Clock
}

func NewSetDefaultDecision(repository Repository, clock Clock) SetDefaultDecision {
	return SetDefaultDecision{repository: repository, clock: clock}
}

func (uc SetDefaultDecision) Execute(
	ctx context.Context,
	decision policydomain.DecisionKind,
) (policydomain.PolicySet, error) {
	if err := policydomain.ValidateDecisionKind(decision); err != nil {
		return policydomain.PolicySet{}, fmt.Errorf("set policy default: %w", err)
	}
	if err := uc.repository.SetDefaultDecision(ctx, decision, uc.clock.Now()); err != nil {
		return policydomain.PolicySet{}, fmt.Errorf("set policy default: %w", err)
	}
	policy, err := uc.repository.GetPolicySet(ctx)
	if err != nil {
		return policydomain.PolicySet{}, fmt.Errorf("set policy default: reload: %w", err)
	}
	return policy, nil
}

type RuleInput struct {
	DisplayName string
	Enabled     bool
	Decision    policydomain.DecisionKind
	Match       policydomain.MatchSpec
}

type CreateRule struct {
	repository Repository
	ids        RuleIDGenerator
	clock      Clock
	references ReferenceLookup
}

func NewCreateRule(
	repository Repository,
	ids RuleIDGenerator,
	clock Clock,
	references ReferenceLookup,
) CreateRule {
	return CreateRule{
		repository: repository,
		ids:        ids,
		clock:      clock,
		references: references,
	}
}

func (uc CreateRule) Execute(
	ctx context.Context,
	input RuleInput,
) (policydomain.Rule, error) {
	match, err := policydomain.NewMatch(input.Match)
	if err != nil {
		return policydomain.Rule{}, fmt.Errorf("create policy rule: %w", err)
	}
	if err := validateReferences(ctx, uc.references, match); err != nil {
		return policydomain.Rule{}, fmt.Errorf("create policy rule: %w", err)
	}
	id, err := uc.ids.NewRuleID()
	if err != nil {
		return policydomain.Rule{}, fmt.Errorf("create policy rule: allocate id: %w", err)
	}
	rule, err := policydomain.NewRule(
		id,
		input.DisplayName,
		input.Enabled,
		input.Decision,
		match,
		uc.clock.Now(),
	)
	if err != nil {
		return policydomain.Rule{}, fmt.Errorf("create policy rule: %w", err)
	}
	if err := uc.repository.CreateRule(ctx, rule); err != nil {
		return policydomain.Rule{}, fmt.Errorf("create policy rule: %w", err)
	}
	return rule, nil
}

type UpdateRule struct {
	repository Repository
	clock      Clock
	references ReferenceLookup
}

func NewUpdateRule(
	repository Repository,
	clock Clock,
	references ReferenceLookup,
) UpdateRule {
	return UpdateRule{
		repository: repository,
		clock:      clock,
		references: references,
	}
}

func (uc UpdateRule) Execute(
	ctx context.Context,
	id policydomain.RuleID,
	input RuleInput,
) (policydomain.Rule, error) {
	existing, err := uc.repository.GetRule(ctx, id)
	if err != nil {
		return policydomain.Rule{}, fmt.Errorf("update policy rule: %w", err)
	}
	match, err := policydomain.NewMatch(input.Match)
	if err != nil {
		return policydomain.Rule{}, fmt.Errorf("update policy rule: %w", err)
	}
	if err := validateReferences(ctx, uc.references, match); err != nil {
		return policydomain.Rule{}, fmt.Errorf("update policy rule: %w", err)
	}
	updated, err := existing.Update(
		input.DisplayName,
		input.Decision,
		match,
		uc.clock.Now(),
	)
	if err != nil {
		return policydomain.Rule{}, fmt.Errorf("update policy rule: %w", err)
	}
	updated, err = updated.SetEnabled(input.Enabled, uc.clock.Now())
	if err != nil {
		return policydomain.Rule{}, fmt.Errorf("update policy rule: %w", err)
	}
	if err := uc.repository.UpdateRule(ctx, updated); err != nil {
		return policydomain.Rule{}, fmt.Errorf("update policy rule: %w", err)
	}
	return updated, nil
}

type DeleteRule struct {
	repository Repository
}

func NewDeleteRule(repository Repository) DeleteRule {
	return DeleteRule{repository: repository}
}

func (uc DeleteRule) Execute(ctx context.Context, id policydomain.RuleID) error {
	if err := uc.repository.DeleteRule(ctx, id); err != nil {
		return fmt.Errorf("delete policy rule: %w", err)
	}
	return nil
}

type SetRuleEnabled struct {
	repository Repository
	clock      Clock
}

func NewSetRuleEnabled(repository Repository, clock Clock) SetRuleEnabled {
	return SetRuleEnabled{repository: repository, clock: clock}
}

func (uc SetRuleEnabled) Execute(
	ctx context.Context,
	id policydomain.RuleID,
	enabled bool,
) (policydomain.Rule, error) {
	rule, err := uc.repository.GetRule(ctx, id)
	if err != nil {
		return policydomain.Rule{}, fmt.Errorf("set policy rule enabled: %w", err)
	}
	updated, err := rule.SetEnabled(enabled, uc.clock.Now())
	if err != nil {
		return policydomain.Rule{}, fmt.Errorf("set policy rule enabled: %w", err)
	}
	if err := uc.repository.UpdateRule(ctx, updated); err != nil {
		return policydomain.Rule{}, fmt.Errorf("set policy rule enabled: %w", err)
	}
	return updated, nil
}

type Evaluate struct {
	repository Repository
}

func NewEvaluate(repository Repository) Evaluate {
	return Evaluate{repository: repository}
}

func (uc Evaluate) Execute(
	ctx context.Context,
	input policydomain.EvaluationInput,
) (policydomain.Decision, error) {
	policy, err := uc.repository.GetPolicySet(ctx)
	if err != nil {
		return policydomain.Decision{}, fmt.Errorf("evaluate policy: %w", err)
	}
	decision, err := policy.Evaluate(input)
	if err != nil {
		return policydomain.Decision{}, fmt.Errorf("evaluate policy: %w", err)
	}
	return decision, nil
}

func validateReferences(
	ctx context.Context,
	lookup ReferenceLookup,
	match policydomain.Match,
) error {
	for _, ref := range match.ProfileRefs() {
		exists, err := lookup.ProfileExists(ctx, ref)
		if err != nil {
			return fmt.Errorf("validate Profile reference %s: %w", ref, err)
		}
		if !exists {
			return fmt.Errorf("%w: %s", ErrProfileReferenceNotFound, ref)
		}
	}
	for _, id := range match.ProviderIDs() {
		exists, err := lookup.ProviderExists(ctx, id)
		if err != nil {
			return fmt.Errorf("validate Provider reference %s: %w", id, err)
		}
		if !exists {
			return fmt.Errorf("%w: %s", ErrProviderReferenceNotFound, id)
		}
	}
	for _, id := range match.ToolIDs() {
		exists, err := lookup.ToolExists(ctx, id)
		if err != nil {
			return fmt.Errorf("validate Tool reference %s: %w", id, err)
		}
		if !exists {
			return fmt.Errorf("%w: %s", ErrToolReferenceNotFound, id)
		}
	}
	return nil
}

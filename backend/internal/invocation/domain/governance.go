package domain

import (
	"fmt"
	"slices"
	"strings"
)

const (
	maxGovernanceEffects   = 32
	maxGovernanceResources = 64
	maxFactKindLength      = 64
	maxFactValueLength     = 1024
)

// FactKnowledge states whether governance facts could be established safely.
type FactKnowledge string

const (
	FactKnowledgeUnknown FactKnowledge = "unknown"
	FactKnowledgeKnown   FactKnowledge = "known"
)

// EffectFact is a bounded policy-facing effect classification.
type EffectFact string

// ResourceFact is one bounded resource identifier exposed to Policy.
type ResourceFact struct {
	kind  string
	value string
}

// NewResourceFact constructs one bounded resource fact.
func NewResourceFact(kind, value string) (ResourceFact, error) {
	kind = strings.TrimSpace(kind)
	value = strings.TrimSpace(value)
	if kind == "" {
		return ResourceFact{}, validationError("resource.kind", "must not be empty")
	}
	if len(kind) > maxFactKindLength {
		return ResourceFact{}, validationError("resource.kind", "is too long")
	}
	if value == "" {
		return ResourceFact{}, validationError("resource.value", "must not be empty")
	}
	if len(value) > maxFactValueLength {
		return ResourceFact{}, validationError("resource.value", "is too long")
	}
	return ResourceFact{kind: kind, value: value}, nil
}

func (f ResourceFact) Kind() string {
	return f.kind
}

func (f ResourceFact) Value() string {
	return f.value
}

// GovernanceFacts is the bounded argument-derived projection consumed by Policy.
type GovernanceFacts struct {
	knowledge FactKnowledge
	effects   []EffectFact
	resources []ResourceFact
}

// UnknownGovernanceFacts reports that no safe fact projection is available.
func UnknownGovernanceFacts() GovernanceFacts {
	return GovernanceFacts{knowledge: FactKnowledgeUnknown}
}

// KnownGovernanceFacts constructs a bounded fact set.
func KnownGovernanceFacts(
	effects []EffectFact,
	resources []ResourceFact,
) (GovernanceFacts, error) {
	if len(effects) > maxGovernanceEffects {
		return GovernanceFacts{}, validationError("effects", "contains too many entries")
	}
	if len(resources) > maxGovernanceResources {
		return GovernanceFacts{}, validationError("resources", "contains too many entries")
	}

	normalizedEffects := make([]EffectFact, 0, len(effects))
	seenEffects := make(map[string]struct{}, len(effects))
	for _, effect := range effects {
		value := strings.TrimSpace(string(effect))
		if value == "" {
			return GovernanceFacts{}, validationError("effects", "must not contain empty values")
		}
		if len(value) > maxFactKindLength {
			return GovernanceFacts{}, validationError("effects", "contains a value that is too long")
		}
		if _, exists := seenEffects[value]; exists {
			continue
		}
		seenEffects[value] = struct{}{}
		normalizedEffects = append(normalizedEffects, EffectFact(value))
	}
	slices.SortFunc(normalizedEffects, func(a, b EffectFact) int {
		return strings.Compare(string(a), string(b))
	})

	normalizedResources := make([]ResourceFact, len(resources))
	for i, resource := range resources {
		validated, err := NewResourceFact(resource.kind, resource.value)
		if err != nil {
			return GovernanceFacts{}, fmt.Errorf("resources[%d]: %w", i, err)
		}
		normalizedResources[i] = validated
	}

	return GovernanceFacts{
		knowledge: FactKnowledgeKnown,
		effects:   normalizedEffects,
		resources: normalizedResources,
	}, nil
}

// Validate rejects an absent/zero fact state. Extractors must explicitly
// report either known or unknown knowledge.
func (f GovernanceFacts) Validate() error {
	switch f.knowledge {
	case FactKnowledgeUnknown:
		if len(f.effects) != 0 || len(f.resources) != 0 {
			return validationError("governance_facts", "unknown facts must not contain values")
		}
		return nil
	case FactKnowledgeKnown:
		_, err := KnownGovernanceFacts(f.effects, f.resources)
		return err
	default:
		return validationError("governance_facts.knowledge", fmt.Sprintf("unknown value %q", f.knowledge))
	}
}

func (f GovernanceFacts) Knowledge() FactKnowledge {
	return f.knowledge
}

func (f GovernanceFacts) Effects() []EffectFact {
	return slices.Clone(f.effects)
}

func (f GovernanceFacts) Resources() []ResourceFact {
	return slices.Clone(f.resources)
}

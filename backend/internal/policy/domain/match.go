package domain

import (
	"fmt"
	"slices"
	"strings"
)

const (
	maxSelectorValues   = 64
	maxSelectorValueLen = 255
	maxEffectValues     = 32
	maxResourceMatchers = 64
	maxFactKindLen      = 64
	maxFactValueLen     = 1024
)

type Origin string

const (
	OriginCLI            Origin = "cli"
	OriginMCP            Origin = "mcp"
	OriginManagementHTTP Origin = "management_http"
	OriginDesktop        Origin = "desktop"
	OriginInternal       Origin = "internal"
)

type ScopeKind string

const (
	ScopeCatalog ScopeKind = "catalog"
	ScopeProfile ScopeKind = "profile"
)

type FactKnowledge string

const (
	FactAny     FactKnowledge = "any"
	FactKnown   FactKnowledge = "known"
	FactUnknown FactKnowledge = "unknown"
)

type ResourceOperator string

const (
	ResourceExact  ResourceOperator = "exact"
	ResourcePrefix ResourceOperator = "prefix"
)

type ResourceMatcher struct {
	kind     string
	operator ResourceOperator
	value    string
}

func NewResourceMatcher(
	kind string,
	operator ResourceOperator,
	value string,
) (ResourceMatcher, error) {
	kind = strings.TrimSpace(kind)
	value = strings.TrimSpace(value)
	if kind == "" {
		return ResourceMatcher{}, fmt.Errorf("resource.kind: must not be empty")
	}
	if len(kind) > maxFactKindLen {
		return ResourceMatcher{}, fmt.Errorf("resource.kind: is too long")
	}
	if value == "" {
		return ResourceMatcher{}, fmt.Errorf("resource.value: must not be empty")
	}
	if len(value) > maxFactValueLen {
		return ResourceMatcher{}, fmt.Errorf("resource.value: is too long")
	}
	switch operator {
	case ResourceExact, ResourcePrefix:
	default:
		return ResourceMatcher{}, fmt.Errorf("resource.operator: unknown value %q", operator)
	}
	return ResourceMatcher{kind: kind, operator: operator, value: value}, nil
}

func (m ResourceMatcher) Kind() string {
	return m.kind
}

func (m ResourceMatcher) Operator() ResourceOperator {
	return m.operator
}

func (m ResourceMatcher) Value() string {
	return m.value
}

type ResourceFact struct {
	Kind  string
	Value string
}

type MatchSpec struct {
	Origins       []Origin
	ScopeKinds    []ScopeKind
	ProfileRefs   []string
	ProviderIDs   []string
	ToolIDs       []string
	FactKnowledge FactKnowledge
	EffectsAny    []string
	ResourcesAny  []ResourceMatcher
}

type Match struct {
	origins       []Origin
	scopeKinds    []ScopeKind
	profileRefs   []string
	providerIDs   []string
	toolIDs       []string
	factKnowledge FactKnowledge
	effectsAny    []string
	resourcesAny  []ResourceMatcher
}

func NewMatch(spec MatchSpec) (Match, error) {
	origins, err := normalizeOrigins(spec.Origins)
	if err != nil {
		return Match{}, err
	}
	scopeKinds, err := normalizeScopeKinds(spec.ScopeKinds)
	if err != nil {
		return Match{}, err
	}
	profileRefs, err := normalizeStrings("profile_refs", spec.ProfileRefs, maxSelectorValues, maxSelectorValueLen)
	if err != nil {
		return Match{}, err
	}
	providerIDs, err := normalizeStrings("provider_ids", spec.ProviderIDs, maxSelectorValues, maxSelectorValueLen)
	if err != nil {
		return Match{}, err
	}
	toolIDs, err := normalizeStrings("tool_ids", spec.ToolIDs, maxSelectorValues, maxSelectorValueLen)
	if err != nil {
		return Match{}, err
	}
	factKnowledge := spec.FactKnowledge
	if factKnowledge == "" {
		factKnowledge = FactAny
	}
	switch factKnowledge {
	case FactAny, FactKnown, FactUnknown:
	default:
		return Match{}, fmt.Errorf("fact_knowledge: unknown value %q", factKnowledge)
	}
	effects, err := normalizeStrings("effects_any", spec.EffectsAny, maxEffectValues, maxFactKindLen)
	if err != nil {
		return Match{}, err
	}
	if len(spec.ResourcesAny) > maxResourceMatchers {
		return Match{}, fmt.Errorf("resources_any: contains too many entries")
	}
	resources := make([]ResourceMatcher, 0, len(spec.ResourcesAny))
	seenResources := make(map[string]struct{}, len(spec.ResourcesAny))
	for i, resource := range spec.ResourcesAny {
		validated, err := NewResourceMatcher(resource.Kind(), resource.Operator(), resource.Value())
		if err != nil {
			return Match{}, fmt.Errorf("resources_any[%d]: %w", i, err)
		}
		key := validated.Kind() + "\x00" + string(validated.Operator()) + "\x00" + validated.Value()
		if _, exists := seenResources[key]; exists {
			continue
		}
		seenResources[key] = struct{}{}
		resources = append(resources, validated)
	}
	slices.SortFunc(resources, func(a, b ResourceMatcher) int {
		if result := strings.Compare(a.Kind(), b.Kind()); result != 0 {
			return result
		}
		if result := strings.Compare(string(a.Operator()), string(b.Operator())); result != 0 {
			return result
		}
		return strings.Compare(a.Value(), b.Value())
	})

	if len(profileRefs) != 0 &&
		len(scopeKinds) != 0 &&
		!slices.Contains(scopeKinds, ScopeProfile) {
		return Match{}, fmt.Errorf("profile_refs: require profile scope")
	}
	if factKnowledge == FactUnknown && (len(effects) != 0 || len(resources) != 0) {
		return Match{}, fmt.Errorf(
			"fact_knowledge: unknown cannot be combined with effect/resource predicates",
		)
	}

	return Match{
		origins:       origins,
		scopeKinds:    scopeKinds,
		profileRefs:   profileRefs,
		providerIDs:   providerIDs,
		toolIDs:       toolIDs,
		factKnowledge: factKnowledge,
		effectsAny:    effects,
		resourcesAny:  resources,
	}, nil
}

func (m Match) Origins() []Origin {
	return slices.Clone(m.origins)
}

func (m Match) ScopeKinds() []ScopeKind {
	return slices.Clone(m.scopeKinds)
}

func (m Match) ProfileRefs() []string {
	return slices.Clone(m.profileRefs)
}

func (m Match) ProviderIDs() []string {
	return slices.Clone(m.providerIDs)
}

func (m Match) ToolIDs() []string {
	return slices.Clone(m.toolIDs)
}

func (m Match) FactKnowledge() FactKnowledge {
	return m.factKnowledge
}

func (m Match) EffectsAny() []string {
	return slices.Clone(m.effectsAny)
}

func (m Match) ResourcesAny() []ResourceMatcher {
	return slices.Clone(m.resourcesAny)
}

func (m Match) IsWildcard() bool {
	return len(m.origins) == 0 &&
		len(m.scopeKinds) == 0 &&
		len(m.profileRefs) == 0 &&
		len(m.providerIDs) == 0 &&
		len(m.toolIDs) == 0 &&
		m.factKnowledge == FactAny &&
		len(m.effectsAny) == 0 &&
		len(m.resourcesAny) == 0
}

type EvaluationInput struct {
	Origin         Origin
	ScopeKind      ScopeKind
	ScopeReference string
	ProviderID     string
	ToolID         string
	FactKnowledge  FactKnowledge
	Effects        []string
	Resources      []ResourceFact
}

func (i EvaluationInput) Validate() error {
	switch i.Origin {
	case OriginCLI, OriginMCP, OriginManagementHTTP, OriginDesktop, OriginInternal:
	default:
		return fmt.Errorf("evaluation.origin: unknown value %q", i.Origin)
	}
	switch i.ScopeKind {
	case ScopeCatalog:
		if strings.TrimSpace(i.ScopeReference) != "" {
			return fmt.Errorf("evaluation.scope_reference: must be empty for catalog scope")
		}
	case ScopeProfile:
		if strings.TrimSpace(i.ScopeReference) == "" {
			return fmt.Errorf("evaluation.scope_reference: must not be empty for profile scope")
		}
		if len(i.ScopeReference) > maxSelectorValueLen {
			return fmt.Errorf("evaluation.scope_reference: is too long")
		}
	default:
		return fmt.Errorf("evaluation.scope_kind: unknown value %q", i.ScopeKind)
	}
	if strings.TrimSpace(i.ProviderID) == "" || len(i.ProviderID) > maxSelectorValueLen {
		return fmt.Errorf("evaluation.provider_id: must be non-empty and bounded")
	}
	if strings.TrimSpace(i.ToolID) == "" || len(i.ToolID) > maxSelectorValueLen {
		return fmt.Errorf("evaluation.tool_id: must be non-empty and bounded")
	}
	switch i.FactKnowledge {
	case FactKnown, FactUnknown:
	default:
		return fmt.Errorf("evaluation.fact_knowledge: unknown value %q", i.FactKnowledge)
	}
	if len(i.Effects) > maxEffectValues {
		return fmt.Errorf("evaluation.effects: contains too many entries")
	}
	for _, effect := range i.Effects {
		value := strings.TrimSpace(effect)
		if value == "" || len(value) > maxFactKindLen {
			return fmt.Errorf("evaluation.effects: contains invalid value")
		}
	}
	if len(i.Resources) > maxResourceMatchers {
		return fmt.Errorf("evaluation.resources: contains too many entries")
	}
	for index, resource := range i.Resources {
		kind := strings.TrimSpace(resource.Kind)
		value := strings.TrimSpace(resource.Value)
		if kind == "" || len(kind) > maxFactKindLen || value == "" || len(value) > maxFactValueLen {
			return fmt.Errorf("evaluation.resources[%d]: contains invalid value", index)
		}
	}
	if i.FactKnowledge == FactUnknown && (len(i.Effects) != 0 || len(i.Resources) != 0) {
		return fmt.Errorf("evaluation: unknown facts must not contain effect/resource values")
	}
	return nil
}

func (m Match) Matches(input EvaluationInput) bool {
	if len(m.origins) != 0 && !slices.Contains(m.origins, input.Origin) {
		return false
	}
	if len(m.scopeKinds) != 0 && !slices.Contains(m.scopeKinds, input.ScopeKind) {
		return false
	}
	if len(m.profileRefs) != 0 {
		if input.ScopeKind != ScopeProfile ||
			!slices.Contains(m.profileRefs, input.ScopeReference) {
			return false
		}
	}
	if len(m.providerIDs) != 0 && !slices.Contains(m.providerIDs, input.ProviderID) {
		return false
	}
	if len(m.toolIDs) != 0 && !slices.Contains(m.toolIDs, input.ToolID) {
		return false
	}
	if m.factKnowledge != FactAny && m.factKnowledge != input.FactKnowledge {
		return false
	}
	if len(m.effectsAny) != 0 {
		if input.FactKnowledge != FactKnown || !hasAnyEffect(m.effectsAny, input.Effects) {
			return false
		}
	}
	if len(m.resourcesAny) != 0 {
		if input.FactKnowledge != FactKnown || !hasAnyResource(m.resourcesAny, input.Resources) {
			return false
		}
	}
	return true
}

func hasAnyEffect(expected, actual []string) bool {
	for _, candidate := range expected {
		if slices.Contains(actual, candidate) {
			return true
		}
	}
	return false
}

func hasAnyResource(matchers []ResourceMatcher, facts []ResourceFact) bool {
	for _, matcher := range matchers {
		for _, fact := range facts {
			if matcher.Kind() != fact.Kind {
				continue
			}
			switch matcher.Operator() {
			case ResourceExact:
				if fact.Value == matcher.Value() {
					return true
				}
			case ResourcePrefix:
				if strings.HasPrefix(fact.Value, matcher.Value()) {
					return true
				}
			}
		}
	}
	return false
}

func normalizeOrigins(values []Origin) ([]Origin, error) {
	if len(values) > maxSelectorValues {
		return nil, fmt.Errorf("origins: contains too many entries")
	}
	result := make([]Origin, 0, len(values))
	seen := make(map[Origin]struct{}, len(values))
	for _, value := range values {
		switch value {
		case OriginCLI, OriginMCP, OriginManagementHTTP, OriginDesktop, OriginInternal:
		default:
			return nil, fmt.Errorf("origins: unknown value %q", value)
		}
		if _, exists := seen[value]; exists {
			continue
		}
		seen[value] = struct{}{}
		result = append(result, value)
	}
	slices.Sort(result)
	return result, nil
}

func normalizeScopeKinds(values []ScopeKind) ([]ScopeKind, error) {
	if len(values) > maxSelectorValues {
		return nil, fmt.Errorf("scope_kinds: contains too many entries")
	}
	result := make([]ScopeKind, 0, len(values))
	seen := make(map[ScopeKind]struct{}, len(values))
	for _, value := range values {
		switch value {
		case ScopeCatalog, ScopeProfile:
		default:
			return nil, fmt.Errorf("scope_kinds: unknown value %q", value)
		}
		if _, exists := seen[value]; exists {
			continue
		}
		seen[value] = struct{}{}
		result = append(result, value)
	}
	slices.Sort(result)
	return result, nil
}

func normalizeStrings(
	field string,
	values []string,
	maxItems int,
	maxLength int,
) ([]string, error) {
	if len(values) > maxItems {
		return nil, fmt.Errorf("%s: contains too many entries", field)
	}
	result := make([]string, 0, len(values))
	seen := make(map[string]struct{}, len(values))
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value == "" {
			return nil, fmt.Errorf("%s: contains empty value", field)
		}
		if len(value) > maxLength {
			return nil, fmt.Errorf("%s: contains value that is too long", field)
		}
		if _, exists := seen[value]; exists {
			continue
		}
		seen[value] = struct{}{}
		result = append(result, value)
	}
	slices.Sort(result)
	return result, nil
}

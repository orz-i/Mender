package domain

import (
	"encoding/hex"
	"fmt"
	"slices"
	"strings"

	governancedomain "github.com/orz-i/Mender/backend/internal/governance/domain"
)

const (
	maxSelectorValues   = 64
	maxSelectorValueLen = 255
	maxEffectValues     = 32
	maxResourceMatchers = 64
	maxFactKindLen      = 64
	maxFactValueLen     = 1024
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

func (m ResourceMatcher) Kind() string               { return m.kind }
func (m ResourceMatcher) Operator() ResourceOperator { return m.operator }
func (m ResourceMatcher) Value() string              { return m.value }

type ResourceFact struct {
	Kind  string
	Value string
}

type MatchSpec struct {
	Actions        []governancedomain.Action
	Origins        []governancedomain.Origin
	PrincipalKinds []governancedomain.PrincipalKind
	PrincipalRefs  []string
	ScopeKinds     []governancedomain.ScopeKind
	ProfileRefs    []string
	ProviderIDs    []string
	ToolIDs        []string
	FactKnowledge  FactKnowledge
	EffectsAny     []string
	ResourcesAny   []ResourceMatcher
}

type Match struct {
	actions        []governancedomain.Action
	origins        []governancedomain.Origin
	principalKinds []governancedomain.PrincipalKind
	principalRefs  []string
	scopeKinds     []governancedomain.ScopeKind
	profileRefs    []string
	providerIDs    []string
	toolIDs        []string
	factKnowledge  FactKnowledge
	effectsAny     []string
	resourcesAny   []ResourceMatcher
}

func NewMatch(spec MatchSpec) (Match, error) {
	actions, err := normalizeActions(spec.Actions)
	if err != nil {
		return Match{}, err
	}
	origins, err := normalizeOrigins(spec.Origins)
	if err != nil {
		return Match{}, err
	}
	principalKinds, err := normalizePrincipalKinds(spec.PrincipalKinds)
	if err != nil {
		return Match{}, err
	}
	principalRefs, err := normalizeStrings(
		"principal_refs",
		spec.PrincipalRefs,
		maxSelectorValues,
		maxSelectorValueLen,
	)
	if err != nil {
		return Match{}, err
	}
	scopeKinds, err := normalizeScopeKinds(spec.ScopeKinds)
	if err != nil {
		return Match{}, err
	}
	profileRefs, err := normalizeStrings(
		"profile_refs",
		spec.ProfileRefs,
		maxSelectorValues,
		maxSelectorValueLen,
	)
	if err != nil {
		return Match{}, err
	}
	providerIDs, err := normalizeStrings(
		"provider_ids",
		spec.ProviderIDs,
		maxSelectorValues,
		maxSelectorValueLen,
	)
	if err != nil {
		return Match{}, err
	}
	toolIDs, err := normalizeStrings(
		"tool_ids",
		spec.ToolIDs,
		maxSelectorValues,
		maxSelectorValueLen,
	)
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

	effects, err := normalizeStrings(
		"effects_any",
		spec.EffectsAny,
		maxEffectValues,
		maxFactKindLen,
	)
	if err != nil {
		return Match{}, err
	}
	if len(spec.ResourcesAny) > maxResourceMatchers {
		return Match{}, fmt.Errorf("resources_any: contains too many entries")
	}
	resources := make([]ResourceMatcher, 0, len(spec.ResourcesAny))
	seenResources := make(map[string]struct{}, len(spec.ResourcesAny))
	for i, resource := range spec.ResourcesAny {
		validated, validateErr := NewResourceMatcher(
			resource.Kind(),
			resource.Operator(),
			resource.Value(),
		)
		if validateErr != nil {
			return Match{}, fmt.Errorf("resources_any[%d]: %w", i, validateErr)
		}
		key := validated.Kind() + "\x00" +
			string(validated.Operator()) + "\x00" +
			validated.Value()
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
		!slices.Contains(scopeKinds, governancedomain.ScopeProfile) {
		return Match{}, fmt.Errorf("profile_refs: require profile scope")
	}
	if factKnowledge == FactUnknown && (len(effects) != 0 || len(resources) != 0) {
		return Match{}, fmt.Errorf(
			"fact_knowledge: unknown cannot be combined with effect/resource predicates",
		)
	}
	if (hasSandboxAction(actions) || hasAgentAction(actions)) &&
		(len(providerIDs) != 0 || len(toolIDs) != 0 ||
			(slices.Contains(actions, governancedomain.ActionToolInvoke) ||
				slices.Contains(actions, governancedomain.ActionProviderActivate))) {
		return Match{}, fmt.Errorf("resource action rules cannot use Provider/Tool selectors or mix with Tool/Provider actions")
	}
	if slices.Contains(actions, governancedomain.ActionProviderActivate) &&
		(len(toolIDs) != 0 ||
			factKnowledge != FactAny ||
			len(effects) != 0 ||
			len(resources) != 0) {
		return Match{}, fmt.Errorf(
			"provider.activate rules cannot contain tool or governance-fact predicates",
		)
	}

	return Match{
		actions:        actions,
		origins:        origins,
		principalKinds: principalKinds,
		principalRefs:  principalRefs,
		scopeKinds:     scopeKinds,
		profileRefs:    profileRefs,
		providerIDs:    providerIDs,
		toolIDs:        toolIDs,
		factKnowledge:  factKnowledge,
		effectsAny:     effects,
		resourcesAny:   resources,
	}, nil
}

func (m Match) Actions() []governancedomain.Action {
	return slices.Clone(m.actions)
}

func (m Match) Origins() []governancedomain.Origin {
	return slices.Clone(m.origins)
}

func (m Match) PrincipalKinds() []governancedomain.PrincipalKind {
	return slices.Clone(m.principalKinds)
}

func (m Match) PrincipalRefs() []string {
	return slices.Clone(m.principalRefs)
}

func (m Match) ScopeKinds() []governancedomain.ScopeKind {
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

// IsWildcard reports whether all selectors except the mandatory Action selector are wildcards.
func (m Match) IsWildcard() bool {
	return len(m.actions) != 0 &&
		len(m.origins) == 0 &&
		len(m.principalKinds) == 0 &&
		len(m.principalRefs) == 0 &&
		len(m.scopeKinds) == 0 &&
		len(m.profileRefs) == 0 &&
		len(m.providerIDs) == 0 &&
		len(m.toolIDs) == 0 &&
		m.factKnowledge == FactAny &&
		len(m.effectsAny) == 0 &&
		len(m.resourcesAny) == 0
}

type EvaluationInput struct {
	Action             governancedomain.Action
	Origin             governancedomain.Origin
	PrincipalKind      governancedomain.PrincipalKind
	PrincipalReference string
	ScopeKind          governancedomain.ScopeKind
	ScopeReference     string
	ProviderID         string
	ToolID             string
	FactKnowledge      FactKnowledge
	Effects            []string
	Resources          []ResourceFact
}

func (i EvaluationInput) Validate() error {
	if err := i.Action.Validate(); err != nil {
		return fmt.Errorf("evaluation.action: %w", err)
	}
	principal, err := governancedomain.NewPrincipal(
		i.PrincipalKind,
		i.PrincipalReference,
	)
	if err != nil {
		return fmt.Errorf("evaluation.principal: %w", err)
	}
	var scope governancedomain.Scope
	switch i.ScopeKind {
	case governancedomain.ScopeCatalog:
		if strings.TrimSpace(i.ScopeReference) != "" {
			return fmt.Errorf("evaluation.scope_reference: must be empty for catalog scope")
		}
		scope = governancedomain.CatalogScope()
	case governancedomain.ScopeProfile:
		scope, err = governancedomain.ProfileScope(i.ScopeReference)
		if err != nil {
			return fmt.Errorf("evaluation.scope_reference: %w", err)
		}
	default:
		return fmt.Errorf("evaluation.scope_kind: unknown value %q", i.ScopeKind)
	}
	if _, err := governancedomain.NewContext(i.Origin, principal, scope); err != nil {
		return fmt.Errorf("evaluation.context: %w", err)
	}
	if !isSandboxAction(i.Action) && !isAgentAction(i.Action) &&
		(strings.TrimSpace(i.ProviderID) == "" || len(i.ProviderID) > maxSelectorValueLen) {
		return fmt.Errorf("evaluation.provider_id: must be non-empty and bounded")
	}

	switch i.Action {
	case governancedomain.ActionToolInvoke:
		if strings.TrimSpace(i.ToolID) == "" || len(i.ToolID) > maxSelectorValueLen {
			return fmt.Errorf("evaluation.tool_id: must be non-empty and bounded")
		}
		switch i.FactKnowledge {
		case FactKnown, FactUnknown:
		default:
			return fmt.Errorf(
				"evaluation.fact_knowledge: unknown value %q",
				i.FactKnowledge,
			)
		}
		if err := validateEvaluationFacts(i); err != nil {
			return err
		}
	case governancedomain.ActionSandboxAcquire, governancedomain.ActionSandboxProvision,
		governancedomain.ActionSandboxStart, governancedomain.ActionSandboxStop,
		governancedomain.ActionSandboxDestroy, governancedomain.ActionSandboxReconcile:
		if i.ProviderID != "" || i.ToolID != "" {
			return fmt.Errorf("evaluation: sandbox action cannot target Provider or Tool")
		}
		if i.FactKnowledge != FactKnown {
			return fmt.Errorf("evaluation: sandbox action requires reviewed governance facts")
		}
		if err := validateEvaluationFacts(i); err != nil {
			return err
		}
		var sandboxID bool
		for _, resource := range i.Resources {
			if resource.Kind == "sandbox_id" && len(resource.Value) == 36 &&
				strings.HasPrefix(resource.Value, "sbx_") &&
				resource.Value == strings.ToLower(resource.Value) {
				if _, err := hex.DecodeString(strings.TrimPrefix(resource.Value, "sbx_")); err == nil {
					sandboxID = true
				}
			}
		}
		if !sandboxID {
			return fmt.Errorf("evaluation: sandbox action requires sandbox_id fact")
		}
	case governancedomain.ActionAgentActivate, governancedomain.ActionAgentTaskSubmit, governancedomain.ActionAgentTaskCancel:
		if i.ProviderID != "" || i.ToolID != "" || i.FactKnowledge != FactKnown {
			return fmt.Errorf("evaluation: Agent action must use reviewed resource facts without Provider/Tool selectors")
		}
		if err := validateEvaluationFacts(i); err != nil {
			return err
		}
		found := false
		for _, resource := range i.Resources {
			if resource.Kind == "agent_id" && len(resource.Value) == 38 && strings.HasPrefix(resource.Value, "agent_") {
				if _, err := hex.DecodeString(strings.TrimPrefix(resource.Value, "agent_")); err == nil {
					found = true
				}
			}
		}
		if !found {
			return fmt.Errorf("evaluation: Agent action requires agent_id fact")
		}
	case governancedomain.ActionProviderActivate:
		if strings.TrimSpace(i.ToolID) != "" {
			return fmt.Errorf("evaluation.tool_id: must be empty for provider.activate")
		}
		if i.FactKnowledge != "" || len(i.Effects) != 0 || len(i.Resources) != 0 {
			return fmt.Errorf(
				"evaluation: provider.activate must not contain governance facts",
			)
		}
	}
	return nil
}

func validateEvaluationFacts(i EvaluationInput) error {
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
		if kind == "" ||
			len(kind) > maxFactKindLen ||
			value == "" ||
			len(value) > maxFactValueLen {
			return fmt.Errorf(
				"evaluation.resources[%d]: contains invalid value",
				index,
			)
		}
	}
	if i.FactKnowledge == FactUnknown &&
		(len(i.Effects) != 0 || len(i.Resources) != 0) {
		return fmt.Errorf(
			"evaluation: unknown facts must not contain effect/resource values",
		)
	}
	return nil
}

func (m Match) Matches(input EvaluationInput) bool {
	if !slices.Contains(m.actions, input.Action) {
		return false
	}
	if len(m.origins) != 0 && !slices.Contains(m.origins, input.Origin) {
		return false
	}
	if len(m.principalKinds) != 0 &&
		!slices.Contains(m.principalKinds, input.PrincipalKind) {
		return false
	}
	if len(m.principalRefs) != 0 &&
		!slices.Contains(m.principalRefs, input.PrincipalReference) {
		return false
	}
	if len(m.scopeKinds) != 0 && !slices.Contains(m.scopeKinds, input.ScopeKind) {
		return false
	}
	if len(m.profileRefs) != 0 {
		if input.ScopeKind != governancedomain.ScopeProfile ||
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
	if input.Action == governancedomain.ActionToolInvoke || isSandboxAction(input.Action) || isAgentAction(input.Action) {
		if m.factKnowledge != FactAny && m.factKnowledge != input.FactKnowledge {
			return false
		}
		if len(m.effectsAny) != 0 {
			if input.FactKnowledge != FactKnown ||
				!hasAnyEffect(m.effectsAny, input.Effects) {
				return false
			}
		}
		if len(m.resourcesAny) != 0 {
			if input.FactKnowledge != FactKnown ||
				!hasAnyResource(m.resourcesAny, input.Resources) {
				return false
			}
		}
	}
	return true
}

func isSandboxAction(a governancedomain.Action) bool {
	switch a {
	case governancedomain.ActionSandboxAcquire, governancedomain.ActionSandboxProvision,
		governancedomain.ActionSandboxStart, governancedomain.ActionSandboxStop,
		governancedomain.ActionSandboxDestroy, governancedomain.ActionSandboxReconcile:
		return true
	default:
		return false
	}
}

func isAgentAction(a governancedomain.Action) bool {
	switch a {
	case governancedomain.ActionAgentActivate, governancedomain.ActionAgentTaskSubmit, governancedomain.ActionAgentTaskCancel:
		return true
	default:
		return false
	}
}

func hasAgentAction(actions []governancedomain.Action) bool {
	for _, action := range actions {
		if isAgentAction(action) {
			return true
		}
	}
	return false
}

func hasSandboxAction(actions []governancedomain.Action) bool {
	for _, action := range actions {
		if isSandboxAction(action) {
			return true
		}
	}
	return false
}

func normalizeActions(values []governancedomain.Action) ([]governancedomain.Action, error) {
	if len(values) == 0 {
		return nil, fmt.Errorf("actions: must contain at least one action")
	}
	if len(values) > maxSelectorValues {
		return nil, fmt.Errorf("actions: contains too many entries")
	}
	result := make([]governancedomain.Action, 0, len(values))
	seen := make(map[governancedomain.Action]struct{}, len(values))
	for _, value := range values {
		if err := value.Validate(); err != nil {
			return nil, fmt.Errorf("actions: %w", err)
		}
		if _, ok := seen[value]; ok {
			continue
		}
		seen[value] = struct{}{}
		result = append(result, value)
	}
	slices.SortFunc(result, func(a, b governancedomain.Action) int {
		return strings.Compare(string(a), string(b))
	})
	return result, nil
}

func normalizeOrigins(values []governancedomain.Origin) ([]governancedomain.Origin, error) {
	if len(values) > maxSelectorValues {
		return nil, fmt.Errorf("origins: contains too many entries")
	}
	result := make([]governancedomain.Origin, 0, len(values))
	seen := make(map[governancedomain.Origin]struct{}, len(values))
	for _, value := range values {
		switch value {
		case governancedomain.OriginCLI,
			governancedomain.OriginMCP,
			governancedomain.OriginManagementHTTP,
			governancedomain.OriginDesktop,
			governancedomain.OriginInternal:
		default:
			return nil, fmt.Errorf("origins: unknown value %q", value)
		}
		if _, ok := seen[value]; ok {
			continue
		}
		seen[value] = struct{}{}
		result = append(result, value)
	}
	slices.SortFunc(result, func(a, b governancedomain.Origin) int {
		return strings.Compare(string(a), string(b))
	})
	return result, nil
}

func normalizePrincipalKinds(
	values []governancedomain.PrincipalKind,
) ([]governancedomain.PrincipalKind, error) {
	if len(values) > maxSelectorValues {
		return nil, fmt.Errorf("principal_kinds: contains too many entries")
	}
	result := make([]governancedomain.PrincipalKind, 0, len(values))
	seen := make(map[governancedomain.PrincipalKind]struct{}, len(values))
	for _, value := range values {
		switch value {
		case governancedomain.PrincipalLocalUser, governancedomain.PrincipalService:
		default:
			return nil, fmt.Errorf("principal_kinds: unknown value %q", value)
		}
		if _, ok := seen[value]; ok {
			continue
		}
		seen[value] = struct{}{}
		result = append(result, value)
	}
	slices.SortFunc(result, func(a, b governancedomain.PrincipalKind) int {
		return strings.Compare(string(a), string(b))
	})
	return result, nil
}

func normalizeScopeKinds(
	values []governancedomain.ScopeKind,
) ([]governancedomain.ScopeKind, error) {
	if len(values) > maxSelectorValues {
		return nil, fmt.Errorf("scope_kinds: contains too many entries")
	}
	result := make([]governancedomain.ScopeKind, 0, len(values))
	seen := make(map[governancedomain.ScopeKind]struct{}, len(values))
	for _, value := range values {
		switch value {
		case governancedomain.ScopeCatalog, governancedomain.ScopeProfile:
		default:
			return nil, fmt.Errorf("scope_kinds: unknown value %q", value)
		}
		if _, ok := seen[value]; ok {
			continue
		}
		seen[value] = struct{}{}
		result = append(result, value)
	}
	slices.SortFunc(result, func(a, b governancedomain.ScopeKind) int {
		return strings.Compare(string(a), string(b))
	})
	return result, nil
}

func normalizeStrings(
	field string,
	values []string,
	maxValues int,
	maxLength int,
) ([]string, error) {
	if len(values) > maxValues {
		return nil, fmt.Errorf("%s: contains too many entries", field)
	}
	result := make([]string, 0, len(values))
	seen := make(map[string]struct{}, len(values))
	for _, raw := range values {
		value := strings.TrimSpace(raw)
		if value == "" {
			return nil, fmt.Errorf("%s: contains an empty value", field)
		}
		if len(value) > maxLength {
			return nil, fmt.Errorf("%s: contains a value that is too long", field)
		}
		if _, ok := seen[value]; ok {
			continue
		}
		seen[value] = struct{}{}
		result = append(result, value)
	}
	slices.Sort(result)
	return result, nil
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

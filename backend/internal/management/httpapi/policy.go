package httpapi

import (
	"net/http"

	policyapplication "github.com/orz-i/Mender/backend/internal/policy/application"
	policydomain "github.com/orz-i/Mender/backend/internal/policy/domain"
)

type policyDefaultRequest struct {
	Decision string `json:"decision"`
}

type policyResourceMatcherRequest struct {
	Kind     string `json:"kind"`
	Operator string `json:"operator"`
	Value    string `json:"value"`
}

type policyMatchRequest struct {
	Origins       []string                       `json:"origins"`
	ScopeKinds    []string                       `json:"scopeKinds"`
	ProfileRefs   []string                       `json:"profileRefs"`
	ProviderIDs   []string                       `json:"providerIds"`
	ToolIDs       []string                       `json:"toolIds"`
	FactKnowledge string                         `json:"factKnowledge"`
	EffectsAny    []string                       `json:"effectsAny"`
	ResourcesAny  []policyResourceMatcherRequest `json:"resourcesAny"`
}

type policyRuleRequest struct {
	DisplayName string             `json:"displayName"`
	Enabled     bool               `json:"enabled"`
	Decision    string             `json:"decision"`
	Match       policyMatchRequest `json:"match"`
}

func (h *Handler) getPolicy(w http.ResponseWriter, r *http.Request) {
	policy, err := h.runtime.Policy.Get.Execute(r.Context())
	if err != nil {
		writeKnownError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, policyProjection(policy))
}

func (h *Handler) setPolicyDefault(w http.ResponseWriter, r *http.Request) {
	var input policyDefaultRequest
	if err := decodeJSON(r, &input); err != nil {
		writeAPIError(w, http.StatusBadRequest, "validation_error", err)
		return
	}
	policy, err := h.runtime.Policy.SetDefault.Execute(
		r.Context(),
		policydomain.DecisionKind(input.Decision),
	)
	if err != nil {
		writeKnownError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, policyProjection(policy))
}

func (h *Handler) createPolicyRule(w http.ResponseWriter, r *http.Request) {
	var input policyRuleRequest
	if err := decodeJSON(r, &input); err != nil {
		writeAPIError(w, http.StatusBadRequest, "validation_error", err)
		return
	}
	ruleInput, err := parsePolicyRuleInput(input)
	if err != nil {
		writeAPIError(w, http.StatusBadRequest, "validation_error", err)
		return
	}
	rule, err := h.runtime.Policy.CreateRule.Execute(r.Context(), ruleInput)
	if err != nil {
		writeKnownError(w, err)
		return
	}
	writeJSON(w, http.StatusCreated, policyRuleProjection(rule))
}

func (h *Handler) updatePolicyRule(w http.ResponseWriter, r *http.Request) {
	id, err := policydomain.ParseRuleID(r.PathValue("ruleId"))
	if err != nil {
		writeAPIError(w, http.StatusBadRequest, "validation_error", err)
		return
	}
	var input policyRuleRequest
	if err := decodeJSON(r, &input); err != nil {
		writeAPIError(w, http.StatusBadRequest, "validation_error", err)
		return
	}
	ruleInput, err := parsePolicyRuleInput(input)
	if err != nil {
		writeAPIError(w, http.StatusBadRequest, "validation_error", err)
		return
	}
	rule, err := h.runtime.Policy.UpdateRule.Execute(r.Context(), id, ruleInput)
	if err != nil {
		writeKnownError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, policyRuleProjection(rule))
}

func (h *Handler) deletePolicyRule(w http.ResponseWriter, r *http.Request) {
	id, err := policydomain.ParseRuleID(r.PathValue("ruleId"))
	if err != nil {
		writeAPIError(w, http.StatusBadRequest, "validation_error", err)
		return
	}
	if err := h.runtime.Policy.DeleteRule.Execute(r.Context(), id); err != nil {
		writeKnownError(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (h *Handler) enablePolicyRule(w http.ResponseWriter, r *http.Request) {
	h.setPolicyRuleEnabled(w, r, true)
}

func (h *Handler) disablePolicyRule(w http.ResponseWriter, r *http.Request) {
	h.setPolicyRuleEnabled(w, r, false)
}

func (h *Handler) setPolicyRuleEnabled(
	w http.ResponseWriter,
	r *http.Request,
	enabled bool,
) {
	id, err := policydomain.ParseRuleID(r.PathValue("ruleId"))
	if err != nil {
		writeAPIError(w, http.StatusBadRequest, "validation_error", err)
		return
	}
	rule, err := h.runtime.Policy.SetRuleEnabled.Execute(r.Context(), id, enabled)
	if err != nil {
		writeKnownError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, policyRuleProjection(rule))
}

func parsePolicyRuleInput(input policyRuleRequest) (policyapplication.RuleInput, error) {
	resources := make([]policydomain.ResourceMatcher, 0, len(input.Match.ResourcesAny))
	for _, item := range input.Match.ResourcesAny {
		resource, err := policydomain.NewResourceMatcher(
			item.Kind,
			policydomain.ResourceOperator(item.Operator),
			item.Value,
		)
		if err != nil {
			return policyapplication.RuleInput{}, err
		}
		resources = append(resources, resource)
	}
	return policyapplication.RuleInput{
		DisplayName: input.DisplayName,
		Enabled:     input.Enabled,
		Decision:    policydomain.DecisionKind(input.Decision),
		Match: policydomain.MatchSpec{
			Origins:       policyOrigins(input.Match.Origins),
			ScopeKinds:    policyScopeKinds(input.Match.ScopeKinds),
			ProfileRefs:   append([]string(nil), input.Match.ProfileRefs...),
			ProviderIDs:   append([]string(nil), input.Match.ProviderIDs...),
			ToolIDs:       append([]string(nil), input.Match.ToolIDs...),
			FactKnowledge: policydomain.FactKnowledge(input.Match.FactKnowledge),
			EffectsAny:    append([]string(nil), input.Match.EffectsAny...),
			ResourcesAny:  resources,
		},
	}, nil
}

func policyOrigins(values []string) []policydomain.Origin {
	result := make([]policydomain.Origin, 0, len(values))
	for _, value := range values {
		result = append(result, policydomain.Origin(value))
	}
	return result
}

func policyScopeKinds(values []string) []policydomain.ScopeKind {
	result := make([]policydomain.ScopeKind, 0, len(values))
	for _, value := range values {
		result = append(result, policydomain.ScopeKind(value))
	}
	return result
}

func policyProjection(policy policydomain.PolicySet) map[string]any {
	rules := make([]map[string]any, 0, len(policy.Rules()))
	for _, rule := range policy.Rules() {
		rules = append(rules, policyRuleProjection(rule))
	}
	return map[string]any{
		"defaultDecision": string(policy.DefaultDecision()),
		"rules":           rules,
	}
}

func policyRuleProjection(rule policydomain.Rule) map[string]any {
	match := rule.Match()
	resources := make([]map[string]any, 0, len(match.ResourcesAny()))
	for _, resource := range match.ResourcesAny() {
		resources = append(resources, map[string]any{
			"kind":     resource.Kind(),
			"operator": string(resource.Operator()),
			"value":    resource.Value(),
		})
	}
	return map[string]any{
		"ruleId":      rule.ID().String(),
		"displayName": rule.DisplayName(),
		"enabled":     rule.Enabled(),
		"decision":    string(rule.Decision()),
		"match": map[string]any{
			"origins":       policyOriginStrings(match.Origins()),
			"scopeKinds":    policyScopeKindStrings(match.ScopeKinds()),
			"profileRefs":   match.ProfileRefs(),
			"providerIds":   match.ProviderIDs(),
			"toolIds":       match.ToolIDs(),
			"factKnowledge": string(match.FactKnowledge()),
			"effectsAny":    match.EffectsAny(),
			"resourcesAny":  resources,
		},
		"createdAt": rule.CreatedAt(),
		"updatedAt": rule.UpdatedAt(),
	}
}

func policyOriginStrings(values []policydomain.Origin) []string {
	result := make([]string, 0, len(values))
	for _, value := range values {
		result = append(result, string(value))
	}
	return result
}

func policyScopeKindStrings(values []policydomain.ScopeKind) []string {
	result := make([]string, 0, len(values))
	for _, value := range values {
		result = append(result, string(value))
	}
	return result
}

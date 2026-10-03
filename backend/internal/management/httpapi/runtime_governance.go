package httpapi

import (
	"net/http"

	runtimegovernancedomain "github.com/orz-i/Mender/backend/internal/runtimegovernance/domain"
)

type runtimeBudgetRequest struct {
	MaxInvocations          int   `json:"maxInvocations"`
	MaxPrincipalInvocations int   `json:"maxPrincipalInvocations"`
	MaxProviderInvocations  int   `json:"maxProviderInvocations"`
	MaxToolInvocations      int   `json:"maxToolInvocations"`
	MaxProviderActivations  int   `json:"maxProviderActivations"`
	MaxExecutionTimeoutMS   int64 `json:"maxExecutionTimeoutMs"`
	MaxArgumentBytes        int   `json:"maxArgumentBytes"`
	MaxResultBytes          int   `json:"maxResultBytes"`
}

type runtimeIsolationRequest struct {
	WorkspaceShell   string `json:"workspaceShell"`
	ExternalProvider string `json:"externalProvider"`
}

func (h *Handler) getRuntimeGovernance(w http.ResponseWriter, r *http.Request) {
	budget, err := h.runtime.RuntimeGovernance.GetBudget.Execute(r.Context())
	if err != nil {
		writeKnownError(w, err)
		return
	}
	isolation, err := h.runtime.RuntimeGovernance.GetIsolation.Execute(r.Context())
	if err != nil {
		writeKnownError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, runtimeGovernanceProjection(h, budget, isolation))
}

func (h *Handler) updateRuntimeBudget(w http.ResponseWriter, r *http.Request) {
	var input runtimeBudgetRequest
	if err := decodeJSON(r, &input); err != nil {
		writeAPIError(w, http.StatusBadRequest, "validation_error", err)
		return
	}
	updated, err := h.runtime.RuntimeGovernance.UpdateBudget.Execute(
		r.Context(),
		runtimegovernancedomain.BudgetSpec{
			MaxInvocations:          input.MaxInvocations,
			MaxPrincipalInvocations: input.MaxPrincipalInvocations,
			MaxProviderInvocations:  input.MaxProviderInvocations,
			MaxToolInvocations:      input.MaxToolInvocations,
			MaxProviderActivations:  input.MaxProviderActivations,
			MaxExecutionTimeoutMS:   input.MaxExecutionTimeoutMS,
			MaxArgumentBytes:        input.MaxArgumentBytes,
			MaxResultBytes:          input.MaxResultBytes,
		},
	)
	if err != nil {
		writeAPIError(w, http.StatusBadRequest, "validation_error", err)
		return
	}
	isolation, err := h.runtime.RuntimeGovernance.GetIsolation.Execute(r.Context())
	if err != nil {
		writeKnownError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, runtimeGovernanceProjection(h, updated, isolation))
}

func (h *Handler) updateRuntimeIsolation(w http.ResponseWriter, r *http.Request) {
	var input runtimeIsolationRequest
	if err := decodeJSON(r, &input); err != nil {
		writeAPIError(w, http.StatusBadRequest, "validation_error", err)
		return
	}
	updated, err := h.runtime.RuntimeGovernance.UpdateIsolation(
		r.Context(),
		runtimegovernancedomain.IsolationSpec{
			WorkspaceShell:   runtimegovernancedomain.IsolationMode(input.WorkspaceShell),
			ExternalProvider: runtimegovernancedomain.IsolationMode(input.ExternalProvider),
		},
	)
	if err != nil {
		writeAPIError(w, http.StatusBadRequest, "validation_error", err)
		return
	}
	budget, err := h.runtime.RuntimeGovernance.GetBudget.Execute(r.Context())
	if err != nil {
		writeKnownError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, runtimeGovernanceProjection(h, budget, updated))
}

func runtimeGovernanceProjection(
	h *Handler,
	budget runtimegovernancedomain.Budget,
	isolation runtimegovernancedomain.Isolation,
) map[string]any {
	spec := budget.Spec()
	backend := h.runtime.RuntimeGovernance.IsolationBackend()
	usage := h.runtime.RuntimeGovernance.BudgetUsage()
	return map[string]any{
		"budget": map[string]any{
			"maxInvocations":          spec.MaxInvocations,
			"maxPrincipalInvocations": spec.MaxPrincipalInvocations,
			"maxProviderInvocations":  spec.MaxProviderInvocations,
			"maxToolInvocations":      spec.MaxToolInvocations,
			"maxProviderActivations":  spec.MaxProviderActivations,
			"maxExecutionTimeoutMs":   spec.MaxExecutionTimeoutMS,
			"maxArgumentBytes":        spec.MaxArgumentBytes,
			"maxResultBytes":          spec.MaxResultBytes,
		},
		"isolation": map[string]any{
			"workspaceShell":   string(isolation.WorkspaceShell()),
			"externalProvider": string(isolation.ExternalProvider()),
		},
		"backend": map[string]any{
			"name":                    backend.Name,
			"restrictedHostAvailable": backend.RestrictedHostAvailable,
		},
		"usage": map[string]any{
			"invocations": usage.Invocations,
			"activations": usage.Activations,
		},
	}
}

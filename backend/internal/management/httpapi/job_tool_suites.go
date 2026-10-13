package httpapi

import "net/http"

// Read-only supply projection. The owner must explicitly choose to prepare or
// retire the Jobs facade. GET performs no SSH or Provider side effects.
func (h *Handler) getSandboxJobTools(w http.ResponseWriter, r *http.Request) {
	id, caller, err := h.sandboxToolsRequest(r)
	if err != nil {
		writeAPIError(w, http.StatusBadRequest, "validation_error", err)
		return
	}
	state, err := h.runtime.SandboxJobTools(r.Context(), id, caller)
	if err != nil {
		h.workspaceSuiteError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, state)
}
func (h *Handler) ensureSandboxJobTools(w http.ResponseWriter, r *http.Request) {
	id, caller, err := h.sandboxToolsRequest(r)
	if err != nil {
		writeAPIError(w, http.StatusBadRequest, "validation_error", err)
		return
	}
	state, err := h.runtime.EnsureSandboxJobTools(r.Context(), id, caller)
	if err != nil {
		h.workspaceSuiteError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, state)
}
func (h *Handler) removeSandboxJobTools(w http.ResponseWriter, r *http.Request) {
	id, caller, err := h.sandboxToolsRequest(r)
	if err != nil {
		writeAPIError(w, http.StatusBadRequest, "validation_error", err)
		return
	}
	if err = h.runtime.RemoveSandboxJobTools(r.Context(), id, caller); err != nil {
		h.workspaceSuiteError(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

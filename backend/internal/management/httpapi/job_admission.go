package httpapi

import (
	"errors"
	"net/http"
)

// A local owner explicitly toggles *new* SSH Job admissions. This is not
// remote task cancellation and does not require an available SSH host.
func (h *Handler) getSSHJobAdmission(w http.ResponseWriter, r *http.Request) {
	id, caller, err := h.sandboxToolsRequest(r)
	if err != nil {
		writeAPIError(w, http.StatusBadRequest, "validation_error", err)
		return
	}
	state, err := h.runtime.Sandboxes.SSHJobAdmission(r.Context(), id, caller)
	if err != nil {
		h.workspaceSuiteError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, state)
}
func (h *Handler) setSSHJobAdmission(w http.ResponseWriter, r *http.Request) {
	id, caller, err := h.sandboxToolsRequest(r)
	if err != nil {
		writeAPIError(w, http.StatusBadRequest, "validation_error", err)
		return
	}
	var input struct {
		Paused *bool `json:"paused"`
	}
	if err = decodeJSON(r, &input); err != nil || input.Paused == nil {
		if err == nil {
			err = errors.New("paused is required")
		}
		writeAPIError(w, http.StatusBadRequest, "validation_error", err)
		return
	}
	state, err := h.runtime.Sandboxes.SetSSHJobSubmissionsPaused(r.Context(), id, *input.Paused, caller)
	if err != nil {
		h.workspaceSuiteError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, state)
}

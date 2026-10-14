package httpapi

import (
	"errors"
	"net/http"
)

// Owner management-only, one-way v2 SSH Job protocol activation. This never
// appears as a Profile/MCP Tool and requires an explicit confirmation body.
func (h *Handler) activateSSHJobV2(w http.ResponseWriter, r *http.Request) {
	id, caller, err := h.sandboxToolsRequest(r)
	if err != nil {
		writeAPIError(w, http.StatusBadRequest, "validation_error", err)
		return
	}
	var input struct {
		Confirm *bool `json:"confirm"`
	}
	if err = decodeJSON(r, &input); err != nil || input.Confirm == nil || !*input.Confirm {
		if err == nil {
			err = errors.New("explicit confirm=true is required")
		}
		writeAPIError(w, http.StatusBadRequest, "validation_error", err)
		return
	}
	state, err := h.runtime.Sandboxes.ActivateSSHJobV2(r.Context(), id, caller)
	if err != nil {
		h.workspaceSuiteError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, state)
}

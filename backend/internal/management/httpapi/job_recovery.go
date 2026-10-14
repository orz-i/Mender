package httpapi

import "net/http"

// Owner-only, evidence-only SSH Job recovery. Readable while an unknown
// submission prevents normal Stop/Destroy; never edits remote records.
func (h *Handler) getSSHJobRecovery(w http.ResponseWriter, r *http.Request) {
	id, caller, err := h.sandboxToolsRequest(r)
	if err != nil {
		writeAPIError(w, http.StatusBadRequest, "validation_error", err)
		return
	}
	view, err := h.runtime.Sandboxes.InspectSSHJobRecovery(r.Context(), id, caller)
	if err != nil {
		h.workspaceSuiteError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, view)
}

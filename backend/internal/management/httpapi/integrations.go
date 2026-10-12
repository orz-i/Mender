package httpapi

import "net/http"

// An integration offering is public *metadata* available only to authorized
// management callers. It never activates a Provider or contacts the vendor.
func (h *Handler) listIntegrationOfferings(w http.ResponseWriter, r *http.Request) {
	offerings, err := h.runtime.ListIntegrationOfferings(r.Context())
	if err != nil {
		writeKnownError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"integrations": offerings})
}

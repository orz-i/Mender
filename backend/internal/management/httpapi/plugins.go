package httpapi

import (
	"net/http"

	platformplugin "github.com/orz-i/Mender/backend/internal/plugin"
	installationdomain "github.com/orz-i/Mender/backend/internal/plugin/installation/domain"
)

func (h *Handler) listPluginCatalogue(w http.ResponseWriter, r *http.Request) {
	items, err := h.runtime.ListPluginCatalogue(r.Context())
	if err != nil {
		writeKnownError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"plugins": items})
}
func (h *Handler) enablePluginInstallation(w http.ResponseWriter, r *http.Request) {
	h.setPluginStatus(w, r, installationdomain.StatusEnabled)
}
func (h *Handler) disablePluginInstallation(w http.ResponseWriter, r *http.Request) {
	h.setPluginStatus(w, r, installationdomain.StatusDisabled)
}
func (h *Handler) setPluginStatus(w http.ResponseWriter, r *http.Request, status installationdomain.Status) {
	id, err := platformplugin.ParseInstallationID(r.PathValue("installationId"))
	if err != nil {
		writeAPIError(w, http.StatusBadRequest, "validation_error", err)
		return
	}
	item, err := h.runtime.SetPluginInstallationStatus(r.Context(), id, status)
	if err != nil {
		writeKnownError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"installation": pluginInstallationProjection(item)})
}
func (h *Handler) deletePluginInstallation(w http.ResponseWriter, r *http.Request) {
	id, err := platformplugin.ParseInstallationID(r.PathValue("installationId"))
	if err != nil {
		writeAPIError(w, http.StatusBadRequest, "validation_error", err)
		return
	}
	if err := h.runtime.RemovePluginInstallation(r.Context(), id); err != nil {
		writeKnownError(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

package httpapi

import (
	"net/http"

	providerplugin "github.com/orz-i/Mender/backend/internal/providerplugin"
	installationdomain "github.com/orz-i/Mender/backend/internal/providerplugin/installation/domain"
)

func (h *Handler) listProviderPluginInstallations(
	w http.ResponseWriter,
	r *http.Request,
) {
	items, err := h.runtime.ListProviderPluginInstallations(r.Context())
	if err != nil {
		writeKnownError(w, err)
		return
	}
	projected := make([]map[string]any, 0, len(items))
	for _, item := range items {
		projected = append(projected, pluginInstallationProjection(item))
	}
	writeJSON(
		w,
		http.StatusOK,
		map[string]any{"installations": projected},
	)
}

func (h *Handler) enableProviderPluginInstallation(
	w http.ResponseWriter,
	r *http.Request,
) {
	h.setProviderPluginInstallationStatus(
		w,
		r,
		installationdomain.StatusEnabled,
	)
}

func (h *Handler) disableProviderPluginInstallation(
	w http.ResponseWriter,
	r *http.Request,
) {
	h.setProviderPluginInstallationStatus(
		w,
		r,
		installationdomain.StatusDisabled,
	)
}

func (h *Handler) setProviderPluginInstallationStatus(
	w http.ResponseWriter,
	r *http.Request,
	status installationdomain.Status,
) {
	id, err := providerplugin.ParseInstallationID(
		r.PathValue("installationId"),
	)
	if err != nil {
		writeAPIError(w, http.StatusBadRequest, "validation_error", err)
		return
	}
	item, err := h.runtime.SetProviderPluginInstallationStatus(
		r.Context(),
		id,
		status,
	)
	if err != nil {
		writeKnownError(w, err)
		return
	}
	writeJSON(
		w,
		http.StatusOK,
		map[string]any{"installation": pluginInstallationProjection(item)},
	)
}

func (h *Handler) deleteProviderPluginInstallation(
	w http.ResponseWriter,
	r *http.Request,
) {
	id, err := providerplugin.ParseInstallationID(
		r.PathValue("installationId"),
	)
	if err != nil {
		writeAPIError(w, http.StatusBadRequest, "validation_error", err)
		return
	}
	if err := h.runtime.RemoveProviderPluginInstallation(r.Context(), id); err != nil {
		writeKnownError(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func pluginInstallationProjection(
	item installationdomain.Installation,
) map[string]any {
	return map[string]any{
		"installationId": item.ID().String(),
		"pluginId":       item.PluginID().String(),
		"packageVersion": item.PackageVersion(),
		"sourceKind":     string(item.SourceKind()),
		"digest":         item.Digest(),
		"status":         string(item.Status()),
		"installedAt":    item.InstalledAt(),
	}
}

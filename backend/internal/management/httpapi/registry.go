package httpapi

import (
	"fmt"
	"net/http"
	"strings"

	platformplugin "github.com/orz-i/Mender/backend/internal/plugin"
)

type installRegistryPluginRequest struct {
	PluginID string `json:"pluginId"`
	Version  string `json:"version,omitempty"`
}

type upgradeRegistryPluginRequest struct {
	InstallationID string `json:"installationId"`
	Version        string `json:"version"`
}

func (h *Handler) syncRegistry(w http.ResponseWriter, r *http.Request) {
	synced, count, err := h.runtime.SyncRegistry(r.Context())
	if err != nil {
		writeKnownError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"synced": synced,
		"count":  count,
	})
}

func (h *Handler) installRegistryPlugin(w http.ResponseWriter, r *http.Request) {
	var req installRegistryPluginRequest
	if err := decodeJSON(r, &req); err != nil {
		writeAPIError(w, http.StatusBadRequest, "invalid_request", err)
		return
	}
	if strings.TrimSpace(req.PluginID) == "" {
		writeAPIError(w, http.StatusBadRequest, "validation_error", fmt.Errorf("pluginId is required"))
		return
	}

	item, err := h.runtime.InstallRegistryPlugin(r.Context(), strings.TrimSpace(req.PluginID), strings.TrimSpace(req.Version))
	if err != nil {
		writeKnownError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"installation": pluginInstallationProjection(item)})
}

func (h *Handler) upgradeRegistryPlugin(w http.ResponseWriter, r *http.Request) {
	var req upgradeRegistryPluginRequest
	if err := decodeJSON(r, &req); err != nil {
		writeAPIError(w, http.StatusBadRequest, "invalid_request", err)
		return
	}
	if strings.TrimSpace(req.InstallationID) == "" {
		writeAPIError(w, http.StatusBadRequest, "validation_error", fmt.Errorf("installationId is required"))
		return
	}
	if strings.TrimSpace(req.Version) == "" {
		writeAPIError(w, http.StatusBadRequest, "validation_error", fmt.Errorf("version is required"))
		return
	}

	id, err := platformplugin.ParseInstallationID(req.InstallationID)
	if err != nil {
		writeAPIError(w, http.StatusBadRequest, "validation_error", err)
		return
	}

	item, err := h.runtime.UpgradeRegistryPlugin(r.Context(), id, strings.TrimSpace(req.Version))
	if err != nil {
		writeKnownError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"installation": pluginInstallationProjection(item)})
}

func (h *Handler) listRegistryUpdates(w http.ResponseWriter, r *http.Request) {
	updates, err := h.runtime.ListRegistryUpdates(r.Context())
	if err != nil {
		writeKnownError(w, err)
		return
	}
	projected := make([]map[string]any, 0, len(updates))
	for _, u := range updates {
		projected = append(projected, map[string]any{
			"installationId": u.InstallationID,
			"pluginId":       u.PluginID,
			"currentVersion": u.CurrentVersion,
			"targetVersion":  u.TargetVersion,
			"canUpgrade":     u.CanUpgrade,
			"usageCount":     u.UsageCount,
		})
	}
	writeJSON(w, http.StatusOK, map[string]any{"updates": projected})
}

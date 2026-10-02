package httpapi

import (
	"net/http"

	"github.com/orz-i/Mender/backend/internal/bootstrap"
	profiledomain "github.com/orz-i/Mender/backend/internal/profile/domain"
)

type startMCPServerRequest struct {
	Profile string `json:"profile"`
	Address string `json:"address"`
}

func (h *Handler) listMCPServers(w http.ResponseWriter, _ *http.Request) {
	items := h.runtime.ListMCPServers()
	servers := make([]map[string]any, 0, len(items))
	for _, item := range items {
		servers = append(servers, mcpServerProjection(item))
	}
	writeJSON(w, http.StatusOK, map[string]any{"servers": servers})
}

func (h *Handler) startMCPServer(w http.ResponseWriter, r *http.Request) {
	var input startMCPServerRequest
	if err := decodeJSON(r, &input); err != nil {
		writeAPIError(w, http.StatusBadRequest, "validation_error", err)
		return
	}

	slug, err := profiledomain.NewSlug(input.Profile)
	if err != nil {
		writeAPIError(w, http.StatusBadRequest, "validation_error", err)
		return
	}

	instance, err := h.startMCPHTTPServer(
		r.Context(),
		slug,
		input.Address,
	)
	if err != nil {
		switch {
		case isKnownClientError(err):
			writeKnownError(w, err)
		default:
			writeAPIError(w, http.StatusConflict, "server_start_failed", err)
		}
		return
	}
	writeJSON(w, http.StatusCreated, mcpServerProjection(instance))
}

func (h *Handler) restartMCPServer(w http.ResponseWriter, r *http.Request) {
	instance, err := h.restartManagedMCPServer(
		r.Context(),
		r.PathValue("serverId"),
	)
	if err != nil {
		if isKnownClientError(err) {
			writeKnownError(w, err)
		} else {
			writeAPIError(w, http.StatusConflict, "server_restart_failed", err)
		}
		return
	}
	writeJSON(w, http.StatusOK, mcpServerProjection(instance))
}

func (h *Handler) stopMCPServer(w http.ResponseWriter, r *http.Request) {
	if err := h.runtime.StopMCPServer(
		r.Context(),
		r.PathValue("serverId"),
	); err != nil {
		writeKnownError(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func mcpServerProjection(instance bootstrap.MCPServerInstance) map[string]any {
	result := map[string]any{
		"serverId":  instance.ServerID,
		"profile":   instance.Profile,
		"transport": instance.Transport,
		"address":   instance.Address,
		"endpoint":  instance.Endpoint,
		"status":    instance.Status,
		"startedAt": instance.StartedAt,
	}
	if instance.TunnelID != "" {
		result["tunnelId"] = instance.TunnelID
		result["tunnelDisplayName"] = instance.TunnelDisplayName
		result["tunnelProvider"] = instance.TunnelProvider
		result["tunnelExternalId"] = instance.TunnelExternalID
		result["tunnelStatus"] = instance.TunnelStatus
	}
	return result
}

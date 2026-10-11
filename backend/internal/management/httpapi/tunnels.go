package httpapi

import (
	"net/http"

	credentialdomain "github.com/orz-i/Mender/backend/internal/credential/domain"
	profileapplication "github.com/orz-i/Mender/backend/internal/profile/application"
	profiledomain "github.com/orz-i/Mender/backend/internal/profile/domain"
	tunnelapplication "github.com/orz-i/Mender/backend/internal/tunnel/application"
	tunneldomain "github.com/orz-i/Mender/backend/internal/tunnel/domain"
)

type createOpenAITunnelRequest struct {
	DisplayName          string `json:"displayName"`
	ExternalTunnelID     string `json:"externalTunnelId"`
	RuntimeCredentialRef string `json:"runtimeCredentialRef"`
}

type setProfileTunnelRequest struct {
	TunnelID string `json:"tunnelId"`
}

func (h *Handler) listTunnels(w http.ResponseWriter, r *http.Request) {
	items, err := h.runtime.Tunnels.List.Execute(r.Context())
	if err != nil {
		writeKnownError(w, err)
		return
	}
	result := make([]map[string]any, 0, len(items))
	for _, item := range items {
		result = append(result, tunnelProjection(item))
	}
	writeJSON(w, http.StatusOK, map[string]any{"tunnels": result})
}

func (h *Handler) createOpenAITunnel(w http.ResponseWriter, r *http.Request) {
	var input createOpenAITunnelRequest
	if err := decodeJSON(r, &input); err != nil {
		writeAPIError(w, http.StatusBadRequest, "validation_error", err)
		return
	}
	ref, err := credentialdomain.ParseReference(input.RuntimeCredentialRef)
	if err != nil {
		writeAPIError(w, http.StatusBadRequest, "validation_error", err)
		return
	}
	item, err := h.runtime.Tunnels.Create.Execute(
		r.Context(),
		tunnelapplication.CreateOpenAITunnelRequest{
			DisplayName:          input.DisplayName,
			ExternalTunnelID:     input.ExternalTunnelID,
			RuntimeCredentialRef: ref,
		},
	)
	if err != nil {
		writeKnownError(w, err)
		return
	}
	writeJSON(w, http.StatusCreated, tunnelProjection(item))
}

func (h *Handler) deleteTunnel(w http.ResponseWriter, r *http.Request) {
	id, err := tunneldomain.ParseTunnelID(r.PathValue("tunnelId"))
	if err != nil {
		writeAPIError(w, http.StatusBadRequest, "validation_error", err)
		return
	}
	if err := h.runtime.Tunnels.Delete.Execute(r.Context(), id); err != nil {
		writeKnownError(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (h *Handler) setProfileTunnel(w http.ResponseWriter, r *http.Request) {
	slug, err := profiledomain.NewSlug(r.PathValue("slug"))
	if err != nil {
		writeAPIError(w, http.StatusBadRequest, "validation_error", err)
		return
	}
	var input setProfileTunnelRequest
	if err := decodeJSON(r, &input); err != nil {
		writeAPIError(w, http.StatusBadRequest, "validation_error", err)
		return
	}

	var tunnelID tunneldomain.TunnelID
	if input.TunnelID != "" {
		tunnelID, err = tunneldomain.ParseTunnelID(input.TunnelID)
		if err != nil {
			writeAPIError(w, http.StatusBadRequest, "validation_error", err)
			return
		}
	}
	profile, err := h.runtime.Profiles.SetTunnel.Execute(
		r.Context(),
		profileapplication.SetTunnelRequest{
			ProfileSlug: slug,
			TunnelID:    tunnelID,
		},
	)
	if err != nil {
		writeKnownError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, profileProjection(profile))
}

func tunnelProjection(item tunneldomain.Tunnel) map[string]any {
	return map[string]any{
		"tunnelId":             item.ID().String(),
		"displayName":          item.DisplayName(),
		"provider":             string(item.Provider()),
		"pluginId":             item.Binding().PluginID.String(),
		"extensionId":          item.Binding().ExtensionID.String(),
		"installationId":       item.Binding().InstallationID.String(),
		"externalTunnelId":     item.ExternalTunnelID(),
		"runtimeCredentialRef": item.RuntimeCredentialRef().String(),
	}
}

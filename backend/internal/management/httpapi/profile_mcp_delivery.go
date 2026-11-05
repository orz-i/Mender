package httpapi

import (
	"errors"
	"fmt"
	"net/http"

	"github.com/orz-i/Mender/backend/internal/bootstrap"
	catalog "github.com/orz-i/Mender/backend/internal/catalog/domain"
	mcpdelivery "github.com/orz-i/Mender/backend/internal/mcp/application/delivery"
	mcp "github.com/orz-i/Mender/backend/internal/mcp/domain"
	profile "github.com/orz-i/Mender/backend/internal/profile/domain"
	tunnel "github.com/orz-i/Mender/backend/internal/tunnel/domain"
)

func (h *Handler) getProfileMCPDelivery(w http.ResponseWriter, r *http.Request) {
	slug, err := profile.NewSlug(r.PathValue("slug"))
	if err != nil {
		writeAPIError(w, http.StatusBadRequest, "validation_error", err)
		return
	}
	snapshot, err := h.runtime.GetProfileMCPDelivery(r.Context(), slug)
	if err != nil {
		writeProfileMCPDeliveryError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, profileMCPDeliveryProjection(snapshot))
}

type configureProfileMCPDeliveryRequest struct {
	Mode                    mcp.Mode  `json:"mode"`
	TunnelID                string    `json:"tunnelId"`
	DirectToolIDs           *[]string `json:"directToolIds"`
	ExpectedProfileRevision *uint64   `json:"expectedProfileRevision"`
	ExpectedRevision        *uint64   `json:"expectedRevision"`
}

func (h *Handler) configureProfileMCPDelivery(w http.ResponseWriter, r *http.Request) {
	slug, err := profile.NewSlug(r.PathValue("slug"))
	if err != nil {
		writeAPIError(w, http.StatusBadRequest, "validation_error", err)
		return
	}
	var input configureProfileMCPDeliveryRequest
	if err := decodeJSON(r, &input); err != nil {
		writeAPIError(w, http.StatusBadRequest, "validation_error", err)
		return
	}
	if input.ExpectedProfileRevision == nil || input.DirectToolIDs == nil {
		writeAPIError(w, http.StatusBadRequest, "validation_error", fmt.Errorf("expectedProfileRevision and directToolIds are required"))
		return
	}
	var tunnelID tunnel.TunnelID
	if input.TunnelID != "" {
		tunnelID, err = tunnel.ParseTunnelID(input.TunnelID)
		if err != nil {
			writeAPIError(w, http.StatusBadRequest, "validation_error", err)
			return
		}
	}
	ids := make([]catalog.ToolID, 0, len(*input.DirectToolIDs))
	for _, raw := range *input.DirectToolIDs {
		id, err := catalog.ParseToolID(raw)
		if err != nil {
			writeAPIError(w, http.StatusBadRequest, "validation_error", err)
			return
		}
		ids = append(ids, id)
	}
	snapshot, err := h.runtime.ConfigureProfileMCPDelivery(r.Context(), slug, mcpdelivery.ConfigureRequest{
		Mode: input.Mode, TunnelID: tunnelID, DirectToolIDs: ids, ExpectedProfileRevision: *input.ExpectedProfileRevision, ExpectedRevision: input.ExpectedRevision,
	})
	if err != nil {
		writeProfileMCPDeliveryError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, profileMCPDeliveryProjection(snapshot))
}

func profileMCPDeliveryProjection(snapshot bootstrap.ProfileMCPDelivery) map[string]any {
	ids := make([]string, 0, len(snapshot.Config.DirectToolIDs()))
	for _, id := range snapshot.Config.DirectToolIDs() {
		ids = append(ids, id.String())
	}
	result := map[string]any{"profileId": snapshot.Profile.ID().String(), "profileRevision": snapshot.Profile.Revision(), "revision": snapshot.Config.Revision(), "mode": string(snapshot.Config.Mode()), "directToolIds": ids}
	if id, ok := snapshot.Config.TunnelID(); ok {
		result["tunnelId"] = id.String()
	}
	return result
}

func writeProfileMCPDeliveryError(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, mcpdelivery.ErrNotFound):
		writeAPIError(w, http.StatusNotFound, "mcp_delivery_not_configured", err)
	case errors.Is(err, mcpdelivery.ErrRevisionConflict), errors.Is(err, mcpdelivery.ErrProfileRevisionConflict):
		writeAPIError(w, http.StatusConflict, "revision_conflict", err)
	case errors.Is(err, mcpdelivery.ErrInvalidConfiguration), errors.Is(err, mcpdelivery.ErrToolOutsideProfile):
		writeAPIError(w, http.StatusBadRequest, "validation_error", err)
	default:
		writeKnownError(w, err)
	}
}

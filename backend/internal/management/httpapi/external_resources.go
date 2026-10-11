package httpapi

import (
	"encoding/json"
	"errors"
	"net/http"

	"github.com/orz-i/Mender/backend/internal/bootstrap"
	credentialdomain "github.com/orz-i/Mender/backend/internal/credential/domain"
	governancedomain "github.com/orz-i/Mender/backend/internal/governance/domain"
	platformplugin "github.com/orz-i/Mender/backend/internal/plugin"
	sandboxdomain "github.com/orz-i/Mender/backend/internal/sandbox/domain"
	sandboxsqlite "github.com/orz-i/Mender/backend/internal/sandbox/infrastructure/sqlite"
	tunneldomain "github.com/orz-i/Mender/backend/internal/tunnel/domain"
)

type createExternalSandboxRequest struct {
	Name           string          `json:"name"`
	InstallationID string          `json:"installationId"`
	Config         json.RawMessage `json:"config"`
}

func (h *Handler) createExternalSandbox(w http.ResponseWriter, r *http.Request) {
	var input createExternalSandboxRequest
	if err := decodeJSON(r, &input); err != nil {
		writeAPIError(w, http.StatusBadRequest, "validation_error", err)
		return
	}
	id, err := platformplugin.ParseInstallationID(input.InstallationID)
	if err != nil {
		writeAPIError(w, http.StatusBadRequest, "validation_error", err)
		return
	}
	caller, err := governancedomain.NewContext(governancedomain.OriginManagementHTTP, h.runtime.OwnerPrincipal(), governancedomain.CatalogScope())
	if err != nil {
		writeKnownError(w, err)
		return
	}
	item, err := h.runtime.GetPluginInstallation(r.Context(), id)
	if err != nil {
		writeKnownError(w, err)
		return
	}
	descriptor, verified := item.VerifiedDescriptor()
	if !verified {
		writeAPIError(w, http.StatusConflict, "unverified_plugin", sandboxdomain.ErrBackendUnavailable)
		return
	}
	created, createErr := h.runtime.Sandboxes.CreateExternalSandbox(r.Context(), bootstrap.CreateExternalSandboxRequest{
		Name: input.Name, InstallationID: id, Config: sandboxsqlite.ExternalConfig{
			ConfigVersion: descriptor.ConfigVersion, JSON: input.Config,
		},
	}, caller)
	if createErr != nil && created.ID() == "" {
		if errors.Is(createErr, sandboxdomain.ErrNotAuthorized) {
			writeAPIError(w, http.StatusForbidden, "policy_denied", createErr)
			return
		}
		if errors.Is(createErr, sandboxdomain.ErrConflict) {
			writeAPIError(w, http.StatusConflict, "conflict", createErr)
			return
		}
		writeAPIError(w, http.StatusBadRequest, "validation_error", createErr)
		return
	}
	state, err := h.sandboxStatus(r, created)
	if err != nil {
		writeKnownError(w, err)
		return
	}
	status := http.StatusCreated
	if createErr != nil {
		status = http.StatusAccepted
	}
	writeJSON(w, status, state)
}

type createExternalTunnelRequest struct {
	DisplayName          string `json:"displayName"`
	InstallationID       string `json:"installationId"`
	ExternalTunnelID     string `json:"externalTunnelId"`
	RuntimeCredentialRef string `json:"runtimeCredentialRef"`
}

func (h *Handler) createExternalTunnel(w http.ResponseWriter, r *http.Request) {
	var input createExternalTunnelRequest
	if err := decodeJSON(r, &input); err != nil {
		writeAPIError(w, http.StatusBadRequest, "validation_error", err)
		return
	}
	if err := tunneldomain.ValidateNameAndExternalID(input.DisplayName, input.ExternalTunnelID); err != nil {
		writeAPIError(w, http.StatusBadRequest, "validation_error", err)
		return
	}
	id, err := platformplugin.ParseInstallationID(input.InstallationID)
	if err != nil {
		writeAPIError(w, http.StatusBadRequest, "validation_error", err)
		return
	}
	ref, err := credentialdomain.ParseReference(input.RuntimeCredentialRef)
	if err != nil {
		writeAPIError(w, http.StatusBadRequest, "validation_error", err)
		return
	}
	item, err := h.runtime.Tunnels.CreateExternal(r.Context(), bootstrap.CreateExternalTunnelRequest{
		DisplayName: input.DisplayName, InstallationID: id,
		ExternalTunnelID: input.ExternalTunnelID, RuntimeCredentialRef: ref,
	})
	if err != nil {
		writeKnownError(w, err)
		return
	}
	writeJSON(w, http.StatusCreated, tunnelProjection(item))
}

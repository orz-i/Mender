package httpapi

import (
	"errors"
	"net/http"

	"github.com/orz-i/Mender/backend/internal/bootstrap"
	credentialdomain "github.com/orz-i/Mender/backend/internal/credential/domain"
	governancedomain "github.com/orz-i/Mender/backend/internal/governance/domain"
	sandboxdomain "github.com/orz-i/Mender/backend/internal/sandbox/domain"
	sandboxssh "github.com/orz-i/Mender/backend/internal/sandbox/infrastructure/sshremote"
)

type createSSHRemoteSandboxRequest struct {
	Name          string `json:"name"`
	Host          string `json:"host"`
	Port          uint16 `json:"port"`
	User          string `json:"user"`
	Root          string `json:"root"`
	HostPublicKey string `json:"hostPublicKey"`
	CredentialRef string `json:"credentialRef"`
}

func (h *Handler) createSSHRemoteSandbox(w http.ResponseWriter, r *http.Request) {
	var input createSSHRemoteSandboxRequest
	if err := decodeJSON(r, &input); err != nil {
		writeAPIError(w, http.StatusBadRequest, "validation_error", err)
		return
	}
	ref, err := credentialdomain.ParseReference(input.CredentialRef)
	if err != nil {
		writeAPIError(w, http.StatusBadRequest, "validation_error", err)
		return
	}
	caller, err := governancedomain.NewContext(governancedomain.OriginManagementHTTP, h.runtime.OwnerPrincipal(), governancedomain.CatalogScope())
	if err != nil {
		writeKnownError(w, err)
		return
	}
	t := sandboxssh.Template{Host: input.Host, Port: input.Port, User: input.User, Root: input.Root, HostPublicKey: input.HostPublicKey, CredentialRef: ref}
	item, err := h.runtime.Sandboxes.CreateSSHRemoteSandbox(r.Context(), bootstrap.CreateSSHRemoteSandboxRequest{Name: input.Name, Template: t}, caller)
	if err != nil {
		switch {
		case errors.Is(err, sandboxdomain.ErrNotAuthorized):
			writeAPIError(w, http.StatusForbidden, "policy_denied", err)
		case errors.Is(err, sandboxdomain.ErrBackendUnavailable):
			if code, ok := sandboxssh.FailureCodeOf(err); ok {
				writeAPIError(w, http.StatusServiceUnavailable, "ssh_remote_"+string(code), err)
			} else {
				writeAPIError(w, http.StatusServiceUnavailable, "ssh_remote_unavailable", err)
			}
		case errors.Is(err, sandboxdomain.ErrConflict):
			writeAPIError(w, http.StatusConflict, "conflict", err)
		default:
			writeAPIError(w, http.StatusBadRequest, "validation_error", err)
		}
		return
	}
	projected, err := h.sandboxStatus(r, item)
	if err != nil {
		writeKnownError(w, err)
		return
	}
	writeJSON(w, http.StatusCreated, projected)
}

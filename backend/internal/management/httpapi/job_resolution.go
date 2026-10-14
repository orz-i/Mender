package httpapi

import (
	"errors"
	"net/http"

	"github.com/orz-i/Mender/backend/internal/bootstrap"
	sandboxdomain "github.com/orz-i/Mender/backend/internal/sandbox/domain"
	sshremote "github.com/orz-i/Mender/backend/internal/sandbox/infrastructure/sshremote"
)

// Explicit owner-only mutation. This API cannot be mounted under Profile MCP
// routes and cannot accept a command, host path, Credential or arbitrary
// launch script from the request body.
func (h *Handler) resolveSSHJobRecovery(w http.ResponseWriter, r *http.Request) {
	id, caller, err := h.sandboxToolsRequest(r)
	if err != nil {
		writeAPIError(w, http.StatusBadRequest, "validation_error", err)
		return
	}
	taskID := r.PathValue("taskId")
	var input struct {
		Fingerprint string `json:"fingerprint"`
		Generation  uint64 `json:"generation"`
		Reason      string `json:"reason"`
	}
	if err = decodeJSON(r, &input); err != nil {
		writeAPIError(w, http.StatusBadRequest, "validation_error", err)
		return
	}
	// Validate syntax BEFORE Core Policy and SSH. Detailed resource authorization
	// remains centralized in bootstrap, after exact typed input validation.
	if err = (sshremote.V2ResolveRequest{TaskID: taskID, Fingerprint: input.Fingerprint,
		Generation: input.Generation, Reason: input.Reason, Actor: "local:owner"}).Validate(); err != nil {
		writeAPIError(w, http.StatusBadRequest, "validation_error", err)
		return
	}
	result, err := h.runtime.Sandboxes.ResolveSSHJobRecovery(r.Context(), id, bootstrap.SSHJobResolutionRequest{
		TaskID: taskID, Fingerprint: input.Fingerprint, Generation: input.Generation, Reason: input.Reason,
	}, caller)
	if err != nil {
		switch {
		case errors.Is(err, sandboxdomain.ErrNotAuthorized):
			writeAPIError(w, http.StatusForbidden, "policy_denied", err)
		case errors.Is(err, sandboxdomain.ErrNotFound):
			writeAPIError(w, http.StatusNotFound, "not_found", err)
		case errors.Is(err, sandboxdomain.ErrConflict), errors.Is(err, sshremote.ErrJobConflict):
			writeAPIError(w, http.StatusConflict, "recovery_conflict", err)
		case errors.Is(err, sshremote.ErrJobUnavailable), errors.Is(err, sshremote.ErrJobUnknown):
			writeAPIError(w, http.StatusServiceUnavailable, "recovery_unproven", err)
		default:
			writeKnownError(w, err)
		}
		return
	}
	writeJSON(w, http.StatusOK, result)
}

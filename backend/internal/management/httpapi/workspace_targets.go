package httpapi

import (
	"errors"
	"net/http"

	sandboxdomain "github.com/orz-i/Mender/backend/internal/sandbox/domain"
)

// Owner-only management for additional Workspace Tool targets.
// The service's default Sandbox stays in its stable Provider binding.
func (h *Handler) workspaceTargetError(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, sandboxdomain.ErrNotFound):
		writeAPIError(w, http.StatusNotFound, "not_found", err)
	case errors.Is(err, sandboxdomain.ErrConflict):
		writeAPIError(w, http.StatusConflict, "conflict", err)
	case errors.Is(err, sandboxdomain.ErrCapabilityUnavailable):
		writeAPIError(w, http.StatusConflict, "capability_unavailable", err)
	default:
		writeKnownError(w, err)
	}
}
func (h *Handler) listWorkspaceTargets(w http.ResponseWriter, r *http.Request) {
	caller, err := h.agentOwnerContext()
	if err != nil {
		h.workspaceTargetError(w, err)
		return
	}
	items, err := h.runtime.ListWorkspaceToolTargets(r.Context(), r.PathValue("providerId"), caller)
	if err != nil {
		h.workspaceTargetError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"workspaces": items})
}
func (h *Handler) bindWorkspaceTarget(w http.ResponseWriter, r *http.Request) {
	var body struct {
		SandboxID string `json:"sandboxId"`
	}
	if err := decodeJSON(r, &body); err != nil {
		writeAPIError(w, http.StatusBadRequest, "validation_error", err)
		return
	}
	caller, err := h.agentOwnerContext()
	if err != nil {
		h.workspaceTargetError(w, err)
		return
	}
	if err := h.runtime.BindWorkspaceToolTarget(r.Context(), r.PathValue("providerId"), r.PathValue("workspaceKey"), body.SandboxID, caller); err != nil {
		h.workspaceTargetError(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}
func (h *Handler) unbindWorkspaceTarget(w http.ResponseWriter, r *http.Request) {
	caller, err := h.agentOwnerContext()
	if err != nil {
		h.workspaceTargetError(w, err)
		return
	}
	if err := h.runtime.UnbindWorkspaceToolTarget(r.Context(), r.PathValue("providerId"), r.PathValue("workspaceKey"), caller); err != nil {
		h.workspaceTargetError(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

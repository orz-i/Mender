package httpapi

import (
	"net/http"

	agentdomain "github.com/orz-i/Mender/backend/internal/agent/domain"
)

func (h *Handler) updateAgentSettings(w http.ResponseWriter, r *http.Request) {
	id, err := agentdomain.ParseID(r.PathValue("agentId"))
	if err != nil {
		h.agentError(w, err)
		return
	}
	var payload struct {
		DefaultModel string `json:"defaultModel"`
	}
	if err := decodeJSON(r, &payload); err != nil {
		writeAPIError(w, http.StatusBadRequest, "validation_error", err)
		return
	}
	caller, err := h.agentOwnerContext()
	if err != nil {
		h.agentError(w, err)
		return
	}
	agent, err := h.runtime.Agents.UpdateDefaultModel(r.Context(), id, payload.DefaultModel, caller)
	if err != nil {
		h.agentError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, h.agentProjection(agent))
}
func (h *Handler) listAgentWorkspaces(w http.ResponseWriter, r *http.Request) {
	id, err := agentdomain.ParseID(r.PathValue("agentId"))
	if err != nil {
		h.agentError(w, err)
		return
	}
	caller, err := h.agentOwnerContext()
	if err != nil {
		h.agentError(w, err)
		return
	}
	items, err := h.runtime.Agents.Workspaces(r.Context(), id, caller)
	if err != nil {
		h.agentError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"workspaces": items})
}
func (h *Handler) bindAgentWorkspace(w http.ResponseWriter, r *http.Request) {
	id, err := agentdomain.ParseID(r.PathValue("agentId"))
	if err != nil {
		h.agentError(w, err)
		return
	}
	var payload struct {
		SandboxID string `json:"sandboxId"`
	}
	if err := decodeJSON(r, &payload); err != nil {
		writeAPIError(w, http.StatusBadRequest, "validation_error", err)
		return
	}
	caller, err := h.agentOwnerContext()
	if err != nil {
		h.agentError(w, err)
		return
	}
	if err := h.runtime.Agents.BindWorkspace(r.Context(), id, r.PathValue("workspaceKey"), payload.SandboxID, caller); err != nil {
		h.agentError(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}
func (h *Handler) unbindAgentWorkspace(w http.ResponseWriter, r *http.Request) {
	id, err := agentdomain.ParseID(r.PathValue("agentId"))
	if err != nil {
		h.agentError(w, err)
		return
	}
	caller, err := h.agentOwnerContext()
	if err != nil {
		h.agentError(w, err)
		return
	}
	if err := h.runtime.Agents.UnbindWorkspace(r.Context(), id, r.PathValue("workspaceKey"), caller); err != nil {
		h.agentError(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}
func (h *Handler) grantAgentWorkspace(w http.ResponseWriter, r *http.Request) {
	id, err := agentdomain.ParseID(r.PathValue("agentId"))
	if err != nil {
		h.agentError(w, err)
		return
	}
	caller, err := h.agentOwnerContext()
	if err != nil {
		h.agentError(w, err)
		return
	}
	if err := h.runtime.Agents.GrantWorkspace(r.Context(), id, r.PathValue("workspaceKey"), r.PathValue("profileId"), caller); err != nil {
		h.agentError(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}
func (h *Handler) revokeAgentWorkspace(w http.ResponseWriter, r *http.Request) {
	id, err := agentdomain.ParseID(r.PathValue("agentId"))
	if err != nil {
		h.agentError(w, err)
		return
	}
	caller, err := h.agentOwnerContext()
	if err != nil {
		h.agentError(w, err)
		return
	}
	if err := h.runtime.Agents.RevokeWorkspace(r.Context(), id, r.PathValue("workspaceKey"), r.PathValue("profileId"), caller); err != nil {
		h.agentError(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

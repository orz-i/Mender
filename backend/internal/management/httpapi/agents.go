package httpapi

import (
	"errors"
	"net/http"

	agentapplication "github.com/orz-i/Mender/backend/internal/agent/application"
	agentdomain "github.com/orz-i/Mender/backend/internal/agent/domain"
	gov "github.com/orz-i/Mender/backend/internal/governance/domain"
)

func (h *Handler) agentOwnerContext() (gov.Context, error) {
	return gov.NewContext(gov.OriginManagementHTTP, h.runtime.OwnerPrincipal(), gov.CatalogScope())
}
func (h *Handler) agentError(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, agentdomain.ErrForbidden):
		writeAPIError(w, http.StatusForbidden, "policy_denied", err)
	case errors.Is(err, agentdomain.ErrNotFound):
		writeAPIError(w, http.StatusNotFound, "not_found", err)
	case errors.Is(err, agentdomain.ErrConflict):
		writeAPIError(w, http.StatusConflict, "conflict", err)
	case errors.Is(err, agentdomain.ErrInvalid):
		writeAPIError(w, http.StatusBadRequest, "validation_error", err)
	case errors.Is(err, agentdomain.ErrUnavailable):
		writeAPIError(w, http.StatusServiceUnavailable, "unavailable", err)
	default:
		writeKnownError(w, err)
	}
}
func (h *Handler) agentProjection(a agentdomain.Agent) map[string]any {
	s := a.Spec()
	out := map[string]any{"agentId": s.ID, "name": s.Name, "backend": s.Backend, "pluginId": s.PluginID,
		"extensionId": s.ExtensionID, "workspaceRoot": s.WorkspaceRoot, "defaultModel": s.DefaultModel, "isolation": s.Isolation, "createdAt": s.CreatedAt,
		"backendAvailable": false}
	if _, err := h.runtime.AgentAvailability(s.Backend); err != nil {
		out["issue"] = err.Error()
	} else {
		out["backendAvailable"] = true
	}
	return out
}
func (h *Handler) listAgents(w http.ResponseWriter, r *http.Request) {
	caller, err := h.agentOwnerContext()
	if err != nil {
		h.agentError(w, err)
		return
	}
	agents, err := h.runtime.Agents.List(r.Context(), caller)
	if err != nil {
		h.agentError(w, err)
		return
	}
	items := make([]map[string]any, 0, len(agents))
	for _, a := range agents {
		items = append(items, h.agentProjection(a))
	}
	writeJSON(w, http.StatusOK, map[string]any{"agents": items})
}
func (h *Handler) createAgent(w http.ResponseWriter, r *http.Request) {
	var req agentapplication.CreateRequest
	if err := decodeJSON(r, &req); err != nil {
		writeAPIError(w, http.StatusBadRequest, "validation_error", err)
		return
	}
	caller, err := h.agentOwnerContext()
	if err != nil {
		h.agentError(w, err)
		return
	}
	a, err := h.runtime.CreateAgentWithTools(r.Context(), req, caller)
	if err != nil {
		h.agentError(w, err)
		return
	}
	writeJSON(w, http.StatusCreated, h.agentProjection(a))
}
func (h *Handler) getAgent(w http.ResponseWriter, r *http.Request) {
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
	a, err := h.runtime.Agents.Get(r.Context(), id, caller)
	if err != nil {
		h.agentError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, h.agentProjection(a))
}
func (h *Handler) deleteAgent(w http.ResponseWriter, r *http.Request) {
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
	if err := h.runtime.DeleteAgentWithTools(r.Context(), id, caller); err != nil {
		h.agentError(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// Tool suite inspection is inert. Recovery is an explicit owner-only POST.
func (h *Handler) getAgentTools(w http.ResponseWriter, r *http.Request) {
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
	tools, err := h.runtime.AgentTools(r.Context(), id, caller)
	if err != nil {
		h.agentError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, tools)
}
func (h *Handler) ensureAgentTools(w http.ResponseWriter, r *http.Request) {
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
	tools, err := h.runtime.EnsureAgentTools(r.Context(), id, caller)
	if err != nil {
		h.agentError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, tools)
}
func (h *Handler) listAgentTasks(w http.ResponseWriter, r *http.Request) {
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
	items, err := h.runtime.Agents.Tasks(r.Context(), id, caller)
	if err != nil {
		h.agentError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"tasks": items})
}
func (h *Handler) getAgentTask(w http.ResponseWriter, r *http.Request) {
	id, err := agentdomain.ParseTaskID(r.PathValue("taskId"))
	if err != nil {
		h.agentError(w, err)
		return
	}
	caller, err := h.agentOwnerContext()
	if err != nil {
		h.agentError(w, err)
		return
	}
	task, err := h.runtime.Agents.Task(r.Context(), id, caller)
	if err != nil {
		h.agentError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, task)
}
func (h *Handler) cancelAgentTask(w http.ResponseWriter, r *http.Request) {
	id, err := agentdomain.ParseTaskID(r.PathValue("taskId"))
	if err != nil {
		h.agentError(w, err)
		return
	}
	caller, err := h.agentOwnerContext()
	if err != nil {
		h.agentError(w, err)
		return
	}
	if err := h.runtime.Agents.Cancel(r.Context(), id, caller); err != nil {
		h.agentError(w, err)
		return
	}
	writeJSON(w, http.StatusAccepted, map[string]any{"taskId": id, "cancelRequested": true})
}
func (h *Handler) listAgentAdmissions(w http.ResponseWriter, r *http.Request) {
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
	events, err := h.runtime.Agents.Admissions(r.Context(), id, caller, 100)
	if err != nil {
		h.agentError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"events": events})
}

func (h *Handler) listAgentGrants(w http.ResponseWriter, r *http.Request) {
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
	items, err := h.runtime.Agents.Grants(r.Context(), id, caller)
	if err != nil {
		h.agentError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"profileIds": items})
}
func (h *Handler) grantAgentProfile(w http.ResponseWriter, r *http.Request) {
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
	if err := h.runtime.Agents.Grant(r.Context(), id, r.PathValue("profileId"), caller); err != nil {
		h.agentError(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}
func (h *Handler) revokeAgentProfile(w http.ResponseWriter, r *http.Request) {
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
	if err := h.runtime.Agents.Revoke(r.Context(), id, r.PathValue("profileId"), caller); err != nil {
		h.agentError(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

package httpapi

import (
	"net/http"

	agentdomain "github.com/orz-i/Mender/backend/internal/agent/domain"
)

// Owner-only manual disposition. This is NOT an Agent outcome transition,
// does not execute a Tool, and does not rerun any uncertain Task.
func (h *Handler) reviewUnknownAgentTask(w http.ResponseWriter, r *http.Request) {
	id, err := agentdomain.ParseTaskID(r.PathValue("taskId"))
	if err != nil {
		h.agentError(w, err)
		return
	}
	var req struct {
		Handling string `json:"handling"`
	}
	if err := decodeJSON(r, &req); err != nil {
		writeAPIError(w, http.StatusBadRequest, "validation_error", err)
		return
	}
	caller, err := h.agentOwnerContext()
	if err != nil {
		h.agentError(w, err)
		return
	}
	task, err := h.runtime.Agents.ReviewUnknownTask(r.Context(), id, req.Handling, caller)
	if err != nil {
		h.agentError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, task)
}

// Owner-only explicit new instruction to a proven-complete, owner-origin
// native AgentSession. A Profile-origin Task must instead use its exact
// authorized, explicitly bound agent.resume Tool in that Profile.
func (h *Handler) resumeAgentTask(w http.ResponseWriter, r *http.Request) {
	id, err := agentdomain.ParseTaskID(r.PathValue("taskId"))
	if err != nil {
		h.agentError(w, err)
		return
	}
	var req struct {
		Prompt string `json:"prompt"`
	}
	if err := decodeJSON(r, &req); err != nil {
		writeAPIError(w, http.StatusBadRequest, "validation_error", err)
		return
	}
	caller, err := h.agentOwnerContext()
	if err != nil {
		h.agentError(w, err)
		return
	}
	source, err := h.runtime.Agents.Task(r.Context(), id, caller)
	if err != nil {
		h.agentError(w, err)
		return
	}
	child, err := h.runtime.Agents.Resume(r.Context(), id, req.Prompt, source.AgentID, caller)
	if err != nil {
		h.agentError(w, err)
		return
	}
	writeJSON(w, http.StatusAccepted, child)
}

package httpapi

import (
	"net/http"

	profiledomain "github.com/orz-i/Mender/backend/internal/profile/domain"
	skilldomain "github.com/orz-i/Mender/backend/internal/skill/domain"
)

// Remote management may inspect exact Profile-owned Skill deliveries, but
// arbitrary host filesystem materialization remains Desktop/CLI owner-only.
func (h *Handler) undeployProfileSkill(w http.ResponseWriter, r *http.Request) {
	slug, err := profiledomain.NewSlug(r.PathValue("slug"))
	if err != nil {
		writeAPIError(w, http.StatusBadRequest, "validation_error", err)
		return
	}
	id, err := skilldomain.ParseDeploymentID(r.PathValue("deploymentId"))
	if err != nil {
		writeAPIError(w, http.StatusBadRequest, "validation_error", err)
		return
	}
	if err := h.runtime.UndeployProfileSkill(r.Context(), slug, id); err != nil {
		writeKnownError(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (h *Handler) listProfileSkillDeployments(w http.ResponseWriter, r *http.Request) {
	slug, err := profiledomain.NewSlug(r.PathValue("slug"))
	if err != nil {
		writeAPIError(w, http.StatusBadRequest, "validation_error", err)
		return
	}
	items, err := h.runtime.ListProfileSkillDeployments(r.Context(), slug)
	if err != nil {
		writeKnownError(w, err)
		return
	}
	result := make([]map[string]any, 0, len(items))
	for _, item := range items {
		result = append(result, skillDeploymentProjection(item))
	}
	writeJSON(w, http.StatusOK, map[string]any{"deployments": result})
}

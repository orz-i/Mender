package httpapi

import (
	"net/http"

	"github.com/orz-i/Mender/backend/internal/bootstrap"
	skillapplication "github.com/orz-i/Mender/backend/internal/skill/application"
	skilldomain "github.com/orz-i/Mender/backend/internal/skill/domain"
)

func (h *Handler) listSkills(w http.ResponseWriter, r *http.Request) {
	items, err := h.runtime.Skills.List(r.Context())
	if err != nil {
		writeKnownError(w, err)
		return
	}
	projected := make([]map[string]any, 0, len(items))
	for _, item := range items {
		projected = append(projected, skillInstallationProjection(item))
	}
	writeJSON(w, http.StatusOK, map[string]any{"installations": projected})
}

func (h *Handler) getSkillInstallation(w http.ResponseWriter, r *http.Request) {
	id, err := skilldomain.ParseInstallationID(r.PathValue("installationId"))
	if err != nil {
		writeAPIError(w, http.StatusBadRequest, "validation_error", err)
		return
	}
	item, err := h.runtime.Skills.Inspect(r.Context(), id)
	if err != nil {
		writeKnownError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, skillInspectionProjection(item))
}

func (h *Handler) trustSkillInstallation(w http.ResponseWriter, r *http.Request) {
	h.setSkillInstallationTrust(w, r, true)
}

func (h *Handler) untrustSkillInstallation(w http.ResponseWriter, r *http.Request) {
	h.setSkillInstallationTrust(w, r, false)
}

func (h *Handler) setSkillInstallationTrust(
	w http.ResponseWriter,
	r *http.Request,
	trusted bool,
) {
	id, err := skilldomain.ParseInstallationID(r.PathValue("installationId"))
	if err != nil {
		writeAPIError(w, http.StatusBadRequest, "validation_error", err)
		return
	}
	item, err := h.runtime.Skills.SetTrust(r.Context(), id, trusted)
	if err != nil {
		writeKnownError(w, err)
		return
	}
	writeJSON(
		w,
		http.StatusOK,
		map[string]any{"installation": managedSkillInstallationProjection(item)},
	)
}

func (h *Handler) enableSkillInstallation(w http.ResponseWriter, r *http.Request) {
	h.setSkillInstallationEnabled(w, r, true)
}

func (h *Handler) disableSkillInstallation(w http.ResponseWriter, r *http.Request) {
	h.setSkillInstallationEnabled(w, r, false)
}

func (h *Handler) setSkillInstallationEnabled(
	w http.ResponseWriter,
	r *http.Request,
	enabled bool,
) {
	id, err := skilldomain.ParseInstallationID(r.PathValue("installationId"))
	if err != nil {
		writeAPIError(w, http.StatusBadRequest, "validation_error", err)
		return
	}
	item, err := h.runtime.Skills.SetEnabled(r.Context(), id, enabled)
	if err != nil {
		writeKnownError(w, err)
		return
	}
	writeJSON(
		w,
		http.StatusOK,
		map[string]any{"installation": managedSkillInstallationProjection(item)},
	)
}

func (h *Handler) deleteSkillInstallation(w http.ResponseWriter, r *http.Request) {
	id, err := skilldomain.ParseInstallationID(r.PathValue("installationId"))
	if err != nil {
		writeAPIError(w, http.StatusBadRequest, "validation_error", err)
		return
	}
	if err := h.runtime.Skills.Remove(r.Context(), id); err != nil {
		writeKnownError(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (h *Handler) listSkillDeployments(w http.ResponseWriter, r *http.Request) {
	items, err := h.runtime.Skills.ListDeployments(r.Context())
	if err != nil {
		writeKnownError(w, err)
		return
	}
	projected := make([]map[string]any, 0, len(items))
	for _, item := range items {
		projected = append(projected, skillDeploymentProjection(item))
	}
	writeJSON(w, http.StatusOK, map[string]any{"deployments": projected})
}

func (h *Handler) undeploySkill(w http.ResponseWriter, r *http.Request) {
	id, err := skilldomain.ParseDeploymentID(r.PathValue("deploymentId"))
	if err != nil {
		writeAPIError(w, http.StatusBadRequest, "validation_error", err)
		return
	}
	if err := h.runtime.Skills.Undeploy(r.Context(), id); err != nil {
		writeKnownError(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func skillInstallationProjection(item bootstrap.SkillInstallation) map[string]any {
	result := map[string]any{
		"skillId":         item.SkillID.String(),
		"installationId":  item.InstallationID.String(),
		"name":            item.Name,
		"description":     item.Description,
		"sourceKind":      item.SourceKind,
		"sourceReference": item.SourceReference,
		"digest":          item.Digest,
		"versionHint":     item.VersionHint,
		"trusted":         item.Trusted,
		"enabled":         item.Enabled,
		"builtIn":         item.BuiltIn,
	}
	if item.InstalledAt != nil {
		result["installedAt"] = *item.InstalledAt
	}
	return result
}

func managedSkillInstallationProjection(
	item skilldomain.Installation,
) map[string]any {
	return map[string]any{
		"skillId":         item.SkillID().String(),
		"installationId":  item.ID().String(),
		"name":            item.Name(),
		"description":     item.Description(),
		"sourceKind":      string(item.SourceKind()),
		"sourceReference": item.SourceReference(),
		"digest":          item.Digest(),
		"versionHint":     item.VersionHint(),
		"trusted":         item.Trusted(),
		"enabled":         item.Enabled(),
		"builtIn":         false,
		"installedAt":     item.InstalledAt(),
	}
}

func skillInspectionProjection(item bootstrap.SkillInspection) map[string]any {
	resources := make([]map[string]any, 0, len(item.Resources))
	for _, resource := range item.Resources {
		resources = append(resources, map[string]any{
			"path":       resource.Path,
			"kind":       string(resource.Kind),
			"size":       resource.Size,
			"sha256":     resource.SHA256,
			"executable": resource.Executable,
		})
	}
	return map[string]any{
		"installation": skillInstallationProjection(item.Installation),
		"metadata": map[string]any{
			"name":          item.Metadata.Name,
			"description":   item.Metadata.Description,
			"license":       item.Metadata.License,
			"compatibility": item.Metadata.Compatibility,
			"metadata":      item.Metadata.Values,
			"allowedTools":  item.Metadata.AllowedTools,
		},
		"resources":  resources,
		"totalBytes": item.TotalBytes,
	}
}

func skillDeploymentProjection(
	item skillapplication.DeploymentInspection,
) map[string]any {
	deployment := item.Deployment
	result := map[string]any{
		"deploymentId":   deployment.ID().String(),
		"installationId": deployment.InstallationID().String(),
		"target":         string(deployment.Target()),
		"root":           deployment.Root(),
		"destination":    deployment.Destination(),
		"digest":         deployment.Digest(),
		"deployedAt":     deployment.DeployedAt(),
		"drift":          string(item.Drift),
	}
	if item.CurrentDigest != "" {
		result["currentDigest"] = item.CurrentDigest
	}
	return result
}

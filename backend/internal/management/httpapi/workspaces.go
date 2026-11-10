package httpapi

import (
	"errors"
	"net/http"

	catalog "github.com/orz-i/Mender/backend/internal/catalog/domain"
	gov "github.com/orz-i/Mender/backend/internal/governance/domain"
	profile "github.com/orz-i/Mender/backend/internal/profile/domain"
	sandbox "github.com/orz-i/Mender/backend/internal/sandbox/domain"
	app "github.com/orz-i/Mender/backend/internal/workspace/application"
	workspace "github.com/orz-i/Mender/backend/internal/workspace/domain"
)

func (h *Handler) workspaceOwner() (gov.Context, error) {
	return gov.NewContext(gov.OriginManagementHTTP, h.runtime.OwnerPrincipal(), gov.CatalogScope())
}
func workspaceAPIError(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, app.ErrRegistryDenied), errors.Is(err, sandbox.ErrNotAuthorized):
		writeAPIError(w, http.StatusForbidden, "policy_denied", err)
	case errors.Is(err, app.ErrRegistryInvalid):
		writeAPIError(w, http.StatusBadRequest, "validation_error", err)
	case errors.Is(err, app.ErrRegistryMissing), errors.Is(err, sandbox.ErrNotFound):
		writeAPIError(w, http.StatusNotFound, "not_found", err)
	case errors.Is(err, app.ErrRegistryConflict):
		writeAPIError(w, http.StatusConflict, "conflict", err)
	default:
		writeKnownError(w, err)
	}
}
func getWorkspaceRef(raw string) (workspace.Ref, error) { return workspace.ParseRef(raw) }
func (h *Handler) listWorkspaces(w http.ResponseWriter, r *http.Request) {
	caller, err := h.workspaceOwner()
	if err != nil {
		workspaceAPIError(w, err)
		return
	}
	values, err := h.runtime.Workspaces.List(r.Context(), caller)
	if err != nil {
		workspaceAPIError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"workspaces": values})
}

type createWorkspaceInput struct {
	DisplayName  string `json:"displayName"`
	VerifiedRoot string `json:"verifiedRoot"`
}

func (h *Handler) createWorkspace(w http.ResponseWriter, r *http.Request) {
	var input createWorkspaceInput
	if err := decodeJSON(r, &input); err != nil {
		writeAPIError(w, http.StatusBadRequest, "validation_error", err)
		return
	}
	caller, err := h.workspaceOwner()
	if err != nil {
		workspaceAPIError(w, err)
		return
	}
	created, err := h.runtime.Workspaces.RegisterLocal(r.Context(), caller, input.DisplayName, input.VerifiedRoot)
	if err != nil {
		workspaceAPIError(w, err)
		return
	}
	writeJSON(w, http.StatusCreated, created)
}
func (h *Handler) getWorkspace(w http.ResponseWriter, r *http.Request) {
	ref, err := getWorkspaceRef(r.PathValue("workspaceId"))
	if err != nil {
		writeAPIError(w, http.StatusBadRequest, "validation_error", err)
		return
	}
	caller, err := h.workspaceOwner()
	if err != nil {
		workspaceAPIError(w, err)
		return
	}
	result, err := h.runtime.Workspaces.Get(r.Context(), caller, ref)
	if err != nil {
		workspaceAPIError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, result)
}
func (h *Handler) deleteWorkspace(w http.ResponseWriter, r *http.Request) {
	ref, err := getWorkspaceRef(r.PathValue("workspaceId"))
	if err != nil {
		writeAPIError(w, http.StatusBadRequest, "validation_error", err)
		return
	}
	caller, err := h.workspaceOwner()
	if err != nil {
		workspaceAPIError(w, err)
		return
	}
	if err = h.runtime.Workspaces.Delete(r.Context(), caller, ref); err != nil {
		workspaceAPIError(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}
func (h *Handler) listWorkspacePlacements(w http.ResponseWriter, r *http.Request) {
	ref, err := getWorkspaceRef(r.PathValue("workspaceId"))
	if err != nil {
		writeAPIError(w, http.StatusBadRequest, "validation_error", err)
		return
	}
	caller, err := h.workspaceOwner()
	if err != nil {
		workspaceAPIError(w, err)
		return
	}
	result, err := h.runtime.Workspaces.Placements(r.Context(), caller, ref)
	if err != nil {
		workspaceAPIError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"placements": result})
}
func (h *Handler) mutateWorkspacePlacement(w http.ResponseWriter, r *http.Request) {
	ref, err := getWorkspaceRef(r.PathValue("workspaceId"))
	if err != nil {
		writeAPIError(w, http.StatusBadRequest, "validation_error", err)
		return
	}
	id, err := sandbox.ParseID(r.PathValue("sandboxId"))
	if err != nil {
		writeAPIError(w, http.StatusBadRequest, "validation_error", err)
		return
	}
	caller, err := h.workspaceOwner()
	if err != nil {
		workspaceAPIError(w, err)
		return
	}
	if r.Method == http.MethodPut {
		err = h.runtime.Workspaces.LinkLocal(r.Context(), caller, ref, id.String())
	} else {
		err = h.runtime.Workspaces.Unlink(r.Context(), caller, ref, id.String())
	}
	if err != nil {
		workspaceAPIError(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}
func (h *Handler) listWorkspaceProfileGrants(w http.ResponseWriter, r *http.Request) {
	ref, err := getWorkspaceRef(r.PathValue("workspaceId"))
	if err != nil {
		writeAPIError(w, http.StatusBadRequest, "validation_error", err)
		return
	}
	caller, err := h.workspaceOwner()
	if err != nil {
		workspaceAPIError(w, err)
		return
	}
	ids, err := h.runtime.Workspaces.Grants(r.Context(), caller, ref)
	if err != nil {
		workspaceAPIError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"profileIds": ids})
}
func (h *Handler) mutateWorkspaceProfileGrant(w http.ResponseWriter, r *http.Request) {
	ref, err := getWorkspaceRef(r.PathValue("workspaceId"))
	if err != nil {
		writeAPIError(w, http.StatusBadRequest, "validation_error", err)
		return
	}
	id := r.PathValue("profileId")
	if _, err = profile.ParseProfileID(id); err != nil {
		writeAPIError(w, http.StatusBadRequest, "validation_error", err)
		return
	}
	caller, err := h.workspaceOwner()
	if err != nil {
		workspaceAPIError(w, err)
		return
	}
	if r.Method == http.MethodPut {
		err = h.runtime.Workspaces.Grant(r.Context(), caller, ref, id)
	} else {
		err = h.runtime.Workspaces.Revoke(r.Context(), caller, ref, id)
	}
	if err != nil {
		workspaceAPIError(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}
func (h *Handler) workspaceProfileID(w http.ResponseWriter, r *http.Request) (string, bool) {
	slug, err := profile.NewSlug(r.PathValue("slug"))
	if err != nil {
		writeAPIError(w, http.StatusBadRequest, "validation_error", err)
		return "", false
	}
	p, err := h.runtime.Profiles.Get.Execute(r.Context(), slug)
	if err != nil {
		writeKnownError(w, err)
		return "", false
	}
	return p.ID().String(), true
}

type workspaceTargetInput struct {
	ToolID      string `json:"toolId"`
	WorkspaceID string `json:"workspaceId"`
	SandboxID   string `json:"sandboxId"`
}

func parseWorkspaceTarget(profileID string, input workspaceTargetInput) (app.TargetRecord, error) {
	tid, err := catalog.ParseToolID(input.ToolID)
	if err != nil {
		return app.TargetRecord{}, err
	}
	sid, err := sandbox.ParseID(input.SandboxID)
	if err != nil {
		return app.TargetRecord{}, err
	}
	if input.WorkspaceID != "" {
		if _, err = workspace.ParseRef(input.WorkspaceID); err != nil {
			return app.TargetRecord{}, err
		}
	}
	return app.TargetRecord{ProfileID: profileID, ToolID: tid, WorkspaceID: input.WorkspaceID, SandboxID: sid.String()}, nil
}
func (h *Handler) listProfileToolTargets(w http.ResponseWriter, r *http.Request) {
	id, ok := h.workspaceProfileID(w, r)
	if !ok {
		return
	}
	caller, err := h.workspaceOwner()
	if err != nil {
		workspaceAPIError(w, err)
		return
	}
	result, err := h.runtime.Workspaces.Targets(r.Context(), caller, id)
	if err != nil {
		workspaceAPIError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"targets": result})
}
func (h *Handler) createProfileToolTarget(w http.ResponseWriter, r *http.Request) {
	id, ok := h.workspaceProfileID(w, r)
	if !ok {
		return
	}
	var input workspaceTargetInput
	if err := decodeJSON(r, &input); err != nil {
		writeAPIError(w, http.StatusBadRequest, "validation_error", err)
		return
	}
	target, err := parseWorkspaceTarget(id, input)
	if err != nil {
		writeAPIError(w, http.StatusBadRequest, "validation_error", err)
		return
	}
	caller, err := h.workspaceOwner()
	if err != nil {
		workspaceAPIError(w, err)
		return
	}
	if err = h.runtime.Workspaces.PutTarget(r.Context(), caller, target); err != nil {
		workspaceAPIError(w, err)
		return
	}
	writeJSON(w, http.StatusCreated, target)
}
func (h *Handler) deleteProfileToolTarget(w http.ResponseWriter, r *http.Request) {
	id, ok := h.workspaceProfileID(w, r)
	if !ok {
		return
	}
	target, err := parseWorkspaceTarget(id, workspaceTargetInput{ToolID: r.PathValue("toolId"), SandboxID: r.PathValue("sandboxId"), WorkspaceID: r.URL.Query().Get("workspaceId")})
	if err != nil {
		writeAPIError(w, http.StatusBadRequest, "validation_error", err)
		return
	}
	caller, err := h.workspaceOwner()
	if err != nil {
		workspaceAPIError(w, err)
		return
	}
	if err = h.runtime.Workspaces.DeleteTarget(r.Context(), caller, target); err != nil {
		workspaceAPIError(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

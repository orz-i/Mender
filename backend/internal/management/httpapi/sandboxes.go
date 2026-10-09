package httpapi

import (
	"errors"
	"net/http"

	"github.com/orz-i/Mender/backend/internal/bootstrap"
	governancedomain "github.com/orz-i/Mender/backend/internal/governance/domain"
	sandboxdomain "github.com/orz-i/Mender/backend/internal/sandbox/domain"
)

type createLocalSandboxRequest struct {
	Name      string `json:"name"`
	Root      string `json:"root"`
	Isolation string `json:"isolation"`
}

func (h *Handler) listSandboxes(w http.ResponseWriter, r *http.Request) {
	items, err := h.runtime.Sandboxes.List(r.Context())
	if err != nil {
		writeKnownError(w, err)
		return
	}
	projected := make([]map[string]any, 0, len(items))
	for _, item := range items {
		state, err := h.sandboxStatus(r, item)
		if err != nil {
			writeKnownError(w, err)
			return
		}
		projected = append(projected, state)
	}
	writeJSON(w, http.StatusOK, map[string]any{"sandboxes": projected})
}
func (h *Handler) getSandbox(w http.ResponseWriter, r *http.Request) {
	id, err := sandboxdomain.ParseID(r.PathValue("sandboxId"))
	if err != nil {
		writeAPIError(w, http.StatusBadRequest, "validation_error", err)
		return
	}
	item, err := h.runtime.Sandboxes.Get(r.Context(), id)
	if errors.Is(err, sandboxdomain.ErrNotFound) {
		writeAPIError(w, http.StatusNotFound, "not_found", err)
		return
	}
	if err != nil {
		writeKnownError(w, err)
		return
	}
	projected, err := h.sandboxStatus(r, item)
	if err != nil {
		writeKnownError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, projected)
}
func (h *Handler) createLocalSandbox(w http.ResponseWriter, r *http.Request) {
	var input createLocalSandboxRequest
	if err := decodeJSON(r, &input); err != nil {
		writeAPIError(w, http.StatusBadRequest, "validation_error", err)
		return
	}
	caller, err := governancedomain.NewContext(governancedomain.OriginManagementHTTP, h.runtime.OwnerPrincipal(), governancedomain.CatalogScope())
	if err != nil {
		writeKnownError(w, err)
		return
	}
	item, err := h.runtime.Sandboxes.CreateLocalSandbox(r.Context(), bootstrap.CreateLocalSandboxRequest{Name: input.Name, Root: input.Root, Isolation: sandboxdomain.IsolationClass(input.Isolation)}, caller)
	if err != nil {
		if errors.Is(err, sandboxdomain.ErrNotAuthorized) {
			writeAPIError(w, http.StatusForbidden, "policy_denied", err)
			return
		}
		if errors.Is(err, sandboxdomain.ErrConflict) {
			writeAPIError(w, http.StatusConflict, "conflict", err)
			return
		}
		writeAPIError(w, http.StatusBadRequest, "validation_error", err)
		return
	}
	projected, err := h.sandboxStatus(r, item)
	if err != nil {
		writeKnownError(w, err)
		return
	}
	writeJSON(w, http.StatusCreated, projected)
}
func (h *Handler) sandboxStatus(r *http.Request, item sandboxdomain.Sandbox) (map[string]any, error) {
	state := "defined"
	run, err := h.runtime.Sandboxes.GetRun(r.Context(), item.ID())
	if err == nil {
		state = string(run.State)
	} else if !errors.Is(err, sandboxdomain.ErrNotFound) {
		return nil, err
	}
	spec := item.Spec()
	return map[string]any{
		"sandboxId": spec.ID.String(),
		"name":      spec.Name,
		"backend":   string(spec.Backend),
		"isolation": string(spec.Isolation),
		"state":     state,
		"localRoot": spec.LocalRoot,
		"ownerKind": spec.OwnerKind,
		"ownerRef":  spec.OwnerRef,
	}, nil
}

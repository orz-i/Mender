package httpapi

import (
	"errors"
	"net/http"

	governed "github.com/orz-i/Mender/backend/internal/governance/domain"
	sandboxdomain "github.com/orz-i/Mender/backend/internal/sandbox/domain"
	suitedomain "github.com/orz-i/Mender/backend/internal/toolsuite/domain"
)

func (h *Handler) listToolSuites(w http.ResponseWriter, r *http.Request) {
	definitions, err := h.runtime.ListToolSuiteDefinitions(r.Context())
	if err != nil {
		writeKnownError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"suites": definitions})
}

func (h *Handler) workspaceSuiteContext() (governed.Context, error) {
	return governed.NewContext(governed.OriginManagementHTTP, h.runtime.OwnerPrincipal(), governed.CatalogScope())
}

func (h *Handler) workspaceSuiteError(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, sandboxdomain.ErrNotAuthorized):
		writeAPIError(w, http.StatusForbidden, "policy_denied", err)
	case errors.Is(err, sandboxdomain.ErrNotFound):
		writeAPIError(w, http.StatusNotFound, "not_found", err)
	case errors.Is(err, sandboxdomain.ErrConflict), errors.Is(err, suitedomain.ErrIncompatible),
		errors.Is(err, suitedomain.ErrUnavailable):
		writeAPIError(w, http.StatusConflict, "conflict", err)
	default:
		writeKnownError(w, err)
	}
}

func (h *Handler) sandboxToolsRequest(r *http.Request) (sandboxdomain.ID, governed.Context, error) {
	id, err := sandboxdomain.ParseID(r.PathValue("sandboxId"))
	if err != nil {
		return "", governed.Context{}, err
	}
	caller, err := h.workspaceSuiteContext()
	return id, caller, err
}

func (h *Handler) getSandboxTools(w http.ResponseWriter, r *http.Request) {
	id, caller, err := h.sandboxToolsRequest(r)
	if err != nil {
		writeAPIError(w, http.StatusBadRequest, "validation_error", err)
		return
	}
	state, err := h.runtime.SandboxWorkspaceTools(r.Context(), id, caller)
	if err != nil {
		h.workspaceSuiteError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, state)
}
func (h *Handler) ensureSandboxTools(w http.ResponseWriter, r *http.Request) {
	id, caller, err := h.sandboxToolsRequest(r)
	if err != nil {
		writeAPIError(w, http.StatusBadRequest, "validation_error", err)
		return
	}
	state, err := h.runtime.EnsureSandboxWorkspaceTools(r.Context(), id, caller)
	if err != nil {
		h.workspaceSuiteError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, state)
}
func (h *Handler) removeSandboxTools(w http.ResponseWriter, r *http.Request) {
	id, caller, err := h.sandboxToolsRequest(r)
	if err != nil {
		writeAPIError(w, http.StatusBadRequest, "validation_error", err)
		return
	}
	if err := h.runtime.RemoveSandboxWorkspaceTools(r.Context(), id, caller); err != nil {
		h.workspaceSuiteError(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

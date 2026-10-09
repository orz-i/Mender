package httpapi

import (
	"errors"
	"net/http"

	credentialdomain "github.com/orz-i/Mender/backend/internal/credential/domain"
	sandboxapplication "github.com/orz-i/Mender/backend/internal/sandbox/application"
	sandboxdaytona "github.com/orz-i/Mender/backend/internal/sandbox/infrastructure/daytona"
	sandboxdocker "github.com/orz-i/Mender/backend/internal/sandbox/infrastructure/dockerengine"

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
	leases, err := h.runtime.Sandboxes.ActiveLeases(r.Context(), item.ID())
	if err != nil {
		return nil, err
	}
	spec := item.Spec()
	return map[string]any{
		"activeLeases":     leases,
		"backendAvailable": h.runtime.Sandboxes.BackendAvailable(item),
		"sandboxId":        spec.ID.String(),
		"name":             spec.Name,
		"backend":          string(spec.Backend),
		"isolation":        string(spec.Isolation),
		"state":            state,
		"localRoot":        spec.LocalRoot,
		"ownerKind":        spec.OwnerKind,
		"ownerRef":         spec.OwnerRef,
	}, nil
}

func (h *Handler) startLocalSandbox(w http.ResponseWriter, r *http.Request) {
	h.localSandboxLifecycle(w, r, governancedomain.ActionSandboxStart)
}
func (h *Handler) stopLocalSandbox(w http.ResponseWriter, r *http.Request) {
	h.localSandboxLifecycle(w, r, governancedomain.ActionSandboxStop)
}
func (h *Handler) destroyLocalSandbox(w http.ResponseWriter, r *http.Request) {
	h.localSandboxLifecycle(w, r, governancedomain.ActionSandboxDestroy)
}
func (h *Handler) localSandboxLifecycle(w http.ResponseWriter, r *http.Request, action governancedomain.Action) {
	id, err := sandboxdomain.ParseID(r.PathValue("sandboxId"))
	if err != nil {
		writeAPIError(w, http.StatusBadRequest, "validation_error", err)
		return
	}
	caller, err := governancedomain.NewContext(governancedomain.OriginManagementHTTP, h.runtime.OwnerPrincipal(), governancedomain.CatalogScope())
	if err != nil {
		writeKnownError(w, err)
		return
	}
	if _, err := h.runtime.Sandboxes.ChangeLifecycle(r.Context(), id, action, caller); err != nil {
		switch {
		case errors.Is(err, sandboxdomain.ErrNotFound):
			writeAPIError(w, http.StatusNotFound, "not_found", err)
		case errors.Is(err, sandboxdomain.ErrNotAuthorized), errors.Is(err, sandboxapplication.ErrApprovalDenied):
			writeAPIError(w, http.StatusForbidden, "policy_denied", err)
		case errors.Is(err, sandboxdomain.ErrConflict):
			writeAPIError(w, http.StatusConflict, "conflict", err)
		case errors.Is(err, sandboxdomain.ErrBackendUnavailable):
			writeAPIError(w, http.StatusNotImplemented, "unsupported_backend", err)
		default:
			writeKnownError(w, err)
		}
		return
	}
	item, err := h.runtime.Sandboxes.Get(r.Context(), id)
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

type createDockerSandboxRequest struct {
	Name         string `json:"name"`
	ImageDigest  string `json:"imageDigest"`
	UserID       uint32 `json:"userId"`
	GroupID      uint32 `json:"groupId"`
	CPUmilli     uint32 `json:"cpuMilli"`
	MemoryMiB    uint32 `json:"memoryMiB"`
	WorkspaceMiB uint32 `json:"workspaceMiB"`
	PIDs         uint32 `json:"pids"`
}

func (h *Handler) sandboxBackends(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, map[string]any{"dockerAvailable": h.runtime.Sandboxes.DockerAvailable(), "dockerStatus": h.runtime.Sandboxes.DockerStatus(), "daytonaConfigured": h.runtime.Sandboxes.DaytonaConfigured(), "daytonaStatus": "Credential Reference and live provider availability must be verified during creation"})
}
func (h *Handler) createDockerSandbox(w http.ResponseWriter, r *http.Request) {
	var input createDockerSandboxRequest
	if err := decodeJSON(r, &input); err != nil {
		writeAPIError(w, http.StatusBadRequest, "validation_error", err)
		return
	}
	caller, err := governancedomain.NewContext(governancedomain.OriginManagementHTTP, h.runtime.OwnerPrincipal(), governancedomain.CatalogScope())
	if err != nil {
		writeKnownError(w, err)
		return
	}
	item, err := h.runtime.Sandboxes.CreateDockerSandbox(r.Context(), bootstrap.CreateDockerSandboxRequest{
		Name: input.Name, Template: sandboxdocker.Template{ImageDigest: input.ImageDigest, UserID: input.UserID, GroupID: input.GroupID, CPUmilli: input.CPUmilli, MemoryMiB: input.MemoryMiB, WorkspaceMiB: input.WorkspaceMiB, PIDs: input.PIDs},
	}, caller)
	if err != nil {
		// Resource and immutable Template already exist when a nonzero ID is
		// returned. Expose the observed unknown/defined state, never claim ready.
		if item.ID() != "" {
			status, statusErr := h.sandboxStatus(r, item)
			if statusErr != nil {
				writeKnownError(w, statusErr)
				return
			}
			writeJSON(w, http.StatusAccepted, status)
			return
		}
		if errors.Is(err, sandboxdomain.ErrNotAuthorized) {
			writeAPIError(w, http.StatusForbidden, "policy_denied", err)
			return
		}
		if errors.Is(err, sandboxdomain.ErrBackendUnavailable) {
			writeAPIError(w, http.StatusServiceUnavailable, "backend_unavailable", err)
			return
		}
		if errors.Is(err, sandboxdomain.ErrConflict) {
			writeAPIError(w, http.StatusConflict, "conflict", err)
			return
		}
		writeAPIError(w, http.StatusBadRequest, "validation_error", err)
		return
	}
	response, err := h.sandboxStatus(r, item)
	if err != nil {
		writeKnownError(w, err)
		return
	}
	writeJSON(w, http.StatusCreated, response)
}
func (h *Handler) reconcileDockerSandbox(w http.ResponseWriter, r *http.Request) {
	id, err := sandboxdomain.ParseID(r.PathValue("sandboxId"))
	if err != nil {
		writeAPIError(w, http.StatusBadRequest, "validation_error", err)
		return
	}
	caller, err := governancedomain.NewContext(governancedomain.OriginManagementHTTP, h.runtime.OwnerPrincipal(), governancedomain.CatalogScope())
	if err != nil {
		writeKnownError(w, err)
		return
	}
	if _, err := h.runtime.Sandboxes.ReconcileDocker(r.Context(), id, caller); err != nil {
		if errors.Is(err, sandboxdomain.ErrConflict) {
			writeAPIError(w, http.StatusConflict, "conflict", err)
			return
		}
		if errors.Is(err, sandboxdomain.ErrNotAuthorized) {
			writeAPIError(w, http.StatusForbidden, "policy_denied", err)
			return
		}
		writeAPIError(w, http.StatusServiceUnavailable, "backend_unavailable", err)
		return
	}
	item, err := h.runtime.Sandboxes.Get(r.Context(), id)
	if err != nil {
		writeKnownError(w, err)
		return
	}
	result, err := h.sandboxStatus(r, item)
	if err != nil {
		writeKnownError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, result)
}

type createDaytonaSandboxRequest struct {
	Name          string `json:"name"`
	CredentialRef string `json:"credentialRef"`
	ImageDigest   string `json:"imageDigest"`
	Target        string `json:"target"`
	CPU           int    `json:"cpu"`
	MemoryGiB     int    `json:"memoryGiB"`
	DiskGiB       int    `json:"diskGiB"`
	TTLMinutes    int    `json:"ttlMinutes"`
}

func (h *Handler) createDaytonaSandbox(w http.ResponseWriter, r *http.Request) {
	var input createDaytonaSandboxRequest
	if err := decodeJSON(r, &input); err != nil {
		writeAPIError(w, http.StatusBadRequest, "validation_error", err)
		return
	}
	ref, err := credentialdomain.ParseReference(input.CredentialRef)
	if err != nil {
		writeAPIError(w, http.StatusBadRequest, "validation_error", err)
		return
	}
	caller, err := governancedomain.NewContext(governancedomain.OriginManagementHTTP, h.runtime.OwnerPrincipal(), governancedomain.CatalogScope())
	if err != nil {
		writeKnownError(w, err)
		return
	}
	template := sandboxdaytona.Template{CredentialRef: ref, ImageDigest: input.ImageDigest, Target: input.Target, CPU: input.CPU, MemoryGiB: input.MemoryGiB, DiskGiB: input.DiskGiB, TTLMinutes: input.TTLMinutes}
	item, err := h.runtime.Sandboxes.CreateDaytonaSandbox(r.Context(), bootstrap.CreateDaytonaSandboxRequest{Name: input.Name, Template: template}, caller)
	if err != nil {
		if item.ID() != "" {
			result, stateErr := h.sandboxStatus(r, item)
			if stateErr != nil {
				writeKnownError(w, stateErr)
				return
			}
			writeJSON(w, http.StatusAccepted, result)
			return
		}
		switch {
		case errors.Is(err, sandboxdomain.ErrNotAuthorized), errors.Is(err, sandboxapplication.ErrApprovalDenied):
			writeAPIError(w, http.StatusForbidden, "policy_denied", err)
		case errors.Is(err, sandboxdomain.ErrConflict):
			writeAPIError(w, http.StatusConflict, "resource_limit", err)
		case errors.Is(err, sandboxdomain.ErrBackendUnavailable):
			writeAPIError(w, http.StatusServiceUnavailable, "remote_unavailable", err)
		default:
			writeAPIError(w, http.StatusBadRequest, "validation_error", err)
		}
		return
	}
	response, err := h.sandboxStatus(r, item)
	if err != nil {
		writeKnownError(w, err)
		return
	}
	writeJSON(w, http.StatusCreated, response)
}
func (h *Handler) reconcileDaytonaSandbox(w http.ResponseWriter, r *http.Request) {
	id, err := sandboxdomain.ParseID(r.PathValue("sandboxId"))
	if err != nil {
		writeAPIError(w, http.StatusBadRequest, "validation_error", err)
		return
	}
	caller, err := governancedomain.NewContext(governancedomain.OriginManagementHTTP, h.runtime.OwnerPrincipal(), governancedomain.CatalogScope())
	if err != nil {
		writeKnownError(w, err)
		return
	}
	if _, err := h.runtime.Sandboxes.ReconcileDaytona(r.Context(), id, caller); err != nil {
		switch {
		case errors.Is(err, sandboxdomain.ErrNotAuthorized):
			writeAPIError(w, http.StatusForbidden, "policy_denied", err)
		case errors.Is(err, sandboxdomain.ErrConflict):
			writeAPIError(w, http.StatusConflict, "conflict", err)
		default:
			writeAPIError(w, http.StatusServiceUnavailable, "remote_unavailable", err)
		}
		return
	}
	item, err := h.runtime.Sandboxes.Get(r.Context(), id)
	if err != nil {
		writeKnownError(w, err)
		return
	}
	state, err := h.sandboxStatus(r, item)
	if err != nil {
		writeKnownError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, state)
}

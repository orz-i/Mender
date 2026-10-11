package httpapi

import (
	"errors"
	"fmt"
	"net/http"

	governancedomain "github.com/orz-i/Mender/backend/internal/governance/domain"
	profileapplication "github.com/orz-i/Mender/backend/internal/profile/application"
	sandboxdomain "github.com/orz-i/Mender/backend/internal/sandbox/domain"
)

func (h *Handler) sandboxGrantContext() (governancedomain.Context, error) {
	return governancedomain.NewContext(governancedomain.OriginManagementHTTP, h.runtime.OwnerPrincipal(), governancedomain.CatalogScope())
}

func grantAPIError(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, sandboxdomain.ErrNotFound), errors.Is(err, profileapplication.ErrProfileNotFound):
		writeAPIError(w, http.StatusNotFound, "not_found", err)
	case errors.Is(err, sandboxdomain.ErrNotAuthorized):
		writeAPIError(w, http.StatusForbidden, "policy_denied", err)
	case errors.Is(err, sandboxdomain.ErrConflict):
		writeAPIError(w, http.StatusConflict, "conflict", err)
	default:
		writeKnownError(w, err)
	}
}
func (h *Handler) getSandboxProfileGrants(w http.ResponseWriter, r *http.Request) {
	id, err := sandboxdomain.ParseID(r.PathValue("sandboxId"))
	if err != nil {
		writeAPIError(w, http.StatusBadRequest, "validation_error", err)
		return
	}
	caller, err := h.sandboxGrantContext()
	if err != nil {
		grantAPIError(w, err)
		return
	}
	values, err := h.runtime.Sandboxes.ListProfileGrants(r.Context(), id, caller)
	if err != nil {
		grantAPIError(w, err)
		return
	}
	grants := make([]map[string]any, 0, len(values))
	for _, value := range values {
		capabilities := make([]string, 0, len(value.Capabilities))
		for _, capability := range value.Capabilities {
			capabilities = append(capabilities, string(capability))
		}
		grants = append(grants, map[string]any{"profileId": value.ProfileID, "capabilities": capabilities})
	}
	writeJSON(w, http.StatusOK, map[string]any{"sandboxId": id.String(), "grants": grants})
}

type sandboxGrantInput struct {
	Capabilities []string `json:"capabilities"`
}

func (h *Handler) putSandboxProfileGrants(w http.ResponseWriter, r *http.Request) {
	id, err := sandboxdomain.ParseID(r.PathValue("sandboxId"))
	if err != nil {
		writeAPIError(w, http.StatusBadRequest, "validation_error", err)
		return
	}
	caller, err := h.sandboxGrantContext()
	if err != nil {
		grantAPIError(w, err)
		return
	}
	var input sandboxGrantInput
	if err = decodeJSON(r, &input); err != nil {
		writeAPIError(w, http.StatusBadRequest, "validation_error", err)
		return
	}
	if len(input.Capabilities) == 0 {
		writeAPIError(w, http.StatusBadRequest, "validation_error", fmt.Errorf("grant requires capabilities; use DELETE to revoke"))
		return
	}
	capabilities := make([]sandboxdomain.Capability, 0, len(input.Capabilities))
	for _, raw := range input.Capabilities {
		capability := sandboxdomain.Capability(raw)
		if err = capability.Validate(); err != nil {
			writeAPIError(w, http.StatusBadRequest, "validation_error", err)
			return
		}
		capabilities = append(capabilities, capability)
	}
	if err = h.runtime.Sandboxes.SetProfileGrants(r.Context(), id, r.PathValue("profileId"), capabilities, caller); err != nil {
		grantAPIError(w, err)
		return
	}
	h.getSandboxProfileGrants(w, r)
}

func (h *Handler) deleteSandboxProfileGrants(w http.ResponseWriter, r *http.Request) {
	id, err := sandboxdomain.ParseID(r.PathValue("sandboxId"))
	if err != nil {
		writeAPIError(w, http.StatusBadRequest, "validation_error", err)
		return
	}
	caller, err := h.sandboxGrantContext()
	if err != nil {
		grantAPIError(w, err)
		return
	}
	if err = h.runtime.Sandboxes.SetProfileGrants(r.Context(), id, r.PathValue("profileId"), nil, caller); err != nil {
		grantAPIError(w, err)
		return
	}
	h.getSandboxProfileGrants(w, r)
}

func (h *Handler) getSandboxProfileGrantEvents(w http.ResponseWriter, r *http.Request) {
	id, err := sandboxdomain.ParseID(r.PathValue("sandboxId"))
	if err != nil {
		writeAPIError(w, http.StatusBadRequest, "validation_error", err)
		return
	}
	caller, err := h.sandboxGrantContext()
	if err != nil {
		grantAPIError(w, err)
		return
	}
	events, err := h.runtime.Sandboxes.ListProfileGrantEvents(r.Context(), id, caller, 100)
	if err != nil {
		grantAPIError(w, err)
		return
	}
	out := make([]map[string]any, 0, len(events))
	for _, event := range events {
		out = append(out, map[string]any{"profileId": event.ProfileID, "capability": string(event.Capability), "action": event.Action, "occurredAt": event.OccurredAt})
	}
	writeJSON(w, http.StatusOK, map[string]any{"sandboxId": id.String(), "events": out})
}

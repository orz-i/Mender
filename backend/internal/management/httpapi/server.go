package httpapi

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"

	"github.com/orz-i/Mender/backend/internal/bootstrap"
	catalogapplication "github.com/orz-i/Mender/backend/internal/catalog/application"
	invocationapplication "github.com/orz-i/Mender/backend/internal/invocation/application"
	mcpconfigstore "github.com/orz-i/Mender/backend/internal/mcp/infrastructure/configstore"
	profileapplication "github.com/orz-i/Mender/backend/internal/profile/application"
)

type Handler struct {
	runtime *bootstrap.Runtime
	mux     *http.ServeMux
}

func New(runtime *bootstrap.Runtime) *Handler {
	h := &Handler{runtime: runtime, mux: http.NewServeMux()}
	h.routes()
	return h
}

func (h *Handler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	h.mux.ServeHTTP(w, r)
}

func (h *Handler) routes() {
	h.mux.HandleFunc("GET /api/v1/system/status", h.getSystemStatus)
	h.mux.HandleFunc("GET /api/v1/policy/status", h.getPolicyStatus)
	h.mux.HandleFunc("GET /api/v1/credentials/status", h.getCredentialStatus)

	h.mux.HandleFunc("GET /api/v1/providers", h.listProviders)
	h.mux.HandleFunc("POST /api/v1/providers/http", h.createHTTPProvider)
	h.mux.HandleFunc("POST /api/v1/providers/stdio", h.createStdioProvider)
	h.mux.HandleFunc("POST /api/v1/providers/{providerId}/sync", h.syncProvider)
	h.mux.HandleFunc("DELETE /api/v1/providers/{providerId}", h.deleteProvider)

	h.mux.HandleFunc("GET /api/v1/tools", h.searchTools)
	h.mux.HandleFunc("GET /api/v1/tools/{toolId}", h.getTool)
	h.mux.HandleFunc("POST /api/v1/tools/{toolId}/call", h.callTool)

	h.mux.HandleFunc("GET /api/v1/profiles", h.listProfiles)
	h.mux.HandleFunc("POST /api/v1/profiles", h.createProfile)
	h.mux.HandleFunc("GET /api/v1/profiles/{slug}", h.getProfile)
	h.mux.HandleFunc("POST /api/v1/profiles/{slug}/tools", h.addProfileTool)
	h.mux.HandleFunc("DELETE /api/v1/profiles/{slug}/tools/{toolId}", h.removeProfileTool)

	h.mux.HandleFunc("GET /api/v1/audit", h.listAudit)
}

type apiError struct {
	Code    string `json:"code"`
	Message string `json:"message"`
}

func decodeJSON(r *http.Request, value any) error {
	decoder := json.NewDecoder(r.Body)
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(value); err != nil {
		return fmt.Errorf("decode request body: %w", err)
	}
	var extra any
	err := decoder.Decode(&extra)
	if err == nil {
		return fmt.Errorf("request body must contain exactly one JSON document")
	}
	if !errors.Is(err, io.EOF) {
		return fmt.Errorf("decode trailing request data: %w", err)
	}
	return nil
}

func writeJSON(w http.ResponseWriter, status int, value any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(value)
}

func writeAPIError(w http.ResponseWriter, status int, code string, err error) {
	writeJSON(w, status, apiError{Code: code, Message: err.Error()})
}

func writeKnownError(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, catalogapplication.ErrProviderNotFound),
		errors.Is(err, catalogapplication.ErrToolNotFound),
		errors.Is(err, profileapplication.ErrProfileNotFound),
		errors.Is(err, mcpconfigstore.ErrNotFound):
		writeAPIError(w, http.StatusNotFound, "not_found", err)

	case errors.Is(err, catalogapplication.ErrNamespaceConflict),
		errors.Is(err, profileapplication.ErrSlugConflict),
		errors.Is(err, bootstrap.ErrProviderInUse):
		writeAPIError(w, http.StatusConflict, "conflict", err)

	case errors.Is(err, catalogapplication.ErrInvalidSearchLimit),
		errors.Is(err, catalogapplication.ErrInvalidSearchQuery):
		writeAPIError(w, http.StatusBadRequest, "validation_error", err)

	case errors.Is(err, invocationapplication.ErrPolicyDenied),
		errors.Is(err, invocationapplication.ErrApprovalDenied):
		writeAPIError(w, http.StatusForbidden, "policy_denied", err)

	default:
		writeAPIError(w, http.StatusInternalServerError, "internal_error", err)
	}
}

package httpapi

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"strings"

	"github.com/orz-i/Mender/backend/internal/bootstrap"
	catalogapplication "github.com/orz-i/Mender/backend/internal/catalog/application"
	catalogdomain "github.com/orz-i/Mender/backend/internal/catalog/domain"
	credentialapplication "github.com/orz-i/Mender/backend/internal/credential/application"
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
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("X-Content-Type-Options", "nosniff")

	if err := validateLocalRequest(r); err != nil {
		writeAPIError(w, http.StatusForbidden, "request_forbidden", err)
		return
	}
	h.mux.ServeHTTP(w, r)
}

func (h *Handler) routes() {
	h.mux.HandleFunc("GET /api/v1/system/status", h.getSystemStatus)
	h.mux.HandleFunc("GET /api/v1/policy/status", h.getPolicyStatus)
	h.mux.HandleFunc("GET /api/v1/credentials/status", h.getCredentialStatus)
	h.mux.HandleFunc("GET /api/v1/credentials", h.listCredentials)
	h.mux.HandleFunc("POST /api/v1/credentials/native", h.createNativeCredential)
	h.mux.HandleFunc("POST /api/v1/credentials/environment", h.createEnvironmentCredential)
	h.mux.HandleFunc("GET /api/v1/credentials/{namespace}/{name}", h.getCredential)
	h.mux.HandleFunc("POST /api/v1/credentials/{namespace}/{name}/rotate", h.rotateCredential)
	h.mux.HandleFunc("DELETE /api/v1/credentials/{namespace}/{name}", h.deleteCredential)

	h.mux.HandleFunc("GET /api/v1/providers", h.listProviders)
	h.mux.HandleFunc("POST /api/v1/providers/http", h.createHTTPProvider)
	h.mux.HandleFunc("POST /api/v1/providers/stdio", h.createStdioProvider)
	h.mux.HandleFunc("POST /api/v1/providers/{providerId}/sync", h.syncProvider)
	h.mux.HandleFunc("PUT /api/v1/providers/{providerId}", h.updateProvider)
	h.mux.HandleFunc("DELETE /api/v1/providers/{providerId}", h.deleteProvider)

	h.mux.HandleFunc("GET /api/v1/tools", h.searchTools)
	h.mux.HandleFunc("GET /api/v1/tools/browse", h.browseTools)
	h.mux.HandleFunc("GET /api/v1/tools/{toolId}", h.getTool)
	h.mux.HandleFunc("POST /api/v1/tools/{toolId}/call", h.callTool)

	h.mux.HandleFunc("GET /api/v1/profiles", h.listProfiles)
	h.mux.HandleFunc("POST /api/v1/profiles", h.createProfile)
	h.mux.HandleFunc("GET /api/v1/profiles/{slug}", h.getProfile)
	h.mux.HandleFunc("POST /api/v1/profiles/{slug}/tools", h.addProfileTool)
	h.mux.HandleFunc("PUT /api/v1/profiles/{slug}/tools", h.replaceProfileTools)
	h.mux.HandleFunc("DELETE /api/v1/profiles/{slug}/tools/{toolId}", h.removeProfileTool)

	h.mux.HandleFunc("GET /api/v1/mcp/servers", h.listMCPServers)
	h.mux.HandleFunc("POST /api/v1/mcp/servers", h.startMCPServer)
	h.mux.HandleFunc("POST /api/v1/mcp/servers/{serverId}/restart", h.restartMCPServer)
	h.mux.HandleFunc("DELETE /api/v1/mcp/servers/{serverId}", h.stopMCPServer)

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

func validateLocalRequest(r *http.Request) error {
	if !isLoopbackHost(r.Host) {
		return fmt.Errorf("management request Host must be localhost or loopback")
	}

	if origin := strings.TrimSpace(r.Header.Get("Origin")); origin != "" {
		parsed, err := url.Parse(origin)
		if err != nil {
			return fmt.Errorf("management request Origin is invalid")
		}
		if (parsed.Scheme != "http" && parsed.Scheme != "https") ||
			!isLoopbackHostname(parsed.Hostname()) {
			return fmt.Errorf("management request Origin must be localhost or loopback")
		}
	}

	if strings.EqualFold(
		strings.TrimSpace(r.Header.Get("Sec-Fetch-Site")),
		"cross-site",
	) {
		return fmt.Errorf("cross-site management requests are forbidden")
	}
	return nil
}

func isLoopbackHost(value string) bool {
	value = strings.TrimSpace(value)
	if value == "" {
		return false
	}

	if host, _, err := net.SplitHostPort(value); err == nil {
		return isLoopbackHostname(host)
	}

	if strings.HasPrefix(value, "[") && strings.HasSuffix(value, "]") {
		value = strings.TrimSuffix(strings.TrimPrefix(value, "["), "]")
	}
	return isLoopbackHostname(value)
}

func isLoopbackHostname(host string) bool {
	if strings.EqualFold(strings.TrimSpace(host), "localhost") {
		return true
	}
	ip := net.ParseIP(strings.TrimSpace(host))
	return ip != nil && ip.IsLoopback()
}

func writeKnownError(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, catalogapplication.ErrProviderNotFound),
		errors.Is(err, catalogapplication.ErrToolNotFound),
		errors.Is(err, credentialapplication.ErrCredentialNotFound),
		errors.Is(err, profileapplication.ErrProfileNotFound),
		errors.Is(err, mcpconfigstore.ErrNotFound),
		errors.Is(err, bootstrap.ErrMCPServerNotFound):
		writeAPIError(w, http.StatusNotFound, "not_found", err)

	case errors.Is(err, catalogapplication.ErrNamespaceConflict),
		errors.Is(err, credentialapplication.ErrReferenceConflict),
		errors.Is(err, credentialapplication.ErrCredentialInUse),
		errors.Is(err, profileapplication.ErrSlugConflict),
		errors.Is(err, profileapplication.ErrBindingNameConflict),
		errors.Is(err, bootstrap.ErrProviderInUse):
		writeAPIError(w, http.StatusConflict, "conflict", err)

	case errors.Is(err, catalogapplication.ErrInvalidSearchLimit),
		errors.Is(err, catalogapplication.ErrInvalidSearchQuery),
		errors.Is(err, credentialapplication.ErrInvalidSecret),
		errors.Is(err, credentialapplication.ErrInvalidSource),
		errors.Is(err, profileapplication.ErrInvalidBinding),
		errors.Is(err, bootstrap.ErrInvalidMCPServerAddress):
		writeAPIError(w, http.StatusBadRequest, "validation_error", err)

	case errors.Is(err, credentialapplication.ErrSecretSourceUnavailable),
		errors.Is(err, credentialapplication.ErrEnvironmentMissing):
		writeAPIError(w, http.StatusServiceUnavailable, "credential_unavailable", err)

	case errors.Is(err, invocationapplication.ErrPolicyDenied),
		errors.Is(err, invocationapplication.ErrApprovalDenied):
		writeAPIError(w, http.StatusForbidden, "policy_denied", err)

	default:
		var validationErr catalogdomain.ValidationError
		if errors.As(err, &validationErr) {
			writeAPIError(w, http.StatusBadRequest, "validation_error", err)
			return
		}
		writeAPIError(w, http.StatusInternalServerError, "internal_error", err)
	}
}

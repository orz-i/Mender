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
	policyapplication "github.com/orz-i/Mender/backend/internal/policy/application"
	profileapplication "github.com/orz-i/Mender/backend/internal/profile/application"
	provideractivationapplication "github.com/orz-i/Mender/backend/internal/provideractivation/application"
	providerplugin "github.com/orz-i/Mender/backend/internal/providerplugin"
	providerconfigstore "github.com/orz-i/Mender/backend/internal/providerplugin/configstore"
	installationapplication "github.com/orz-i/Mender/backend/internal/providerplugin/installation/application"
	skillapplication "github.com/orz-i/Mender/backend/internal/skill/application"
	tunnelapplication "github.com/orz-i/Mender/backend/internal/tunnel/application"
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
	h.mux.HandleFunc("GET /api/v1/runtime-governance", h.getRuntimeGovernance)
	h.mux.HandleFunc("PUT /api/v1/runtime-governance/budget", h.updateRuntimeBudget)
	h.mux.HandleFunc("PUT /api/v1/runtime-governance/isolation", h.updateRuntimeIsolation)
	h.mux.HandleFunc("GET /api/v1/policy/status", h.getPolicyStatus)
	h.mux.HandleFunc("GET /api/v1/policy", h.getPolicy)
	h.mux.HandleFunc("PUT /api/v1/policy/default", h.setPolicyDefault)
	h.mux.HandleFunc("POST /api/v1/policy/rules", h.createPolicyRule)
	h.mux.HandleFunc("PUT /api/v1/policy/rules/{ruleId}", h.updatePolicyRule)
	h.mux.HandleFunc("DELETE /api/v1/policy/rules/{ruleId}", h.deletePolicyRule)
	h.mux.HandleFunc("POST /api/v1/policy/rules/{ruleId}/enable", h.enablePolicyRule)
	h.mux.HandleFunc("POST /api/v1/policy/rules/{ruleId}/disable", h.disablePolicyRule)
	h.mux.HandleFunc("GET /api/v1/credentials/status", h.getCredentialStatus)
	h.mux.HandleFunc("GET /api/v1/credentials", h.listCredentials)
	h.mux.HandleFunc("POST /api/v1/credentials/native", h.createNativeCredential)
	h.mux.HandleFunc("POST /api/v1/credentials/environment", h.createEnvironmentCredential)
	h.mux.HandleFunc("GET /api/v1/credentials/{namespace}/{name}", h.getCredential)
	h.mux.HandleFunc("POST /api/v1/credentials/{namespace}/{name}/rotate", h.rotateCredential)
	h.mux.HandleFunc("DELETE /api/v1/credentials/{namespace}/{name}", h.deleteCredential)

	h.mux.HandleFunc("GET /api/v1/provider-plugins", h.listProviderPlugins)
	h.mux.HandleFunc("GET /api/v1/provider-plugin-installations", h.listProviderPluginInstallations)
	h.mux.HandleFunc("POST /api/v1/provider-plugin-installations/{installationId}/enable", h.enableProviderPluginInstallation)
	h.mux.HandleFunc("POST /api/v1/provider-plugin-installations/{installationId}/disable", h.disableProviderPluginInstallation)
	h.mux.HandleFunc("DELETE /api/v1/provider-plugin-installations/{installationId}", h.deleteProviderPluginInstallation)

	h.mux.HandleFunc("GET /api/v1/skills", h.listSkills)
	h.mux.HandleFunc("GET /api/v1/skills/{installationId}", h.getSkillInstallation)
	h.mux.HandleFunc("POST /api/v1/skills/{installationId}/trust", h.trustSkillInstallation)
	h.mux.HandleFunc("POST /api/v1/skills/{installationId}/untrust", h.untrustSkillInstallation)
	h.mux.HandleFunc("POST /api/v1/skills/{installationId}/enable", h.enableSkillInstallation)
	h.mux.HandleFunc("POST /api/v1/skills/{installationId}/disable", h.disableSkillInstallation)
	h.mux.HandleFunc("DELETE /api/v1/skills/{installationId}", h.deleteSkillInstallation)
	h.mux.HandleFunc("GET /api/v1/skill-deployments", h.listSkillDeployments)
	h.mux.HandleFunc("DELETE /api/v1/skill-deployments/{deploymentId}", h.undeploySkill)

	h.mux.HandleFunc("GET /api/v1/providers", h.listProviders)
	h.mux.HandleFunc("POST /api/v1/providers", h.createProvider)
	h.mux.HandleFunc("POST /api/v1/providers/http", h.createHTTPProvider)
	h.mux.HandleFunc("POST /api/v1/providers/stdio", h.createStdioProvider)
	h.mux.HandleFunc("POST /api/v1/providers/{providerId}/rebind", h.rebindProvider)
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
	h.mux.HandleFunc("PUT /api/v1/profiles/{slug}/tunnel", h.setProfileTunnel)

	h.mux.HandleFunc("GET /api/v1/sandboxes", h.listSandboxes)
	h.mux.HandleFunc("POST /api/v1/sandboxes/local", h.createLocalSandbox)
	h.mux.HandleFunc("GET /api/v1/sandboxes/{sandboxId}", h.getSandbox)
	h.mux.HandleFunc("GET /api/v1/tunnels", h.listTunnels)
	h.mux.HandleFunc("POST /api/v1/tunnels/openai", h.createOpenAITunnel)
	h.mux.HandleFunc("DELETE /api/v1/tunnels/{tunnelId}", h.deleteTunnel)

	h.mux.HandleFunc("GET /api/v1/mcp/servers", h.listMCPServers)
	h.mux.HandleFunc("POST /api/v1/mcp/servers", h.startMCPServer)
	h.mux.HandleFunc("POST /api/v1/mcp/servers/{serverId}/restart", h.restartMCPServer)
	h.mux.HandleFunc("DELETE /api/v1/mcp/servers/{serverId}", h.stopMCPServer)

	h.mux.HandleFunc("GET /api/v1/audit", h.listAudit)
	h.mux.HandleFunc("GET /api/v1/audit/provider-activations", h.listProviderActivationAudit)
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
		errors.Is(err, policyapplication.ErrRuleNotFound),
		errors.Is(err, credentialapplication.ErrCredentialNotFound),
		errors.Is(err, profileapplication.ErrProfileNotFound),
		errors.Is(err, mcpconfigstore.ErrNotFound),
		errors.Is(err, providerconfigstore.ErrNotFound),
		errors.Is(err, installationapplication.ErrInstallationNotFound),
		errors.Is(err, skillapplication.ErrInstallationNotFound),
		errors.Is(err, skillapplication.ErrDeploymentNotFound),
		errors.Is(err, tunnelapplication.ErrTunnelNotFound),
		errors.Is(err, bootstrap.ErrMCPServerNotFound):
		writeAPIError(w, http.StatusNotFound, "not_found", err)

	case errors.Is(err, catalogapplication.ErrNamespaceConflict),
		errors.Is(err, policyapplication.ErrRuleConflict),
		errors.Is(err, credentialapplication.ErrReferenceConflict),
		errors.Is(err, credentialapplication.ErrCredentialInUse),
		errors.Is(err, profileapplication.ErrSlugConflict),
		errors.Is(err, profileapplication.ErrBindingNameConflict),
		errors.Is(err, installationapplication.ErrInstallationConflict),
		errors.Is(err, installationapplication.ErrPackageAlreadyInstalled),
		errors.Is(err, installationapplication.ErrInstallationInUse),
		errors.Is(err, skillapplication.ErrInstallationConflict),
		errors.Is(err, skillapplication.ErrPackageAlreadyInstalled),
		errors.Is(err, skillapplication.ErrInstallationInUse),
		errors.Is(err, skillapplication.ErrInstallationUntrusted),
		errors.Is(err, skillapplication.ErrInstallationDisabled),
		errors.Is(err, skillapplication.ErrDeploymentConflict),
		errors.Is(err, skillapplication.ErrDeploymentTargetExists),
		errors.Is(err, skillapplication.ErrAgentPluginTargetExists),
		errors.Is(err, bootstrap.ErrBuiltInSkillImmutable),
		errors.Is(err, tunnelapplication.ErrTunnelConflict),
		errors.Is(err, tunnelapplication.ErrTunnelInUse),
		errors.Is(err, bootstrap.ErrProviderInUse):
		writeAPIError(w, http.StatusConflict, "conflict", err)

	case errors.Is(err, catalogapplication.ErrInvalidSearchLimit),
		errors.Is(err, policyapplication.ErrInvalidRule),
		errors.Is(err, policyapplication.ErrProfileReferenceNotFound),
		errors.Is(err, policyapplication.ErrProviderReferenceNotFound),
		errors.Is(err, policyapplication.ErrToolReferenceNotFound),
		errors.Is(err, catalogapplication.ErrInvalidSearchQuery),
		errors.Is(err, credentialapplication.ErrInvalidSecret),
		errors.Is(err, credentialapplication.ErrInvalidSource),
		errors.Is(err, profileapplication.ErrInvalidBinding),
		errors.Is(err, providerplugin.ErrNotFound),
		errors.Is(err, providerplugin.ErrUnsupportedConfigVersion),
		errors.Is(err, skillapplication.ErrInvalidAgentPluginPackage),
		errors.Is(err, bootstrap.ErrInvalidMCPServerAddress):
		writeAPIError(w, http.StatusBadRequest, "validation_error", err)

	case errors.Is(err, credentialapplication.ErrSecretSourceUnavailable),
		errors.Is(err, credentialapplication.ErrEnvironmentMissing):
		writeAPIError(w, http.StatusServiceUnavailable, "credential_unavailable", err)

	case errors.Is(err, invocationapplication.ErrPolicyDenied),
		errors.Is(err, invocationapplication.ErrApprovalDenied),
		errors.Is(err, provideractivationapplication.ErrPolicyDenied),
		errors.Is(err, provideractivationapplication.ErrApprovalDenied):
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

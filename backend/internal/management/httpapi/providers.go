package httpapi

import (
	"fmt"
	"net/http"

	credentialdomain "github.com/orz-i/Mender/backend/internal/credential/domain"

	catalogapplication "github.com/orz-i/Mender/backend/internal/catalog/application"
	catalogdomain "github.com/orz-i/Mender/backend/internal/catalog/domain"
	mcpconfigstore "github.com/orz-i/Mender/backend/internal/mcp/infrastructure/configstore"
)

type credentialHeaderRequest struct {
	Name          string `json:"name"`
	Scheme        string `json:"scheme"`
	CredentialRef string `json:"credentialRef"`
}

type credentialEnvironmentRequest struct {
	Name          string `json:"name"`
	CredentialRef string `json:"credentialRef"`
}

type createHTTPProviderRequest struct {
	Namespace         string                    `json:"namespace"`
	DisplayName       string                    `json:"displayName"`
	Description       string                    `json:"description"`
	Endpoint          string                    `json:"endpoint"`
	Headers           map[string]string         `json:"headers"`
	CredentialHeaders []credentialHeaderRequest `json:"credentialHeaders"`
}

type createStdioProviderRequest struct {
	Namespace             string                         `json:"namespace"`
	DisplayName           string                         `json:"displayName"`
	Description           string                         `json:"description"`
	Executable            string                         `json:"executable"`
	Args                  []string                       `json:"args"`
	Directory             string                         `json:"directory"`
	CredentialEnvironment []credentialEnvironmentRequest `json:"credentialEnvironment"`
}

type updateProviderRequest struct {
	Transport             string                         `json:"transport"`
	DisplayName           string                         `json:"displayName"`
	Description           string                         `json:"description"`
	Endpoint              string                         `json:"endpoint"`
	Headers               map[string]string              `json:"headers"`
	CredentialHeaders     []credentialHeaderRequest      `json:"credentialHeaders"`
	Executable            string                         `json:"executable"`
	Args                  []string                       `json:"args"`
	Directory             string                         `json:"directory"`
	CredentialEnvironment []credentialEnvironmentRequest `json:"credentialEnvironment"`
}

func (h *Handler) listProviders(w http.ResponseWriter, r *http.Request) {
	providers, err := h.runtime.Catalog.ListProviders.Execute(r.Context())
	if err != nil {
		writeKnownError(w, err)
		return
	}
	configs, err := h.runtime.ListMCPProviderConfigs(r.Context())
	if err != nil {
		writeKnownError(w, err)
		return
	}
	toolCounts, err := h.runtime.Catalog.ListProviderToolCounts.Execute(r.Context())
	if err != nil {
		writeKnownError(w, err)
		return
	}

	configByProvider := make(map[catalogdomain.ProviderID]mcpconfigstore.Config, len(configs))
	for _, config := range configs {
		configByProvider[config.ProviderID] = config
	}

	items := make([]map[string]any, 0, len(providers))
	for _, provider := range providers {
		items = append(
			items,
			providerProjection(
				provider,
				configByProvider[provider.ID()],
				toolCounts[provider.ID()],
			),
		)
	}
	writeJSON(w, http.StatusOK, map[string]any{"providers": items})
}

func (h *Handler) createHTTPProvider(w http.ResponseWriter, r *http.Request) {
	var input createHTTPProviderRequest
	if err := decodeJSON(r, &input); err != nil {
		writeAPIError(w, http.StatusBadRequest, "validation_error", err)
		return
	}
	if input.Namespace == "" || input.DisplayName == "" || input.Endpoint == "" {
		writeAPIError(
			w,
			http.StatusBadRequest,
			"validation_error",
			fmt.Errorf("namespace, displayName, and endpoint are required"),
		)
		return
	}

	credentialHeaders, err := parseCredentialHeaders(input.CredentialHeaders)
	if err != nil {
		writeAPIError(w, http.StatusBadRequest, "validation_error", err)
		return
	}

	provider, err := h.runtime.RegisterMCPProvider(
		r.Context(),
		catalogapplication.RegisterProviderRequest{
			Namespace:   input.Namespace,
			DisplayName: input.DisplayName,
			Description: input.Description,
		},
		mcpconfigstore.Config{
			Transport:         mcpconfigstore.TransportHTTP,
			Endpoint:          input.Endpoint,
			Headers:           cloneHeaders(input.Headers),
			CredentialHeaders: credentialHeaders,
		},
	)
	if err != nil {
		writeKnownError(w, err)
		return
	}

	syncResult, err := h.runtime.SyncConfiguredMCPProvider(r.Context(), provider.ID())
	if err != nil {
		writeAPIError(w, http.StatusBadGateway, "provider_error", err)
		return
	}

	config, _ := h.runtime.MCPProviderConfig(r.Context(), provider.ID())
	writeJSON(w, http.StatusCreated, map[string]any{
		"provider": providerProjection(provider, config, syncResult.Total),
		"sync":     syncProjection(syncResult),
	})
}

func (h *Handler) createStdioProvider(w http.ResponseWriter, r *http.Request) {
	var input createStdioProviderRequest
	if err := decodeJSON(r, &input); err != nil {
		writeAPIError(w, http.StatusBadRequest, "validation_error", err)
		return
	}
	if input.Namespace == "" || input.DisplayName == "" || input.Executable == "" {
		writeAPIError(
			w,
			http.StatusBadRequest,
			"validation_error",
			fmt.Errorf("namespace, displayName, and executable are required"),
		)
		return
	}

	credentialEnvironment, err := parseCredentialEnvironment(input.CredentialEnvironment)
	if err != nil {
		writeAPIError(w, http.StatusBadRequest, "validation_error", err)
		return
	}

	provider, err := h.runtime.RegisterMCPProvider(
		r.Context(),
		catalogapplication.RegisterProviderRequest{
			Namespace:   input.Namespace,
			DisplayName: input.DisplayName,
			Description: input.Description,
		},
		mcpconfigstore.Config{
			Transport:             mcpconfigstore.TransportStdio,
			Executable:            input.Executable,
			Args:                  append([]string(nil), input.Args...),
			Directory:             input.Directory,
			CredentialEnvironment: credentialEnvironment,
		},
	)
	if err != nil {
		writeKnownError(w, err)
		return
	}

	syncResult, err := h.runtime.SyncConfiguredMCPProvider(r.Context(), provider.ID())
	if err != nil {
		writeAPIError(w, http.StatusBadGateway, "provider_error", err)
		return
	}

	config, _ := h.runtime.MCPProviderConfig(r.Context(), provider.ID())
	writeJSON(w, http.StatusCreated, map[string]any{
		"provider": providerProjection(provider, config, syncResult.Total),
		"sync":     syncProjection(syncResult),
	})
}

func (h *Handler) updateProvider(w http.ResponseWriter, r *http.Request) {
	providerID, err := catalogdomain.ParseProviderID(r.PathValue("providerId"))
	if err != nil {
		writeAPIError(w, http.StatusBadRequest, "validation_error", err)
		return
	}

	var input updateProviderRequest
	if err := decodeJSON(r, &input); err != nil {
		writeAPIError(w, http.StatusBadRequest, "validation_error", err)
		return
	}
	if input.DisplayName == "" {
		writeAPIError(
			w,
			http.StatusBadRequest,
			"validation_error",
			fmt.Errorf("displayName is required"),
		)
		return
	}

	var config mcpconfigstore.Config
	switch input.Transport {
	case string(mcpconfigstore.TransportHTTP):
		if input.Endpoint == "" {
			writeAPIError(
				w,
				http.StatusBadRequest,
				"validation_error",
				fmt.Errorf("endpoint is required for HTTP provider"),
			)
			return
		}
		credentialHeaders, parseErr := parseCredentialHeaders(input.CredentialHeaders)
		if parseErr != nil {
			writeAPIError(w, http.StatusBadRequest, "validation_error", parseErr)
			return
		}
		config = mcpconfigstore.Config{
			Transport:         mcpconfigstore.TransportHTTP,
			Endpoint:          input.Endpoint,
			Headers:           cloneHeaders(input.Headers),
			CredentialHeaders: credentialHeaders,
		}
	case string(mcpconfigstore.TransportStdio):
		if input.Executable == "" {
			writeAPIError(
				w,
				http.StatusBadRequest,
				"validation_error",
				fmt.Errorf("executable is required for stdio provider"),
			)
			return
		}
		credentialEnvironment, parseErr := parseCredentialEnvironment(input.CredentialEnvironment)
		if parseErr != nil {
			writeAPIError(w, http.StatusBadRequest, "validation_error", parseErr)
			return
		}
		config = mcpconfigstore.Config{
			Transport:             mcpconfigstore.TransportStdio,
			Executable:            input.Executable,
			Args:                  append([]string(nil), input.Args...),
			Directory:             input.Directory,
			CredentialEnvironment: credentialEnvironment,
		}
	default:
		writeAPIError(
			w,
			http.StatusBadRequest,
			"validation_error",
			fmt.Errorf("transport must be http or stdio"),
		)
		return
	}

	provider, err := h.runtime.UpdateMCPProvider(
		r.Context(),
		providerID,
		catalogapplication.UpdateProviderRequest{
			DisplayName: input.DisplayName,
			Description: input.Description,
		},
		config,
	)
	if err != nil {
		writeKnownError(w, err)
		return
	}

	syncResult, err := h.runtime.SyncConfiguredMCPProvider(r.Context(), providerID)
	if err != nil {
		writeAPIError(w, http.StatusBadGateway, "provider_error", err)
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"provider": providerProjection(provider, config, syncResult.Total),
		"sync":     syncProjection(syncResult),
	})
}

func (h *Handler) syncProvider(w http.ResponseWriter, r *http.Request) {
	providerID, err := catalogdomain.ParseProviderID(r.PathValue("providerId"))
	if err != nil {
		writeAPIError(w, http.StatusBadRequest, "validation_error", err)
		return
	}
	result, err := h.runtime.SyncConfiguredMCPProvider(r.Context(), providerID)
	if err != nil {
		writeAPIError(w, http.StatusBadGateway, "provider_error", err)
		return
	}
	writeJSON(w, http.StatusOK, syncProjection(result))
}

func (h *Handler) deleteProvider(w http.ResponseWriter, r *http.Request) {
	providerID, err := catalogdomain.ParseProviderID(r.PathValue("providerId"))
	if err != nil {
		writeAPIError(w, http.StatusBadRequest, "validation_error", err)
		return
	}
	if err := h.runtime.DeleteMCPProvider(r.Context(), providerID); err != nil {
		writeKnownError(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func providerProjection(
	provider catalogdomain.Provider,
	config mcpconfigstore.Config,
	toolCount int,
) map[string]any {
	result := map[string]any{
		"providerId":  provider.ID().String(),
		"namespace":   provider.Namespace().String(),
		"displayName": provider.DisplayName(),
		"description": provider.Description(),
		"createdAt":   provider.CreatedAt(),
		"updatedAt":   provider.UpdatedAt(),
		"transport":   "unconfigured",
		"toolCount":   toolCount,
	}

	switch config.Transport {
	case mcpconfigstore.TransportHTTP:
		result["transport"] = "http"
		result["endpoint"] = config.Endpoint
		result["headers"] = cloneHeaders(config.Headers)
		result["credentialHeaders"] = credentialHeadersProjection(config.CredentialHeaders)
	case mcpconfigstore.TransportStdio:
		result["transport"] = "stdio"
		result["executable"] = config.Executable
		result["args"] = append([]string(nil), config.Args...)
		result["directory"] = config.Directory
		result["credentialEnvironment"] = credentialEnvironmentProjection(config.CredentialEnvironment)
	}
	return result
}

func parseCredentialHeaders(
	input []credentialHeaderRequest,
) ([]mcpconfigstore.CredentialHeader, error) {
	result := make([]mcpconfigstore.CredentialHeader, 0, len(input))
	for _, item := range input {
		ref, err := credentialdomain.ParseReference(item.CredentialRef)
		if err != nil {
			return nil, fmt.Errorf("credential header %q: %w", item.Name, err)
		}
		result = append(result, mcpconfigstore.CredentialHeader{
			Name:      item.Name,
			Scheme:    item.Scheme,
			Reference: ref,
		})
	}
	return result, nil
}

func parseCredentialEnvironment(
	input []credentialEnvironmentRequest,
) ([]mcpconfigstore.CredentialEnvironment, error) {
	result := make([]mcpconfigstore.CredentialEnvironment, 0, len(input))
	for _, item := range input {
		ref, err := credentialdomain.ParseReference(item.CredentialRef)
		if err != nil {
			return nil, fmt.Errorf("credential environment %q: %w", item.Name, err)
		}
		result = append(result, mcpconfigstore.CredentialEnvironment{
			Name:      item.Name,
			Reference: ref,
		})
	}
	return result, nil
}

func credentialHeadersProjection(
	items []mcpconfigstore.CredentialHeader,
) []map[string]any {
	result := make([]map[string]any, 0, len(items))
	for _, item := range items {
		result = append(result, map[string]any{
			"name":          item.Name,
			"scheme":        item.Scheme,
			"credentialRef": item.Reference.String(),
		})
	}
	return result
}

func credentialEnvironmentProjection(
	items []mcpconfigstore.CredentialEnvironment,
) []map[string]any {
	result := make([]map[string]any, 0, len(items))
	for _, item := range items {
		result = append(result, map[string]any{
			"name":          item.Name,
			"credentialRef": item.Reference.String(),
		})
	}
	return result
}

func cloneHeaders(headers map[string]string) map[string]string {
	if len(headers) == 0 {
		return map[string]string{}
	}
	result := make(map[string]string, len(headers))
	for name, value := range headers {
		result[name] = value
	}
	return result
}

func syncProjection(result catalogapplication.SyncProviderToolsResult) map[string]any {
	return map[string]any{
		"created": result.Created,
		"updated": result.Updated,
		"removed": result.Removed,
		"total":   result.Total,
	}
}

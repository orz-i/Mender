package httpapi

import (
	"fmt"
	"net/http"

	catalogapplication "github.com/orz-i/Mender/backend/internal/catalog/application"
	catalogdomain "github.com/orz-i/Mender/backend/internal/catalog/domain"
	mcpconfigstore "github.com/orz-i/Mender/backend/internal/mcp/infrastructure/configstore"
)

type createHTTPProviderRequest struct {
	Namespace   string `json:"namespace"`
	DisplayName string `json:"displayName"`
	Description string `json:"description"`
	Endpoint    string `json:"endpoint"`
}

type createStdioProviderRequest struct {
	Namespace   string   `json:"namespace"`
	DisplayName string   `json:"displayName"`
	Description string   `json:"description"`
	Executable  string   `json:"executable"`
	Args        []string `json:"args"`
	Directory   string   `json:"directory"`
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
	configByProvider := make(map[catalogdomain.ProviderID]mcpconfigstore.Config, len(configs))
	for _, config := range configs {
		configByProvider[config.ProviderID] = config
	}

	items := make([]map[string]any, 0, len(providers))
	for _, provider := range providers {
		item := providerProjection(provider, configByProvider[provider.ID()])
		items = append(items, item)
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
		writeAPIError(w, http.StatusBadRequest, "validation_error", fmt.Errorf("namespace, displayName, and endpoint are required"))
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
			Transport: mcpconfigstore.TransportHTTP,
			Endpoint:  input.Endpoint,
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
		"provider": providerProjection(provider, config),
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
		writeAPIError(w, http.StatusBadRequest, "validation_error", fmt.Errorf("namespace, displayName, and executable are required"))
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
			Transport:  mcpconfigstore.TransportStdio,
			Executable: input.Executable,
			Args:       append([]string(nil), input.Args...),
			Directory:  input.Directory,
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
		"provider": providerProjection(provider, config),
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
) map[string]any {
	result := map[string]any{
		"providerId":  provider.ID().String(),
		"namespace":   provider.Namespace().String(),
		"displayName": provider.DisplayName(),
		"description": provider.Description(),
		"createdAt":   provider.CreatedAt(),
		"updatedAt":   provider.UpdatedAt(),
		"transport":   "unconfigured",
	}

	switch config.Transport {
	case mcpconfigstore.TransportHTTP:
		result["transport"] = "http"
		result["endpoint"] = config.Endpoint
	case mcpconfigstore.TransportStdio:
		result["transport"] = "stdio"
		result["executable"] = config.Executable
		result["args"] = append([]string(nil), config.Args...)
		result["directory"] = config.Directory
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

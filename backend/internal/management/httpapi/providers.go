package httpapi

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strings"

	"github.com/orz-i/Mender/backend/internal/bootstrap"
	credentialdomain "github.com/orz-i/Mender/backend/internal/credential/domain"

	catalogapplication "github.com/orz-i/Mender/backend/internal/catalog/application"
	catalogdomain "github.com/orz-i/Mender/backend/internal/catalog/domain"
	mcpconfigstore "github.com/orz-i/Mender/backend/internal/mcp/infrastructure/configstore"
	mcpplugin "github.com/orz-i/Mender/backend/internal/mcp/infrastructure/plugin"
	providerplugin "github.com/orz-i/Mender/backend/internal/providerplugin"
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

type createProviderRequest struct {
	Namespace      string          `json:"namespace"`
	DisplayName    string          `json:"displayName"`
	Description    string          `json:"description"`
	PluginID       string          `json:"pluginId"`
	InstallationID string          `json:"installationId"`
	Config         json.RawMessage `json:"config"`
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

type rebindProviderRequest struct {
	InstallationID string          `json:"installationId"`
	Config         json.RawMessage `json:"config"`
}

type updateProviderRequest struct {
	DisplayName string          `json:"displayName"`
	Description string          `json:"description"`
	Config      json.RawMessage `json:"config"`

	Transport             string                         `json:"transport"`
	Endpoint              string                         `json:"endpoint"`
	Headers               map[string]string              `json:"headers"`
	CredentialHeaders     []credentialHeaderRequest      `json:"credentialHeaders"`
	Executable            string                         `json:"executable"`
	Args                  []string                       `json:"args"`
	Directory             string                         `json:"directory"`
	CredentialEnvironment []credentialEnvironmentRequest `json:"credentialEnvironment"`
}

func (h *Handler) listProviderPlugins(w http.ResponseWriter, r *http.Request) {
	available, err := h.runtime.ListAvailableProviderPlugins(r.Context())
	if err != nil {
		writeKnownError(w, err)
		return
	}
	items := make([]map[string]any, 0, len(available))
	for _, item := range available {
		items = append(items, availableProviderPluginProjection(item))
	}
	writeJSON(w, http.StatusOK, map[string]any{"plugins": items})
}

func (h *Handler) listProviders(w http.ResponseWriter, r *http.Request) {
	providers, err := h.runtime.Catalog.ListProviders.Execute(r.Context())
	if err != nil {
		writeKnownError(w, err)
		return
	}
	configs, err := h.runtime.ListProviderConfigs(r.Context())
	if err != nil {
		writeKnownError(w, err)
		return
	}
	toolCounts, err := h.runtime.Catalog.ListProviderToolCounts.Execute(r.Context())
	if err != nil {
		writeKnownError(w, err)
		return
	}

	configByProvider := make(
		map[catalogdomain.ProviderID]providerplugin.Configuration,
		len(configs),
	)
	for _, config := range configs {
		configByProvider[config.ProviderID] = config
	}
	descriptorByID := make(map[providerplugin.ID]providerplugin.Descriptor)
	for _, descriptor := range h.runtime.ProviderPluginDescriptors() {
		descriptorByID[descriptor.ID] = descriptor
	}

	items := make([]map[string]any, 0, len(providers))
	for _, provider := range providers {
		config, configured := configByProvider[provider.ID()]
		var configPtr *providerplugin.Configuration
		var descriptorPtr *providerplugin.Descriptor
		if configured {
			configCopy := config
			configPtr = &configCopy
			if descriptor, ok := descriptorByID[config.PluginID]; ok {
				descriptorCopy := descriptor
				descriptorPtr = &descriptorCopy
			}
		}
		items = append(
			items,
			providerProjection(
				provider,
				configPtr,
				descriptorPtr,
				toolCounts[provider.ID()],
			),
		)
	}
	writeJSON(w, http.StatusOK, map[string]any{"providers": items})
}

func (h *Handler) createProvider(w http.ResponseWriter, r *http.Request) {
	var input createProviderRequest
	if err := decodeJSON(r, &input); err != nil {
		writeAPIError(w, http.StatusBadRequest, "validation_error", err)
		return
	}
	if input.Namespace == "" ||
		input.DisplayName == "" ||
		input.PluginID == "" ||
		len(input.Config) == 0 {
		writeAPIError(
			w,
			http.StatusBadRequest,
			"validation_error",
			fmt.Errorf("namespace, displayName, pluginId, and config are required"),
		)
		return
	}
	pluginID, err := providerplugin.ParseID(input.PluginID)
	if err != nil {
		writeAPIError(w, http.StatusBadRequest, "validation_error", err)
		return
	}

	registerRequest := catalogapplication.RegisterProviderRequest{
		Namespace:   input.Namespace,
		DisplayName: input.DisplayName,
		Description: input.Description,
	}
	var provider catalogdomain.Provider
	if strings.TrimSpace(input.InstallationID) == "" {
		provider, err = h.runtime.RegisterConfiguredProvider(
			r.Context(),
			registerRequest,
			pluginID,
			input.Config,
		)
	} else {
		installationID, parseErr := providerplugin.ParseInstallationID(
			input.InstallationID,
		)
		if parseErr != nil {
			writeAPIError(w, http.StatusBadRequest, "validation_error", parseErr)
			return
		}
		provider, err = h.runtime.RegisterInstalledProvider(
			r.Context(),
			registerRequest,
			pluginID,
			installationID,
			input.Config,
		)
	}
	if err != nil {
		writeKnownError(w, err)
		return
	}

	syncResult, err := h.runtime.SyncConfiguredProvider(r.Context(), provider.ID())
	if err != nil {
		writeAPIError(w, http.StatusBadGateway, "provider_error", err)
		return
	}
	config, _ := h.runtime.ProviderConfig(r.Context(), provider.ID())
	descriptor := descriptorForPlugin(h.runtime.ProviderPluginDescriptors(), config.PluginID)
	writeJSON(w, http.StatusCreated, map[string]any{
		"provider": providerProjection(provider, &config, descriptor, syncResult.Total),
		"sync":     syncProjection(syncResult),
	})
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

	syncResult, err := h.runtime.SyncConfiguredProvider(r.Context(), provider.ID())
	if err != nil {
		writeAPIError(w, http.StatusBadGateway, "provider_error", err)
		return
	}

	config, _ := h.runtime.ProviderConfig(r.Context(), provider.ID())
	descriptor := descriptorForPlugin(h.runtime.ProviderPluginDescriptors(), config.PluginID)
	writeJSON(w, http.StatusCreated, map[string]any{
		"provider": providerProjection(provider, &config, descriptor, syncResult.Total),
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

	syncResult, err := h.runtime.SyncConfiguredProvider(r.Context(), provider.ID())
	if err != nil {
		writeAPIError(w, http.StatusBadGateway, "provider_error", err)
		return
	}

	config, _ := h.runtime.ProviderConfig(r.Context(), provider.ID())
	descriptor := descriptorForPlugin(h.runtime.ProviderPluginDescriptors(), config.PluginID)
	writeJSON(w, http.StatusCreated, map[string]any{
		"provider": providerProjection(provider, &config, descriptor, syncResult.Total),
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

	if len(input.Config) != 0 {
		h.updateConfiguredProvider(w, r, providerID, input)
		return
	}
	h.updateLegacyMCPProvider(w, r, providerID, input)
}

func (h *Handler) updateConfiguredProvider(
	w http.ResponseWriter,
	r *http.Request,
	providerID catalogdomain.ProviderID,
	input updateProviderRequest,
) {
	provider, err := h.runtime.UpdateConfiguredProvider(
		r.Context(),
		providerID,
		catalogapplication.UpdateProviderRequest{
			DisplayName: input.DisplayName,
			Description: input.Description,
		},
		input.Config,
	)
	if err != nil {
		writeKnownError(w, err)
		return
	}
	syncResult, err := h.runtime.SyncConfiguredProvider(r.Context(), providerID)
	if err != nil {
		writeAPIError(w, http.StatusBadGateway, "provider_error", err)
		return
	}
	config, _ := h.runtime.ProviderConfig(r.Context(), providerID)
	descriptor := descriptorForPlugin(h.runtime.ProviderPluginDescriptors(), config.PluginID)
	writeJSON(w, http.StatusOK, map[string]any{
		"provider": providerProjection(provider, &config, descriptor, syncResult.Total),
		"sync":     syncProjection(syncResult),
	})
}

func (h *Handler) updateLegacyMCPProvider(
	w http.ResponseWriter,
	r *http.Request,
	providerID catalogdomain.ProviderID,
	input updateProviderRequest,
) {
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
		credentialHeaders, err := parseCredentialHeaders(input.CredentialHeaders)
		if err != nil {
			writeAPIError(w, http.StatusBadRequest, "validation_error", err)
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
		credentialEnvironment, err := parseCredentialEnvironment(input.CredentialEnvironment)
		if err != nil {
			writeAPIError(w, http.StatusBadRequest, "validation_error", err)
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
	syncResult, err := h.runtime.SyncConfiguredProvider(r.Context(), providerID)
	if err != nil {
		writeAPIError(w, http.StatusBadGateway, "provider_error", err)
		return
	}
	genericConfig, _ := h.runtime.ProviderConfig(r.Context(), providerID)
	descriptor := descriptorForPlugin(
		h.runtime.ProviderPluginDescriptors(),
		genericConfig.PluginID,
	)
	writeJSON(w, http.StatusOK, map[string]any{
		"provider": providerProjection(
			provider,
			&genericConfig,
			descriptor,
			syncResult.Total,
		),
		"sync": syncProjection(syncResult),
	})
}

func (h *Handler) rebindProvider(w http.ResponseWriter, r *http.Request) {
	providerID, err := catalogdomain.ParseProviderID(r.PathValue("providerId"))
	if err != nil {
		writeAPIError(w, http.StatusBadRequest, "validation_error", err)
		return
	}
	var input rebindProviderRequest
	if err := decodeJSON(r, &input); err != nil {
		writeAPIError(w, http.StatusBadRequest, "validation_error", err)
		return
	}
	if strings.TrimSpace(input.InstallationID) == "" || len(input.Config) == 0 {
		writeAPIError(
			w,
			http.StatusBadRequest,
			"validation_error",
			fmt.Errorf("installationId and config are required"),
		)
		return
	}
	installationID, err := providerplugin.ParseInstallationID(input.InstallationID)
	if err != nil {
		writeAPIError(w, http.StatusBadRequest, "validation_error", err)
		return
	}
	result, err := h.runtime.RebindProviderInstallation(
		r.Context(),
		providerID,
		installationID,
		input.Config,
	)
	if err != nil {
		writeAPIError(w, http.StatusBadGateway, "provider_error", err)
		return
	}
	provider, err := h.runtime.Catalog.GetProvider.Execute(r.Context(), providerID)
	if err != nil {
		writeKnownError(w, err)
		return
	}
	config, err := h.runtime.ProviderConfig(r.Context(), providerID)
	if err != nil {
		writeKnownError(w, err)
		return
	}
	descriptor := descriptorForPlugin(h.runtime.ProviderPluginDescriptors(), config.PluginID)
	writeJSON(w, http.StatusOK, map[string]any{
		"provider": providerProjection(provider, &config, descriptor, result.Total),
		"sync":     syncProjection(result),
	})
}

func (h *Handler) syncProvider(w http.ResponseWriter, r *http.Request) {
	providerID, err := catalogdomain.ParseProviderID(r.PathValue("providerId"))
	if err != nil {
		writeAPIError(w, http.StatusBadRequest, "validation_error", err)
		return
	}
	result, err := h.runtime.SyncConfiguredProvider(r.Context(), providerID)
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

	// Until compatibility MCP persistence is retired, use the compatibility
	// delete path so legacy rows cannot become stale. It also removes the
	// generic configuration and canonical Provider.
	if err := h.runtime.DeleteMCPProvider(r.Context(), providerID); err != nil {
		writeKnownError(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func providerProjection(
	provider catalogdomain.Provider,
	config *providerplugin.Configuration,
	descriptor *providerplugin.Descriptor,
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
	if config == nil {
		return result
	}

	result["pluginId"] = config.PluginID.String()
	if config.InstallationID != nil {
		result["installationId"] = config.InstallationID.String()
	}
	result["configVersion"] = config.ConfigVersion
	if metadata, ok := config.ConfigMetadata(); ok && metadata.ConnectionSummary != "" {
		result["connectionSummary"] = metadata.ConnectionSummary
	}
	result["config"] = cloneRawMessage(config.Config)
	if descriptor != nil {
		result["pluginVersion"] = descriptor.Version
		result["pluginDisplayName"] = descriptor.DisplayName
	}

	if config.PluginID != mcpplugin.ID {
		return result
	}
	mcpConfig, err := mcpplugin.DecodeConfig(config.Config)
	if err != nil {
		return result
	}
	switch mcpConfig.Transport.Kind {
	case mcpplugin.TransportStreamableHTTP:
		result["transport"] = "http"
		result["endpoint"] = mcpConfig.Transport.Endpoint
		result["headers"] = cloneHeaders(mcpConfig.Transport.Headers)
		result["credentialHeaders"] = mcpCredentialHeadersProjection(
			mcpConfig.Transport.CredentialHeaders,
		)
	case mcpplugin.TransportStdio:
		result["transport"] = "stdio"
		result["executable"] = mcpConfig.Transport.Executable
		result["args"] = append([]string(nil), mcpConfig.Transport.Args...)
		result["directory"] = mcpConfig.Transport.Directory
		result["credentialEnvironment"] = mcpCredentialEnvironmentProjection(
			mcpConfig.Transport.CredentialEnvironment,
		)
	}
	return result
}

func availableProviderPluginProjection(
	item bootstrap.AvailableProviderPlugin,
) map[string]any {
	descriptor := item.Descriptor
	result := map[string]any{
		"pluginId":       descriptor.ID.String(),
		"version":        descriptor.Version,
		"packageVersion": descriptor.Version,
		"displayName":    descriptor.DisplayName,
		"description":    descriptor.Description,
		"configVersion":  descriptor.ConfigVersion,
		"sourceKind":     item.SourceKind,
		"enabled":        item.Enabled,
		"capabilities":   append([]string{}, descriptor.Capabilities...),
	}
	if item.InstallationID != nil {
		result["installationId"] = item.InstallationID.String()
	}
	if descriptor.ProtocolVersion != 0 {
		result["protocolVersion"] = descriptor.ProtocolVersion
	}
	if len(descriptor.ConfigSchema) != 0 {
		result["configSchema"] = append(json.RawMessage(nil), descriptor.ConfigSchema...)
	}
	if len(descriptor.UISchema) != 0 {
		result["uiSchema"] = append(json.RawMessage(nil), descriptor.UISchema...)
	}
	return result
}

func descriptorForPlugin(
	descriptors []providerplugin.Descriptor,
	pluginID providerplugin.ID,
) *providerplugin.Descriptor {
	for _, descriptor := range descriptors {
		if descriptor.ID == pluginID {
			copy := descriptor
			return &copy
		}
	}
	return nil
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

func mcpCredentialHeadersProjection(
	items []mcpplugin.CredentialHeader,
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

func mcpCredentialEnvironmentProjection(
	items []mcpplugin.CredentialEnvironment,
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

func cloneRawMessage(raw json.RawMessage) json.RawMessage {
	if raw == nil {
		return nil
	}
	return append(json.RawMessage(nil), raw...)
}

func syncProjection(result catalogapplication.SyncProviderToolsResult) map[string]any {
	return map[string]any{
		"created": result.Created,
		"updated": result.Updated,
		"removed": result.Removed,
		"total":   result.Total,
	}
}

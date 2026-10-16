package httpapi

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"

	"github.com/orz-i/Mender/backend/internal/bootstrap"

	catalogapplication "github.com/orz-i/Mender/backend/internal/catalog/application"
	catalogdomain "github.com/orz-i/Mender/backend/internal/catalog/domain"
	mcpplugin "github.com/orz-i/Mender/backend/internal/mcp/infrastructure/plugin"
	providerplugin "github.com/orz-i/Mender/backend/internal/providerplugin"
)

type createProviderRequest struct {
	Namespace      string          `json:"namespace"`
	DisplayName    string          `json:"displayName"`
	Description    string          `json:"description"`
	PluginID       string          `json:"pluginId"`
	InstallationID string          `json:"installationId"`
	Config         json.RawMessage `json:"config"`
}

type rebindProviderRequest struct {
	InstallationID string          `json:"installationId"`
	Config         json.RawMessage `json:"config"`
}

type updateProviderRequest struct {
	DisplayName string          `json:"displayName"`
	Description string          `json:"description"`
	Config      json.RawMessage `json:"config"`
}

func (h *Handler) listProviderPlugins(w http.ResponseWriter, r *http.Request) {
	catalog, err := h.runtime.ListAvailableProviderPluginCatalog(r.Context())
	if err != nil {
		writeKnownError(w, err)
		return
	}
	items := make([]map[string]any, 0, len(catalog.Plugins))
	for _, item := range catalog.Plugins {
		items = append(items, availableProviderPluginProjection(item))
	}
	issues := make([]map[string]any, 0, len(catalog.Issues))
	for _, issue := range catalog.Issues {
		issues = append(issues, providerPluginIssueProjection(issue))
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"plugins": items,
		"issues":  issues,
	})
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
	items := make([]map[string]any, 0, len(providers))
	for _, provider := range providers {
		config, configured := configByProvider[provider.ID()]
		var configPtr *providerplugin.Configuration
		var descriptorPtr *providerplugin.Descriptor
		if configured {
			configCopy := config
			configPtr = &configCopy
			descriptorPtr = descriptorForProviderConfig(
				r.Context(),
				h.runtime,
				config,
			)
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
		provider, err = h.registerConfiguredProvider(
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
		provider, err = h.registerInstalledProvider(
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

	syncResult, err := h.syncConfiguredProvider(r.Context(), provider.ID())
	if err != nil {
		writeAPIError(w, http.StatusBadGateway, "provider_error", err)
		return
	}
	config, _ := h.runtime.ProviderConfig(r.Context(), provider.ID())
	descriptor := descriptorForProviderConfig(r.Context(), h.runtime, config)
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

	if len(input.Config) == 0 {
		writeAPIError(w, http.StatusBadRequest, "validation_error", fmt.Errorf("config is required; legacy flat MCP Provider update is retired"))
		return
	}
	h.updateConfiguredProvider(w, r, providerID, input)
}

func (h *Handler) updateConfiguredProvider(
	w http.ResponseWriter,
	r *http.Request,
	providerID catalogdomain.ProviderID,
	input updateProviderRequest,
) {
	provider, err := h.updateConfiguredProviderGoverned(
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
	syncResult, err := h.syncConfiguredProvider(r.Context(), providerID)
	if err != nil {
		writeAPIError(w, http.StatusBadGateway, "provider_error", err)
		return
	}
	config, _ := h.runtime.ProviderConfig(r.Context(), providerID)
	descriptor := descriptorForProviderConfig(r.Context(), h.runtime, config)
	writeJSON(w, http.StatusOK, map[string]any{
		"provider": providerProjection(provider, &config, descriptor, syncResult.Total),
		"sync":     syncProjection(syncResult),
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
	result, err := h.rebindProviderInstallation(
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
	descriptor := descriptorForProviderConfig(r.Context(), h.runtime, config)
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
	result, err := h.syncConfiguredProvider(r.Context(), providerID)
	if err != nil {
		writeKnownError(w, err)
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

	if err := h.runtime.DeleteConfiguredProvider(r.Context(), providerID); err != nil {
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
	if safeConfig, err := providerplugin.ManagementSafeConfig(descriptor, config.Config); err == nil {
		result["config"] = safeConfig
	}
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

func providerPluginIssueProjection(issue bootstrap.ProviderPluginIssue) map[string]any {
	return map[string]any{
		"installationId": issue.InstallationID.String(),
		"code":           issue.Code,
		"message":        issue.Message,
	}
}

func descriptorForProviderConfig(
	ctx context.Context,
	runtime *bootstrap.Runtime,
	config providerplugin.Configuration,
) *providerplugin.Descriptor {
	descriptor, ok, err := runtime.ProviderPluginDescriptorForConfig(ctx, config)
	if err != nil || !ok {
		return nil
	}
	return &descriptor
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

func syncProjection(result catalogapplication.SyncProviderToolsResult) map[string]any {
	return map[string]any{
		"created": result.Created,
		"updated": result.Updated,
		"removed": result.Removed,
		"total":   result.Total,
	}
}

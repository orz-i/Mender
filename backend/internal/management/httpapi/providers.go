package httpapi

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strings"

	"github.com/orz-i/Mender/backend/internal/bootstrap"

	catalogapplication "github.com/orz-i/Mender/backend/internal/catalog/application"
	catalogdomain "github.com/orz-i/Mender/backend/internal/catalog/domain"
	managementquery "github.com/orz-i/Mender/backend/internal/management/application/query"
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
	providers, err := h.runtime.ListManagementProviders(r.Context())
	if err != nil {
		writeKnownError(w, err)
		return
	}
	items := make([]map[string]any, 0, len(providers))
	for _, provider := range providers {
		items = append(items, providerViewProjection(provider))
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
		if errors.Is(err, bootstrap.ErrProviderInUse) {
			writeKnownError(w, err)
		} else {
			writeAPIError(w, http.StatusBadGateway, "provider_error", err)
		}
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
	return providerViewProjection(bootstrap.ManagementProviderView(provider, config, descriptor, toolCount))
}

// providerViewProjection preserves HTTP field presence independently of the
// shared query model and the desktop binding's omitempty representation.
func providerViewProjection(provider managementquery.ProviderView) map[string]any {
	result := map[string]any{
		"providerId":  provider.ProviderID,
		"namespace":   provider.Namespace,
		"displayName": provider.DisplayName,
		"description": provider.Description,
		"createdAt":   provider.CreatedAt,
		"updatedAt":   provider.UpdatedAt,
		"transport":   "unconfigured",
		"toolCount":   provider.ToolCount,
	}
	if source := provider.Source; source != nil {
		result["toolSourceKind"] = source.Kind
		result["toolSourceSelectable"] = source.Selectable
		if source.ResourceID != "" {
			result["toolSourceId"] = source.ResourceID
		}
		if source.Name != "" {
			result["toolSourceName"] = source.Name
		}
	}
	config := provider.Configuration
	if config == nil {
		return result
	}

	result["pluginId"] = config.PluginID
	if config.InstallationID != "" {
		result["installationId"] = config.InstallationID
	}
	result["configVersion"] = config.ConfigVersion
	if config.ConnectionSummary != "" {
		result["connectionSummary"] = config.ConnectionSummary
	}
	if config.HasSafeConfig {
		result["config"] = config.SafeConfig
	}
	if config.Plugin != nil {
		result["pluginVersion"] = config.Plugin.Version
		result["pluginDisplayName"] = config.Plugin.DisplayName
	}

	connection := config.Connection
	if connection == nil {
		return result
	}
	result["transport"] = connection.Transport
	switch connection.Transport {
	case "http":
		result["endpoint"] = connection.Endpoint
		result["headers"] = connection.Headers
		if connection.OAuthCredentialRef != "" {
			result["oauthCredentialRef"] = connection.OAuthCredentialRef
		}
		headers := make([]map[string]any, 0, len(connection.CredentialHeaders))
		for _, header := range connection.CredentialHeaders {
			headers = append(headers, map[string]any{
				"name": header.Name, "scheme": header.Scheme, "credentialRef": header.CredentialRef,
			})
		}
		result["credentialHeaders"] = headers
	case "stdio":
		result["executable"] = connection.Executable
		result["args"] = connection.Args
		result["directory"] = connection.Directory
		environment := make([]map[string]any, 0, len(connection.CredentialEnvironment))
		for _, variable := range connection.CredentialEnvironment {
			environment = append(environment, map[string]any{
				"name": variable.Name, "credentialRef": variable.CredentialRef,
			})
		}
		result["credentialEnvironment"] = environment
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

func syncProjection(result catalogapplication.SyncProviderToolsResult) map[string]any {
	return map[string]any{
		"created": result.Created,
		"updated": result.Updated,
		"removed": result.Removed,
		"total":   result.Total,
	}
}

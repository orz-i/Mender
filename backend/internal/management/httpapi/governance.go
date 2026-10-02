package httpapi

import (
	"context"
	"encoding/json"

	"github.com/orz-i/Mender/backend/internal/bootstrap"
	catalogapplication "github.com/orz-i/Mender/backend/internal/catalog/application"
	catalogdomain "github.com/orz-i/Mender/backend/internal/catalog/domain"
	governancedomain "github.com/orz-i/Mender/backend/internal/governance/domain"
	profiledomain "github.com/orz-i/Mender/backend/internal/profile/domain"
	providerplugin "github.com/orz-i/Mender/backend/internal/providerplugin"
)

func (h *Handler) governanceContext() (governancedomain.Context, error) {
	return governancedomain.NewContext(
		governancedomain.OriginManagementHTTP,
		h.runtime.OwnerPrincipal(),
		governancedomain.CatalogScope(),
	)
}

func (h *Handler) registerConfiguredProvider(
	ctx context.Context,
	request catalogapplication.RegisterProviderRequest,
	pluginID providerplugin.ID,
	rawConfig json.RawMessage,
) (catalogdomain.Provider, error) {
	governanceContext, err := h.governanceContext()
	if err != nil {
		return catalogdomain.Provider{}, err
	}
	return h.runtime.RegisterConfiguredProvider(
		ctx,
		request,
		pluginID,
		rawConfig,
		governanceContext,
	)
}

func (h *Handler) registerInstalledProvider(
	ctx context.Context,
	request catalogapplication.RegisterProviderRequest,
	pluginID providerplugin.ID,
	installationID providerplugin.InstallationID,
	rawConfig json.RawMessage,
) (catalogdomain.Provider, error) {
	governanceContext, err := h.governanceContext()
	if err != nil {
		return catalogdomain.Provider{}, err
	}
	return h.runtime.RegisterInstalledProvider(
		ctx,
		request,
		pluginID,
		installationID,
		rawConfig,
		governanceContext,
	)
}

func (h *Handler) updateConfiguredProviderGoverned(
	ctx context.Context,
	providerID catalogdomain.ProviderID,
	request catalogapplication.UpdateProviderRequest,
	rawConfig json.RawMessage,
) (catalogdomain.Provider, error) {
	governanceContext, err := h.governanceContext()
	if err != nil {
		return catalogdomain.Provider{}, err
	}
	return h.runtime.UpdateConfiguredProvider(
		ctx,
		providerID,
		request,
		rawConfig,
		governanceContext,
	)
}

func (h *Handler) syncConfiguredProvider(
	ctx context.Context,
	providerID catalogdomain.ProviderID,
) (catalogapplication.SyncProviderToolsResult, error) {
	governanceContext, err := h.governanceContext()
	if err != nil {
		return catalogapplication.SyncProviderToolsResult{}, err
	}
	return h.runtime.SyncConfiguredProvider(ctx, providerID, governanceContext)
}

func (h *Handler) rebindProviderInstallation(
	ctx context.Context,
	providerID catalogdomain.ProviderID,
	installationID providerplugin.InstallationID,
	rawConfig json.RawMessage,
) (catalogapplication.SyncProviderToolsResult, error) {
	governanceContext, err := h.governanceContext()
	if err != nil {
		return catalogapplication.SyncProviderToolsResult{}, err
	}
	return h.runtime.RebindProviderInstallation(
		ctx,
		providerID,
		installationID,
		rawConfig,
		governanceContext,
	)
}

func (h *Handler) startMCPHTTPServer(
	ctx context.Context,
	slug profiledomain.Slug,
	address string,
) (bootstrap.MCPServerInstance, error) {
	governanceContext, err := h.governanceContext()
	if err != nil {
		return bootstrap.MCPServerInstance{}, err
	}
	return h.runtime.StartMCPHTTPServer(ctx, slug, address, governanceContext)
}

func (h *Handler) restartManagedMCPServer(
	ctx context.Context,
	serverID string,
) (bootstrap.MCPServerInstance, error) {
	governanceContext, err := h.governanceContext()
	if err != nil {
		return bootstrap.MCPServerInstance{}, err
	}
	return h.runtime.RestartMCPServer(ctx, serverID, governanceContext)
}

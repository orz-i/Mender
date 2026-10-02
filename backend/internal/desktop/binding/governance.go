package binding

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

func (s *Service) governanceContext() (governancedomain.Context, error) {
	return governancedomain.NewContext(
		governancedomain.OriginDesktop,
		s.runtime.OwnerPrincipal(),
		governancedomain.CatalogScope(),
	)
}

func (s *Service) registerConfiguredProvider(
	ctx context.Context,
	request catalogapplication.RegisterProviderRequest,
	pluginID providerplugin.ID,
	rawConfig json.RawMessage,
) (catalogdomain.Provider, error) {
	governanceContext, err := s.governanceContext()
	if err != nil {
		return catalogdomain.Provider{}, err
	}
	return s.runtime.RegisterConfiguredProvider(
		ctx,
		request,
		pluginID,
		rawConfig,
		governanceContext,
	)
}

func (s *Service) registerInstalledProvider(
	ctx context.Context,
	request catalogapplication.RegisterProviderRequest,
	pluginID providerplugin.ID,
	installationID providerplugin.InstallationID,
	rawConfig json.RawMessage,
) (catalogdomain.Provider, error) {
	governanceContext, err := s.governanceContext()
	if err != nil {
		return catalogdomain.Provider{}, err
	}
	return s.runtime.RegisterInstalledProvider(
		ctx,
		request,
		pluginID,
		installationID,
		rawConfig,
		governanceContext,
	)
}

func (s *Service) updateConfiguredProvider(
	ctx context.Context,
	providerID catalogdomain.ProviderID,
	request catalogapplication.UpdateProviderRequest,
	rawConfig json.RawMessage,
) (catalogdomain.Provider, error) {
	governanceContext, err := s.governanceContext()
	if err != nil {
		return catalogdomain.Provider{}, err
	}
	return s.runtime.UpdateConfiguredProvider(
		ctx,
		providerID,
		request,
		rawConfig,
		governanceContext,
	)
}

func (s *Service) syncConfiguredProvider(
	ctx context.Context,
	providerID catalogdomain.ProviderID,
) (catalogapplication.SyncProviderToolsResult, error) {
	governanceContext, err := s.governanceContext()
	if err != nil {
		return catalogapplication.SyncProviderToolsResult{}, err
	}
	return s.runtime.SyncConfiguredProvider(ctx, providerID, governanceContext)
}

func (s *Service) rebindProviderInstallation(
	ctx context.Context,
	providerID catalogdomain.ProviderID,
	installationID providerplugin.InstallationID,
	rawConfig json.RawMessage,
) (catalogapplication.SyncProviderToolsResult, error) {
	governanceContext, err := s.governanceContext()
	if err != nil {
		return catalogapplication.SyncProviderToolsResult{}, err
	}
	return s.runtime.RebindProviderInstallation(
		ctx,
		providerID,
		installationID,
		rawConfig,
		governanceContext,
	)
}

func (s *Service) startMCPHTTPServer(
	ctx context.Context,
	slug profiledomain.Slug,
	address string,
) (bootstrap.MCPServerInstance, error) {
	governanceContext, err := s.governanceContext()
	if err != nil {
		return bootstrap.MCPServerInstance{}, err
	}
	return s.runtime.StartMCPHTTPServer(ctx, slug, address, governanceContext)
}

func (s *Service) restartMCPServer(
	ctx context.Context,
	serverID string,
) (bootstrap.MCPServerInstance, error) {
	governanceContext, err := s.governanceContext()
	if err != nil {
		return bootstrap.MCPServerInstance{}, err
	}
	return s.runtime.RestartMCPServer(ctx, serverID, governanceContext)
}

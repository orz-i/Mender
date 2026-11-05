package application

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"

	catalogapplication "github.com/orz-i/Mender/backend/internal/catalog/application"
	catalogdomain "github.com/orz-i/Mender/backend/internal/catalog/domain"
	governancedomain "github.com/orz-i/Mender/backend/internal/governance/domain"
	providerplugin "github.com/orz-i/Mender/backend/internal/providerplugin"
)

func (s Service) UpdateConfiguredProvider(
	ctx context.Context,
	providerID catalogdomain.ProviderID,
	request catalogapplication.UpdateProviderRequest,
	rawConfig json.RawMessage,
	governanceContext governancedomain.Context,
) (catalogdomain.Provider, error) {
	if s.deps.SandboxOwnership != nil {
		managed, err := s.deps.SandboxOwnership.Execute(ctx, providerID)
		if err != nil {
			return catalogdomain.Provider{}, err
		}
		if managed {
			return catalogdomain.Provider{}, fmt.Errorf("%w: Provider belongs to a Sandbox Tool Suite", ErrProviderInUse)
		}
	}
	currentProvider, err := s.deps.Catalog.Get(ctx, providerID)
	if err != nil {
		return catalogdomain.Provider{}, err
	}
	if _, err := currentProvider.UpdateMetadata(
		request.DisplayName,
		request.Description,
		s.deps.Clock.Now(),
	); err != nil {
		return catalogdomain.Provider{}, err
	}

	currentConfig, err := s.deps.Configurations.GetProviderConfig(ctx, providerID)
	if err != nil {
		return catalogdomain.Provider{}, fmt.Errorf(
			"update configured provider: load config: %w",
			err,
		)
	}
	if s.deps.IsManagedFacade(currentConfig.PluginID) {
		return catalogdomain.Provider{}, fmt.Errorf("%w: Provider belongs to a resource-owned Tool Suite", ErrProviderInUse)
	}
	resolved, err := s.deps.Resolver.Resolve(ctx, currentConfig)
	if err != nil {
		return catalogdomain.Provider{}, fmt.Errorf(
			"update configured provider: resolve plugin: %w",
			err,
		)
	}
	nextConfig, err := updatedProviderConfiguration(
		providerID,
		currentConfig,
		resolved.Descriptor.ConfigVersion,
		rawConfig,
	)
	if err != nil {
		return catalogdomain.Provider{}, fmt.Errorf(
			"update configured provider: build config: %w",
			err,
		)
	}

	var updated catalogdomain.Provider
	activationErr := s.admitProviderActivation(
		ctx,
		providerID,
		governanceContext,
		func(operationCtx context.Context) error {
			if err := resolved.Plugin.ValidateConfig(
				operationCtx,
				cloneRawJSON(rawConfig),
			); err != nil {
				return fmt.Errorf(
					"update configured provider: validate config: %w: %w",
					providerplugin.ErrInvalidConfiguration,
					err,
				)
			}
			metadata, err := resolved.Plugin.InspectConfig(
				operationCtx,
				cloneRawJSON(rawConfig),
			)
			if err != nil {
				return fmt.Errorf(
					"update configured provider: inspect config: %w",
					err,
				)
			}
			nextConfigWithMetadata, err := nextConfig.WithMetadata(metadata)
			if err != nil {
				return fmt.Errorf(
					"update configured provider: normalize config metadata: %w",
					err,
				)
			}
			if err := s.deps.Configurations.Save(
				operationCtx,
				nextConfigWithMetadata,
			); err != nil {
				return fmt.Errorf(
					"update configured provider: save config: %w",
					err,
				)
			}

			request.ProviderID = providerID
			updated, err = s.deps.Catalog.Update(
				operationCtx,
				request,
			)
			if err != nil {
				restoreErr := s.deps.Configurations.Save(
					context.WithoutCancel(operationCtx),
					currentConfig,
				)
				return errors.Join(
					fmt.Errorf(
						"update configured provider: metadata: %w",
						err,
					),
					wrapOptionalError(
						"update configured provider: restore config",
						restoreErr,
					),
				)
			}
			return nil
		},
	)
	if activationErr != nil {
		return catalogdomain.Provider{}, activationErr
	}
	return updated, nil
}

func updatedProviderConfiguration(
	providerID catalogdomain.ProviderID,
	current providerplugin.Configuration,
	configVersion int,
	rawConfig json.RawMessage,
) (providerplugin.Configuration, error) {
	if current.InstallationID == nil {
		return providerplugin.NewConfiguration(
			providerID,
			current.PluginID,
			configVersion,
			rawConfig,
		)
	}
	return providerplugin.NewInstalledConfiguration(
		providerID,
		current.PluginID,
		*current.InstallationID,
		configVersion,
		rawConfig,
	)
}

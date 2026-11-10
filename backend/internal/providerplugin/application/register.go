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
	sandboxdomain "github.com/orz-i/Mender/backend/internal/sandbox/domain"
)

func (s Service) RegisterConfiguredProvider(
	ctx context.Context, request catalogapplication.RegisterProviderRequest,
	pluginID providerplugin.ID, rawConfig json.RawMessage,
	governanceContext governancedomain.Context,
) (catalogdomain.Provider, error) {
	if s.deps.IsRetiredProviderID != nil && s.deps.IsRetiredProviderID(pluginID) {
		return catalogdomain.Provider{}, sandboxdomain.ErrNotAuthorized
	}
	plugin, descriptor, err := s.resolveProviderPlugin(pluginID)
	if err != nil {
		return catalogdomain.Provider{}, fmt.Errorf(
			"register configured provider: %w",
			err,
		)
	}

	provider, err := s.deps.Catalog.Register(ctx, request)
	if err != nil {
		return catalogdomain.Provider{}, err
	}

	config, err := providerplugin.NewConfiguration(
		provider.ID(),
		descriptor.ID,
		descriptor.ConfigVersion,
		rawConfig,
	)
	if err != nil {
		removeErr := s.deps.Catalog.Remove(ctx, provider.ID())
		return catalogdomain.Provider{}, errors.Join(
			fmt.Errorf("register configured provider: build config: %w", err),
			wrapOptionalError(
				"register configured provider: compensate provider removal",
				removeErr,
			),
		)
	}

	activationErr := s.admitProviderActivation(
		ctx,
		provider.ID(),
		governanceContext,
		func(operationCtx context.Context) error {
			if err := plugin.ValidateConfig(
				operationCtx,
				cloneRawJSON(rawConfig),
			); err != nil {
				return fmt.Errorf(
					"register configured provider: validate config: %w: %w",
					providerplugin.ErrInvalidConfiguration,
					err,
				)
			}
			metadata, err := plugin.InspectConfig(
				operationCtx,
				cloneRawJSON(rawConfig),
			)
			if err != nil {
				return fmt.Errorf(
					"register configured provider: inspect config: %w",
					err,
				)
			}
			nextConfig, err := config.WithMetadata(metadata)
			if err != nil {
				return fmt.Errorf(
					"register configured provider: normalize config metadata: %w",
					err,
				)
			}
			if err := s.deps.Configurations.Save(operationCtx, nextConfig); err != nil {
				return fmt.Errorf(
					"register configured provider: save config: %w",
					err,
				)
			}
			return nil
		},
	)
	if activationErr != nil {
		removeErr := s.deps.Catalog.Remove(
			context.WithoutCancel(ctx),
			provider.ID(),
		)
		return catalogdomain.Provider{}, errors.Join(
			activationErr,
			wrapOptionalError(
				"register configured provider: compensate provider removal",
				removeErr,
			),
		)
	}
	return provider, nil
}

func (s Service) RegisterInstalledProvider(
	ctx context.Context,
	request catalogapplication.RegisterProviderRequest,
	pluginID providerplugin.ID,
	installationID providerplugin.InstallationID,
	rawConfig json.RawMessage,
	governanceContext governancedomain.Context,
) (catalogdomain.Provider, error) {
	if s.deps.IsRetiredProviderID != nil && s.deps.IsRetiredProviderID(pluginID) {
		return catalogdomain.Provider{}, sandboxdomain.ErrNotAuthorized
	}
	if s.deps.Installations == nil {
		return catalogdomain.Provider{}, fmt.Errorf(
			"register installed provider: external plugin resolver is unavailable",
		)
	}
	resolved, err := s.deps.Installations.ResolveInstallation(ctx, installationID)
	if err != nil {
		return catalogdomain.Provider{}, fmt.Errorf(
			"register installed provider: %w",
			err,
		)
	}
	if resolved.Descriptor.ID != pluginID {
		return catalogdomain.Provider{}, fmt.Errorf(
			"register installed provider: installation %s owns plugin %s, request uses %s",
			installationID,
			resolved.Descriptor.ID,
			pluginID,
		)
	}

	provider, err := s.deps.Catalog.Register(ctx, request)
	if err != nil {
		return catalogdomain.Provider{}, err
	}
	config, err := providerplugin.NewInstalledConfiguration(
		provider.ID(),
		resolved.Descriptor.ID,
		installationID,
		resolved.Descriptor.ConfigVersion,
		rawConfig,
	)
	if err != nil {
		removeErr := s.deps.Catalog.Remove(ctx, provider.ID())
		return catalogdomain.Provider{}, errors.Join(
			fmt.Errorf("register installed provider: build config: %w", err),
			wrapOptionalError(
				"register installed provider: compensate provider removal",
				removeErr,
			),
		)
	}

	activationErr := s.admitProviderActivation(
		ctx,
		provider.ID(),
		governanceContext,
		func(operationCtx context.Context) error {
			if err := resolved.Plugin.ValidateConfig(
				operationCtx,
				cloneRawJSON(rawConfig),
			); err != nil {
				return fmt.Errorf(
					"register installed provider: validate config: %w: %w",
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
					"register installed provider: inspect config: %w",
					err,
				)
			}
			nextConfig, err := config.WithMetadata(metadata)
			if err != nil {
				return fmt.Errorf(
					"register installed provider: normalize config metadata: %w",
					err,
				)
			}
			if err := s.deps.Configurations.Save(operationCtx, nextConfig); err != nil {
				return fmt.Errorf(
					"register installed provider: save config: %w",
					err,
				)
			}
			return nil
		},
	)
	if activationErr != nil {
		removeErr := s.deps.Catalog.Remove(
			context.WithoutCancel(ctx),
			provider.ID(),
		)
		return catalogdomain.Provider{}, errors.Join(
			activationErr,
			wrapOptionalError(
				"register installed provider: compensate provider removal",
				removeErr,
			),
		)
	}
	return provider, nil
}

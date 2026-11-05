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

func (s Service) RebindProviderInstallation(
	ctx context.Context,
	providerID catalogdomain.ProviderID,
	targetInstallationID providerplugin.InstallationID,
	rawConfig json.RawMessage,
	governanceContext governancedomain.Context,
) (catalogapplication.SyncProviderToolsResult, error) {
	var result catalogapplication.SyncProviderToolsResult
	err := s.admitProviderActivation(
		ctx,
		providerID,
		governanceContext,
		func(operationCtx context.Context) error {
			var operationErr error
			result, operationErr = s.rebindProviderInstallation(
				operationCtx,
				providerID,
				targetInstallationID,
				rawConfig,
			)
			return operationErr
		},
	)
	return result, err
}

func (s Service) rebindProviderInstallation(
	ctx context.Context,
	providerID catalogdomain.ProviderID,
	targetInstallationID providerplugin.InstallationID,
	rawConfig json.RawMessage,
) (catalogapplication.SyncProviderToolsResult, error) {
	if _, err := s.deps.Catalog.Get(ctx, providerID); err != nil {
		return catalogapplication.SyncProviderToolsResult{}, fmt.Errorf(
			"rebind provider installation: provider: %w",
			err,
		)
	}
	currentConfig, err := s.deps.Configurations.GetProviderConfig(ctx, providerID)
	if err != nil {
		return catalogapplication.SyncProviderToolsResult{}, fmt.Errorf(
			"rebind provider installation: load current config: %w",
			err,
		)
	}
	if currentConfig.InstallationID == nil {
		return catalogapplication.SyncProviderToolsResult{}, fmt.Errorf(
			"rebind provider installation: Provider %s uses a built-in plugin",
			providerID,
		)
	}
	if *currentConfig.InstallationID == targetInstallationID {
		return catalogapplication.SyncProviderToolsResult{}, fmt.Errorf(
			"rebind provider installation: Provider %s already uses installation %s",
			providerID,
			targetInstallationID,
		)
	}

	resolved, err := s.deps.Installations.ResolveInstallation(ctx, targetInstallationID)
	if err != nil {
		return catalogapplication.SyncProviderToolsResult{}, fmt.Errorf(
			"rebind provider installation: target: %w",
			err,
		)
	}
	if resolved.Descriptor.ID != currentConfig.PluginID {
		return catalogapplication.SyncProviderToolsResult{}, fmt.Errorf(
			"rebind provider installation: target plugin %s does not match Provider plugin %s",
			resolved.Descriptor.ID,
			currentConfig.PluginID,
		)
	}
	if err := resolved.Plugin.ValidateConfig(ctx, cloneRawJSON(rawConfig)); err != nil {
		return catalogapplication.SyncProviderToolsResult{}, fmt.Errorf(
			"rebind provider installation: validate config: %w: %w",
			providerplugin.ErrInvalidConfiguration,
			err,
		)
	}
	metadata, err := resolved.Plugin.InspectConfig(ctx, cloneRawJSON(rawConfig))
	if err != nil {
		return catalogapplication.SyncProviderToolsResult{}, fmt.Errorf(
			"rebind provider installation: inspect config: %w",
			err,
		)
	}
	targetConfig, err := providerplugin.NewInstalledConfiguration(
		providerID,
		currentConfig.PluginID,
		targetInstallationID,
		resolved.Descriptor.ConfigVersion,
		rawConfig,
	)
	if err != nil {
		return catalogapplication.SyncProviderToolsResult{}, fmt.Errorf(
			"rebind provider installation: build target config: %w",
			err,
		)
	}
	targetConfig, err = targetConfig.WithMetadata(metadata)
	if err != nil {
		return catalogapplication.SyncProviderToolsResult{}, fmt.Errorf(
			"rebind provider installation: normalize target metadata: %w",
			err,
		)
	}

	previousTools, err := s.deps.Catalog.Snapshot(ctx, providerID)
	if err != nil {
		return catalogapplication.SyncProviderToolsResult{}, fmt.Errorf(
			"rebind provider installation: snapshot current tools: %w",
			err,
		)
	}

	candidate, err := resolved.Plugin.Open(ctx, providerplugin.OpenRequest{
		ProviderID:    providerID,
		ConfigVersion: targetConfig.ConfigVersion,
		Config:        cloneRawJSON(targetConfig.Config),
		Credentials:   s.deps.Credentials,
	})
	if err != nil {
		return catalogapplication.SyncProviderToolsResult{}, fmt.Errorf(
			"rebind provider installation: open target: %w",
			err,
		)
	}
	if candidate == nil {
		return catalogapplication.SyncProviderToolsResult{}, fmt.Errorf(
			"rebind provider installation: target returned a nil instance",
		)
	}
	candidateClosed := false
	closeCandidate := func() error {
		if candidateClosed {
			return nil
		}
		candidateClosed = true
		return candidate.Close()
	}
	defer func() { _ = closeCandidate() }()

	result, err := s.deps.Catalog.Sync(ctx, providerID, candidate)
	if err != nil {
		return catalogapplication.SyncProviderToolsResult{}, fmt.Errorf(
			"rebind provider installation: synchronize target: %w",
			err,
		)
	}
	if err := closeCandidate(); err != nil {
		restoreErr := s.deps.Catalog.Restore(
			context.WithoutCancel(ctx),
			providerID,
			previousTools,
		)
		return catalogapplication.SyncProviderToolsResult{}, errors.Join(
			fmt.Errorf("rebind provider installation: close prepared target: %w", err),
			wrapOptionalError(
				"rebind provider installation: restore Tool snapshot",
				restoreErr,
			),
		)
	}

	if err := s.deps.Configurations.Save(ctx, targetConfig); err != nil {
		restoreErr := s.deps.Catalog.Restore(
			context.WithoutCancel(ctx),
			providerID,
			previousTools,
		)
		return catalogapplication.SyncProviderToolsResult{}, errors.Join(
			fmt.Errorf("rebind provider installation: save target config: %w", err),
			wrapOptionalError(
				"rebind provider installation: restore Tool snapshot",
				restoreErr,
			),
		)
	}

	if _, err := s.deps.Instances.Reload(ctx, providerID); err != nil {
		restoreConfigErr := s.deps.Configurations.Save(
			context.WithoutCancel(ctx),
			currentConfig,
		)
		restoreToolsErr := s.deps.Catalog.Restore(
			context.WithoutCancel(ctx),
			providerID,
			previousTools,
		)
		var restoreRouteErr error
		if restoreConfigErr == nil {
			_, restoreRouteErr = s.deps.Instances.Reload(
				context.WithoutCancel(ctx),
				providerID,
			)
		}
		return catalogapplication.SyncProviderToolsResult{}, errors.Join(
			fmt.Errorf("rebind provider installation: activate target: %w", err),
			wrapOptionalError(
				"rebind provider installation: restore config",
				restoreConfigErr,
			),
			wrapOptionalError(
				"rebind provider installation: restore Tool snapshot",
				restoreToolsErr,
			),
			wrapOptionalError(
				"rebind provider installation: restore execution route",
				restoreRouteErr,
			),
		)
	}
	return result, nil
}

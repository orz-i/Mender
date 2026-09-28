package bootstrap

import (
	"context"
	"errors"
	"fmt"

	providerplugin "github.com/orz-i/Mender/backend/internal/providerplugin"
	installationapplication "github.com/orz-i/Mender/backend/internal/providerplugin/installation/application"
	installationdomain "github.com/orz-i/Mender/backend/internal/providerplugin/installation/domain"
	installationidentity "github.com/orz-i/Mender/backend/internal/providerplugin/installation/infrastructure/identity"
)

// InstallProviderPlugin installs, verifies, and persists one local external plugin package.
func (r *Runtime) InstallProviderPlugin(
	ctx context.Context,
	sourcePath string,
) (installationdomain.Installation, error) {
	if r == nil {
		return installationdomain.Installation{}, fmt.Errorf(
			"install provider plugin: runtime is nil",
		)
	}
	usecase := installationapplication.NewInstallLocal(
		r.pluginInstallations,
		r.pluginPackages,
		installationidentity.NewGenerator(),
		systemClock{},
	)
	installation, err := usecase.Execute(ctx, sourcePath)
	if err != nil {
		return installationdomain.Installation{}, err
	}

	if _, err := r.externalResolver.ResolveInstallation(
		ctx,
		installation.ID(),
	); err != nil {
		processErr := r.externalProcesses.CloseInstallation(installation.ID())
		removeErr := installationapplication.NewRemoveInstallation(
			r.pluginInstallations,
			r.pluginPackages,
			nil,
		).Execute(context.WithoutCancel(ctx), installation.ID())
		return installationdomain.Installation{}, errors.Join(
			fmt.Errorf("install provider plugin: verify executable: %w", err),
			wrapOptionalError(
				"install provider plugin: close verification process",
				processErr,
			),
			wrapOptionalError(
				"install provider plugin: compensate invalid installation",
				removeErr,
			),
		)
	}
	if err := r.externalProcesses.CloseInstallation(installation.ID()); err != nil {
		return installationdomain.Installation{}, fmt.Errorf(
			"install provider plugin: close verification process: %w",
			err,
		)
	}
	return installation, nil
}

// GetProviderPluginInstallation returns one persisted external plugin package.
func (r *Runtime) GetProviderPluginInstallation(
	ctx context.Context,
	id providerplugin.InstallationID,
) (installationdomain.Installation, error) {
	if r == nil {
		return installationdomain.Installation{}, fmt.Errorf(
			"get provider plugin installation: runtime is nil",
		)
	}
	item, err := r.pluginInstallations.Get(ctx, id)
	if err != nil {
		return installationdomain.Installation{}, fmt.Errorf(
			"get provider plugin installation: %w",
			err,
		)
	}
	return item, nil
}

// ListProviderPluginInstallations returns persisted external plugin packages.
func (r *Runtime) ListProviderPluginInstallations(
	ctx context.Context,
) ([]installationdomain.Installation, error) {
	if r == nil {
		return nil, fmt.Errorf("list provider plugin installations: runtime is nil")
	}
	return installationapplication.NewListInstallations(
		r.pluginInstallations,
	).Execute(ctx)
}

// SetProviderPluginInstallationStatus changes administrative availability.
// Disabling or invalidating an installation first persists the fail-closed
// state, then tears down all active Provider routes/sessions and the package process.
func (r *Runtime) SetProviderPluginInstallationStatus(
	ctx context.Context,
	id providerplugin.InstallationID,
	status installationdomain.Status,
) (installationdomain.Installation, error) {
	if r == nil {
		return installationdomain.Installation{}, fmt.Errorf(
			"set provider plugin installation status: runtime is nil",
		)
	}
	var configs []providerplugin.Configuration
	if status != installationdomain.StatusEnabled {
		var err error
		configs, err = r.providerConfigs.List(ctx)
		if err != nil {
			return installationdomain.Installation{}, fmt.Errorf(
				"set provider plugin installation status: list Provider configs: %w",
				err,
			)
		}
	}

	item, err := installationapplication.NewSetInstallationStatus(
		r.pluginInstallations,
	).Execute(ctx, id, status)
	if err != nil {
		return installationdomain.Installation{}, err
	}
	if status == installationdomain.StatusEnabled {
		return item, nil
	}
	var closeErrors []error
	for _, config := range configs {
		if config.InstallationID == nil || *config.InstallationID != id {
			continue
		}
		if err := r.providerInstances.CloseProvider(config.ProviderID); err != nil {
			closeErrors = append(
				closeErrors,
				fmt.Errorf("close Provider %s: %w", config.ProviderID, err),
			)
		}
	}
	if err := r.externalProcesses.CloseInstallation(id); err != nil {
		closeErrors = append(
			closeErrors,
			fmt.Errorf("close external plugin process: %w", err),
		)
	}
	if len(closeErrors) != 0 {
		return item, fmt.Errorf(
			"set provider plugin installation status: %w",
			errors.Join(closeErrors...),
		)
	}
	return item, nil
}

// RemoveProviderPluginInstallation removes one unreferenced external package.
func (r *Runtime) RemoveProviderPluginInstallation(
	ctx context.Context,
	id providerplugin.InstallationID,
) error {
	if r == nil {
		return fmt.Errorf("remove provider plugin installation: runtime is nil")
	}
	usage := runtimePluginInstallationUsageChecker{configs: r.providerConfigs}
	usages, err := usage.Usages(ctx, id)
	if err != nil {
		return fmt.Errorf(
			"remove provider plugin installation: check usage: %w",
			err,
		)
	}
	if len(usages) != 0 {
		return fmt.Errorf(
			"%w: %s",
			installationapplication.ErrInstallationInUse,
			usages[0].Description,
		)
	}
	if err := r.externalProcesses.CloseInstallation(id); err != nil {
		return fmt.Errorf(
			"remove provider plugin installation: close process: %w",
			err,
		)
	}
	return installationapplication.NewRemoveInstallation(
		r.pluginInstallations,
		r.pluginPackages,
		usage,
	).Execute(ctx, id)
}

type runtimePluginInstallationUsageChecker struct {
	configs interface {
		List(context.Context) ([]providerplugin.Configuration, error)
	}
}

func (c runtimePluginInstallationUsageChecker) Usages(
	ctx context.Context,
	id providerplugin.InstallationID,
) ([]installationapplication.Usage, error) {
	configs, err := c.configs.List(ctx)
	if err != nil {
		return nil, err
	}
	usages := make([]installationapplication.Usage, 0)
	for _, config := range configs {
		if config.InstallationID == nil || *config.InstallationID != id {
			continue
		}
		usages = append(usages, installationapplication.Usage{
			Kind:        "provider",
			Description: fmt.Sprintf("provider %s", config.ProviderID),
		})
	}
	return usages, nil
}

package application

import (
	"context"
	"errors"
	"fmt"

	providerplugin "github.com/orz-i/Mender/backend/internal/providerplugin"
	installationdomain "github.com/orz-i/Mender/backend/internal/providerplugin/installation/domain"
)

type InstallLocal struct {
	repository Repository
	packages   PackageStore
	ids        IDGenerator
	clock      Clock
}

func NewInstallLocal(
	repository Repository,
	packages PackageStore,
	ids IDGenerator,
	clock Clock,
) InstallLocal {
	return InstallLocal{
		repository: repository,
		packages:   packages,
		ids:        ids,
		clock:      clock,
	}
}

func (uc InstallLocal) Execute(
	ctx context.Context,
	sourcePath string,
) (installationdomain.Installation, error) {
	id, err := uc.ids.NewInstallationID()
	if err != nil {
		return installationdomain.Installation{}, fmt.Errorf("install provider plugin: allocate id: %w", err)
	}

	pkg, err := uc.packages.InstallLocal(ctx, sourcePath, id)
	if err != nil {
		return installationdomain.Installation{}, fmt.Errorf("install provider plugin: package: %w", err)
	}

	cleanup := func() error {
		return uc.packages.Remove(context.WithoutCancel(ctx), id)
	}

	pluginID, err := providerplugin.ParseID(pkg.Manifest.PluginID)
	if err != nil {
		return installationdomain.Installation{}, errors.Join(
			fmt.Errorf("install provider plugin: plugin id: %w", err),
			wrapCleanupError(cleanup()),
		)
	}
	installation, err := installationdomain.NewInstallation(
		id,
		pluginID,
		pkg.Manifest.Version,
		installationdomain.SourceLocal,
		pkg.SourceReference,
		pkg.Digest,
		pkg.ManifestJSON,
		installationdomain.StatusEnabled,
		uc.clock.Now(),
	)
	if err != nil {
		return installationdomain.Installation{}, errors.Join(
			fmt.Errorf("install provider plugin: metadata: %w", err),
			wrapCleanupError(cleanup()),
		)
	}

	if existing, lookupErr := uc.repository.GetByDigest(ctx, installation.Digest()); lookupErr == nil {
		return installationdomain.Installation{}, errors.Join(
			fmt.Errorf("%w: %s", ErrPackageAlreadyInstalled, existing.ID()),
			wrapCleanupError(cleanup()),
		)
	} else if !errors.Is(lookupErr, ErrInstallationNotFound) {
		return installationdomain.Installation{}, errors.Join(
			fmt.Errorf("install provider plugin: check digest: %w", lookupErr),
			wrapCleanupError(cleanup()),
		)
	}

	if err := uc.repository.Create(ctx, installation); err != nil {
		return installationdomain.Installation{}, errors.Join(
			fmt.Errorf("install provider plugin: persist: %w", err),
			wrapCleanupError(cleanup()),
		)
	}
	return installation, nil
}

type ListInstallations struct {
	repository Repository
}

func NewListInstallations(repository Repository) ListInstallations {
	return ListInstallations{repository: repository}
}

func (uc ListInstallations) Execute(
	ctx context.Context,
) ([]installationdomain.Installation, error) {
	items, err := uc.repository.List(ctx)
	if err != nil {
		return nil, fmt.Errorf("list provider plugin installations: %w", err)
	}
	return items, nil
}

type SetInstallationStatus struct {
	repository Repository
}

func NewSetInstallationStatus(repository Repository) SetInstallationStatus {
	return SetInstallationStatus{repository: repository}
}

func (uc SetInstallationStatus) Execute(
	ctx context.Context,
	id installationdomain.InstallationID,
	status installationdomain.Status,
) (installationdomain.Installation, error) {
	if err := uc.repository.SetStatus(ctx, id, status); err != nil {
		return installationdomain.Installation{}, fmt.Errorf(
			"set provider plugin installation status: %w",
			err,
		)
	}
	item, err := uc.repository.Get(ctx, id)
	if err != nil {
		return installationdomain.Installation{}, fmt.Errorf(
			"set provider plugin installation status: reload: %w",
			err,
		)
	}
	return item, nil
}

type RemoveInstallation struct {
	repository Repository
	packages   PackageStore
	usage      UsageChecker
}

func NewRemoveInstallation(
	repository Repository,
	packages PackageStore,
	usage UsageChecker,
) RemoveInstallation {
	return RemoveInstallation{
		repository: repository,
		packages:   packages,
		usage:      usage,
	}
}

func (uc RemoveInstallation) Execute(
	ctx context.Context,
	id installationdomain.InstallationID,
) error {
	if _, err := uc.repository.Get(ctx, id); err != nil {
		return fmt.Errorf("remove provider plugin installation: %w", err)
	}
	if uc.usage != nil {
		usages, err := uc.usage.Usages(ctx, id)
		if err != nil {
			return fmt.Errorf("remove provider plugin installation: check usage: %w", err)
		}
		if len(usages) != 0 {
			return fmt.Errorf(
				"%w: %s",
				ErrInstallationInUse,
				usages[0].Description,
			)
		}
	}

	// Metadata is authoritative. Delete it first so a partial filesystem
	// cleanup can only leave an inert orphan package, never a launchable
	// installation whose managed files are missing.
	if err := uc.repository.Delete(ctx, id); err != nil {
		return fmt.Errorf("remove provider plugin installation: delete metadata: %w", err)
	}
	if err := uc.packages.Remove(context.WithoutCancel(ctx), id); err != nil {
		return fmt.Errorf("remove provider plugin installation: remove package files: %w", err)
	}
	return nil
}

func wrapCleanupError(err error) error {
	if err == nil {
		return nil
	}
	return fmt.Errorf("install provider plugin: compensate package files: %w", err)
}

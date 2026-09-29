package application

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"

	skilldomain "github.com/orz-i/Mender/backend/internal/skill/domain"
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
) (skilldomain.Installation, error) {
	id, err := uc.ids.NewInstallationID()
	if err != nil {
		return skilldomain.Installation{}, fmt.Errorf("install skill: allocate id: %w", err)
	}
	pkg, err := uc.packages.InstallLocal(ctx, sourcePath, id)
	if err != nil {
		return skilldomain.Installation{}, fmt.Errorf("install skill: package: %w", err)
	}
	cleanup := func() error {
		return uc.packages.Remove(context.WithoutCancel(ctx), id)
	}

	skillID, err := deriveSkillID(
		skilldomain.SourceLocalDirectory,
		pkg.SourceReference,
		pkg.Snapshot.Metadata.Name,
	)
	if err != nil {
		return skilldomain.Installation{}, errors.Join(
			fmt.Errorf("install skill: derive skill id: %w", err),
			wrapCleanupError(cleanup()),
		)
	}
	if existing, lookupErr := uc.repository.GetBySkillDigest(
		ctx,
		skillID,
		pkg.Snapshot.Digest,
	); lookupErr == nil {
		return skilldomain.Installation{}, errors.Join(
			fmt.Errorf("%w: %s", ErrPackageAlreadyInstalled, existing.ID()),
			wrapCleanupError(cleanup()),
		)
	} else if !errors.Is(lookupErr, ErrInstallationNotFound) {
		return skilldomain.Installation{}, errors.Join(
			fmt.Errorf("install skill: check existing package: %w", lookupErr),
			wrapCleanupError(cleanup()),
		)
	}

	installation, err := skilldomain.NewInstallation(
		id,
		skillID,
		pkg.Snapshot.Metadata.Name,
		pkg.Snapshot.Metadata.Description,
		skilldomain.SourceLocalDirectory,
		pkg.SourceReference,
		pkg.Snapshot.Digest,
		pkg.Snapshot.Metadata.Values["version"],
		false,
		false,
		uc.clock.Now(),
	)
	if err != nil {
		return skilldomain.Installation{}, errors.Join(
			fmt.Errorf("install skill: metadata: %w", err),
			wrapCleanupError(cleanup()),
		)
	}
	if err := uc.repository.Create(ctx, installation); err != nil {
		return skilldomain.Installation{}, errors.Join(
			fmt.Errorf("install skill: persist: %w", err),
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

func (uc ListInstallations) Execute(ctx context.Context) ([]skilldomain.Installation, error) {
	items, err := uc.repository.List(ctx)
	if err != nil {
		return nil, fmt.Errorf("list skill installations: %w", err)
	}
	return items, nil
}

type GetInstallation struct {
	repository Repository
}

func NewGetInstallation(repository Repository) GetInstallation {
	return GetInstallation{repository: repository}
}

func (uc GetInstallation) Execute(
	ctx context.Context,
	id skilldomain.InstallationID,
) (skilldomain.Installation, error) {
	item, err := uc.repository.Get(ctx, id)
	if err != nil {
		return skilldomain.Installation{}, fmt.Errorf("get skill installation: %w", err)
	}
	return item, nil
}

type Inspection struct {
	Installation skilldomain.Installation
	Snapshot     skilldomain.Snapshot
}

type InspectInstallation struct {
	repository Repository
	packages   PackageStore
}

func NewInspectInstallation(
	repository Repository,
	packages PackageStore,
) InspectInstallation {
	return InspectInstallation{
		repository: repository,
		packages:   packages,
	}
}

func (uc InspectInstallation) Execute(
	ctx context.Context,
	id skilldomain.InstallationID,
) (Inspection, error) {
	item, err := uc.repository.Get(ctx, id)
	if err != nil {
		return Inspection{}, fmt.Errorf("inspect skill installation: metadata: %w", err)
	}
	snapshot, err := uc.packages.Resolve(ctx, item)
	if err != nil {
		return Inspection{}, fmt.Errorf("inspect skill installation: package: %w", err)
	}
	return Inspection{
		Installation: item,
		Snapshot:     snapshot,
	}, nil
}

type SetTrust struct {
	repository Repository
}

func NewSetTrust(repository Repository) SetTrust {
	return SetTrust{repository: repository}
}

func (uc SetTrust) Execute(
	ctx context.Context,
	id skilldomain.InstallationID,
	trusted bool,
) (skilldomain.Installation, error) {
	item, err := uc.repository.Get(ctx, id)
	if err != nil {
		return skilldomain.Installation{}, fmt.Errorf("set skill trust: %w", err)
	}
	item = item.WithTrusted(trusted)
	if err := uc.repository.SetState(ctx, id, item.Trusted(), item.Enabled()); err != nil {
		return skilldomain.Installation{}, fmt.Errorf("set skill trust: persist: %w", err)
	}
	return item, nil
}

type SetEnabled struct {
	repository Repository
}

func NewSetEnabled(repository Repository) SetEnabled {
	return SetEnabled{repository: repository}
}

func (uc SetEnabled) Execute(
	ctx context.Context,
	id skilldomain.InstallationID,
	enabled bool,
) (skilldomain.Installation, error) {
	item, err := uc.repository.Get(ctx, id)
	if err != nil {
		return skilldomain.Installation{}, fmt.Errorf("set skill enablement: %w", err)
	}
	item, err = item.WithEnabled(enabled)
	if err != nil {
		return skilldomain.Installation{}, fmt.Errorf("set skill enablement: %w", err)
	}
	if err := uc.repository.SetState(ctx, id, item.Trusted(), item.Enabled()); err != nil {
		return skilldomain.Installation{}, fmt.Errorf("set skill enablement: persist: %w", err)
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
	id skilldomain.InstallationID,
) error {
	if _, err := uc.repository.Get(ctx, id); err != nil {
		return fmt.Errorf("remove skill installation: %w", err)
	}
	if uc.usage != nil {
		usages, err := uc.usage.Usages(ctx, id)
		if err != nil {
			return fmt.Errorf("remove skill installation: check usage: %w", err)
		}
		if len(usages) != 0 {
			return fmt.Errorf("%w: %s", ErrInstallationInUse, usages[0].Description)
		}
	}

	if err := uc.repository.Delete(ctx, id); err != nil {
		return fmt.Errorf("remove skill installation: delete metadata: %w", err)
	}
	if err := uc.packages.Remove(context.WithoutCancel(ctx), id); err != nil {
		return fmt.Errorf("remove skill installation: remove package files: %w", err)
	}
	return nil
}

func deriveSkillID(
	sourceKind skilldomain.SourceKind,
	sourceReference string,
	name string,
) (skilldomain.SkillID, error) {
	hash := sha256.New()
	_, _ = hash.Write([]byte("mender-skill-lineage-v1\x00"))
	_, _ = hash.Write([]byte(sourceKind))
	_, _ = hash.Write([]byte{0})
	_, _ = hash.Write([]byte(sourceReference))
	_, _ = hash.Write([]byte{0})
	_, _ = hash.Write([]byte(name))
	sum := hash.Sum(nil)
	return skilldomain.ParseSkillID("skill_" + hex.EncodeToString(sum[:16]))
}

func wrapCleanupError(err error) error {
	if err == nil {
		return nil
	}
	return fmt.Errorf("install skill: compensate package files: %w", err)
}

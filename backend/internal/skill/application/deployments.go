package application

import (
	"context"
	"errors"
	"fmt"

	skilldomain "github.com/orz-i/Mender/backend/internal/skill/domain"
)

// DeployInstallation materializes one trusted+enabled exact Skill installation
// into an explicit Agent host target and persists the exact deployment record.
type DeployInstallation struct {
	source     DeploymentSource
	repository DeploymentRepository
	filesystem DeploymentFilesystem
	ids        DeploymentIDGenerator
	clock      Clock
}

func NewDeployInstallation(
	source DeploymentSource,
	repository DeploymentRepository,
	filesystem DeploymentFilesystem,
	ids DeploymentIDGenerator,
	clock Clock,
) DeployInstallation {
	return DeployInstallation{
		source:     source,
		repository: repository,
		filesystem: filesystem,
		ids:        ids,
		clock:      clock,
	}
}

func (uc DeployInstallation) Execute(
	ctx context.Context,
	installationID skilldomain.InstallationID,
	target skilldomain.DeploymentTarget,
	root string,
) (skilldomain.Deployment, error) {
	item, err := uc.source.GetDeployable(ctx, installationID)
	if err != nil {
		return skilldomain.Deployment{}, fmt.Errorf("deploy skill: resolve installation: %w", err)
	}
	if !item.Trusted {
		return skilldomain.Deployment{}, ErrInstallationUntrusted
	}
	if !item.Enabled {
		return skilldomain.Deployment{}, ErrInstallationDisabled
	}
	if err := target.Validate(); err != nil {
		return skilldomain.Deployment{}, err
	}
	id, err := uc.ids.NewDeploymentID()
	if err != nil {
		return skilldomain.Deployment{}, fmt.Errorf("deploy skill: allocate id: %w", err)
	}
	destination, err := uc.filesystem.ResolveDestination(target, root, item.Name)
	if err != nil {
		return skilldomain.Deployment{}, fmt.Errorf("deploy skill: destination: %w", err)
	}
	if err := uc.source.Materialize(ctx, installationID, destination); err != nil {
		return skilldomain.Deployment{}, fmt.Errorf("deploy skill: materialize: %w", err)
	}
	cleanup := func() error {
		return uc.filesystem.Remove(context.WithoutCancel(ctx), root, destination)
	}
	deployment, err := skilldomain.NewDeployment(
		id,
		installationID,
		target,
		root,
		destination,
		item.Digest,
		uc.clock.Now(),
	)
	if err != nil {
		return skilldomain.Deployment{}, errors.Join(
			fmt.Errorf("deploy skill: metadata: %w", err),
			wrapDeploymentCleanupError(cleanup()),
		)
	}
	if err := uc.repository.CreateDeployment(ctx, deployment); err != nil {
		return skilldomain.Deployment{}, errors.Join(
			fmt.Errorf("deploy skill: persist: %w", err),
			wrapDeploymentCleanupError(cleanup()),
		)
	}
	return deployment, nil
}

// DeploymentInspection reports one persisted deployment plus current drift.
type DeploymentInspection struct {
	Deployment    skilldomain.Deployment
	Drift         skilldomain.DeploymentDrift
	CurrentDigest string
}

// InspectDeployment compares the target's current Skill snapshot with the exact
// persisted deployment digest.
type InspectDeployment struct {
	repository DeploymentRepository
	filesystem DeploymentFilesystem
}

func NewInspectDeployment(
	repository DeploymentRepository,
	filesystem DeploymentFilesystem,
) InspectDeployment {
	return InspectDeployment{
		repository: repository,
		filesystem: filesystem,
	}
}

func (uc InspectDeployment) Execute(
	ctx context.Context,
	id skilldomain.DeploymentID,
) (DeploymentInspection, error) {
	deployment, err := uc.repository.GetDeployment(ctx, id)
	if err != nil {
		return DeploymentInspection{}, fmt.Errorf("inspect skill deployment: %w", err)
	}
	exists, digest, err := uc.filesystem.Observe(ctx, deployment.Destination())
	if err != nil {
		return DeploymentInspection{}, fmt.Errorf(
			"inspect skill deployment: observe target: %w",
			err,
		)
	}
	drift := skilldomain.DeploymentMissing
	if exists {
		drift = skilldomain.DeploymentDrifted
		if digest == deployment.Digest() {
			drift = skilldomain.DeploymentCurrent
		}
	}
	return DeploymentInspection{
		Deployment:    deployment,
		Drift:         drift,
		CurrentDigest: digest,
	}, nil
}

// ListDeployments returns persisted exact deployment records.
type ListDeployments struct {
	repository DeploymentRepository
	filesystem DeploymentFilesystem
}

func NewListDeployments(
	repository DeploymentRepository,
	filesystem DeploymentFilesystem,
) ListDeployments {
	return ListDeployments{
		repository: repository,
		filesystem: filesystem,
	}
}

func (uc ListDeployments) Execute(
	ctx context.Context,
) ([]DeploymentInspection, error) {
	items, err := uc.repository.ListDeployments(ctx)
	if err != nil {
		return nil, fmt.Errorf("list skill deployments: %w", err)
	}
	result := make([]DeploymentInspection, 0, len(items))
	for _, item := range items {
		exists, digest, observeErr := uc.filesystem.Observe(ctx, item.Destination())
		if observeErr != nil {
			return nil, fmt.Errorf(
				"list skill deployments: observe %s: %w",
				item.ID(),
				observeErr,
			)
		}
		drift := skilldomain.DeploymentMissing
		if exists {
			drift = skilldomain.DeploymentDrifted
			if digest == item.Digest() {
				drift = skilldomain.DeploymentCurrent
			}
		}
		result = append(result, DeploymentInspection{
			Deployment:    item,
			Drift:         drift,
			CurrentDigest: digest,
		})
	}
	return result, nil
}

// RemoveDeployment removes materialized content then its metadata record.
type RemoveDeployment struct {
	repository DeploymentRepository
	filesystem DeploymentFilesystem
}

func NewRemoveDeployment(
	repository DeploymentRepository,
	filesystem DeploymentFilesystem,
) RemoveDeployment {
	return RemoveDeployment{
		repository: repository,
		filesystem: filesystem,
	}
}

func (uc RemoveDeployment) Execute(
	ctx context.Context,
	id skilldomain.DeploymentID,
) error {
	item, err := uc.repository.GetDeployment(ctx, id)
	if err != nil {
		return fmt.Errorf("remove skill deployment: %w", err)
	}
	if err := uc.filesystem.Remove(ctx, item.Root(), item.Destination()); err != nil {
		return fmt.Errorf("remove skill deployment: target: %w", err)
	}
	if err := uc.repository.DeleteDeployment(ctx, id); err != nil {
		return fmt.Errorf("remove skill deployment: metadata: %w", err)
	}
	return nil
}

// DeploymentUsageChecker blocks exact installation removal while deployments
// still reference it.
type DeploymentUsageChecker struct {
	repository DeploymentRepository
}

func NewDeploymentUsageChecker(
	repository DeploymentRepository,
) DeploymentUsageChecker {
	return DeploymentUsageChecker{repository: repository}
}

func (c DeploymentUsageChecker) Usages(
	ctx context.Context,
	id skilldomain.InstallationID,
) ([]Usage, error) {
	items, err := c.repository.ListDeploymentsByInstallation(ctx, id)
	if err != nil {
		return nil, err
	}
	result := make([]Usage, 0, len(items))
	for _, item := range items {
		result = append(result, Usage{
			Kind: "deployment",
			Description: fmt.Sprintf(
				"deployment %s at %s",
				item.ID(),
				item.Destination(),
			),
		})
	}
	return result, nil
}

func wrapDeploymentCleanupError(err error) error {
	if err == nil {
		return nil
	}
	return fmt.Errorf("deploy skill: compensate target: %w", err)
}

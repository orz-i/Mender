package application

import (
	"context"
	"errors"
	"fmt"

	platformplugin "github.com/orz-i/Mender/backend/internal/plugin"
	installationdomain "github.com/orz-i/Mender/backend/internal/plugin/installation/domain"
	"github.com/orz-i/Mender/backend/internal/providerplugin"
)

type DescriptorInstallations interface {
	Get(context.Context, providerplugin.InstallationID) (installationdomain.Installation, error)
	SetVerifiedDescriptor(context.Context, providerplugin.InstallationID, platformplugin.Descriptor) error
}

// DescriptorProbe is explicitly executable installation verification. Each
// invocation owns a private probe/process host; it must not borrow runtime
// processes. Normal catalog reads never consume this capability.
type DescriptorProbe interface {
	ProbeInstallation(context.Context, providerplugin.InstallationID) (providerplugin.Resolution, error)
}

type InstallationProcesses interface {
	CloseInstallation(providerplugin.InstallationID) error
}

type InstallationCache interface {
	ForgetInstallation(providerplugin.InstallationID)
}

type VerifyDescriptor struct {
	installations DescriptorInstallations
	probe         DescriptorProbe
	processes     InstallationProcesses
	cache         InstallationCache
}

func NewVerifyDescriptor(installations DescriptorInstallations, probe DescriptorProbe,
	processes InstallationProcesses, cache InstallationCache,
) VerifyDescriptor {
	return VerifyDescriptor{installations: installations, probe: probe, processes: processes, cache: cache}
}

// Execute repairs only a missing descriptor snapshot for one exact enabled
// Provider package. Already verified packages remain offline and untouched;
// this operation cannot replace snapshots or restart active Provider instances.
func (uc VerifyDescriptor) Execute(ctx context.Context, id providerplugin.InstallationID) (installationdomain.Installation, error) {
	item, err := uc.installations.Get(ctx, id)
	if err != nil {
		return item, fmt.Errorf("verify provider plugin descriptor: installation: %w", err)
	}
	if item.Extension().Kind != platformplugin.ProviderTools {
		return item, fmt.Errorf("verify provider plugin descriptor: extension is not provider.tools")
	}
	if _, verified := item.VerifiedDescriptor(); verified {
		return item, nil
	}
	if item.Status() != installationdomain.StatusEnabled {
		return item, fmt.Errorf("verify provider plugin descriptor: installation is %s", item.Status())
	}
	// Close even a failed probe, and finish cleanup BEFORE publishing a
	// snapshot that can admit concurrent Provider registration or invocation.
	resolved, probeErr := uc.probe.ProbeInstallation(ctx, id)
	closeErr := uc.processes.CloseInstallation(id)
	if probeErr != nil || closeErr != nil {
		return item, errors.Join(
			wrapOptionalError("verify provider plugin descriptor: probe", probeErr),
			wrapOptionalError("verify provider plugin descriptor: close verification process", closeErr),
		)
	}
	descriptor, err := providerplugin.PlatformDescriptor(resolved.Descriptor)
	if err != nil {
		return item, fmt.Errorf("verify provider plugin descriptor: descriptor: %w", err)
	}
	updated, err := item.WithVerifiedDescriptor(descriptor)
	if err != nil {
		return item, fmt.Errorf("verify provider plugin descriptor: installation descriptor: %w", err)
	}
	if err := uc.installations.SetVerifiedDescriptor(ctx, id, descriptor); err != nil {
		return item, fmt.Errorf("verify provider plugin descriptor: persist snapshot: %w", err)
	}
	if uc.cache != nil {
		uc.cache.ForgetInstallation(id)
	}
	return updated, nil
}

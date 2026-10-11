package bootstrap

import (
	"context"
	"errors"
	"fmt"

	platformplugin "github.com/orz-i/Mender/backend/internal/plugin"
	installationapp "github.com/orz-i/Mender/backend/internal/plugin/installation/application"
	installationdomain "github.com/orz-i/Mender/backend/internal/plugin/installation/domain"
	installationidentity "github.com/orz-i/Mender/backend/internal/plugin/installation/infrastructure/identity"
	tunnelexternal "github.com/orz-i/Mender/backend/internal/tunnel/infrastructure/external"
)

// InstallTunnelPlugin verifies the executable using the shared Plugin Core,
// pins its typed descriptor, then closes the descriptor-only probe process.
// Neither a Tunnel session nor a public MCP listener is created by install.
func (r *Runtime) InstallTunnelPlugin(ctx context.Context, sourcePath string) (installationdomain.Installation, error) {
	if r == nil || r.pluginInstallations == nil || r.externalProcesses == nil {
		return installationdomain.Installation{}, fmt.Errorf("tunnel package installation unavailable")
	}
	usecase := installationapp.NewInstallLocal(r.pluginInstallations, r.pluginPackages, installationidentity.NewGenerator(), systemClock{})
	item, err := usecase.Execute(ctx, sourcePath)
	if err != nil {
		return installationdomain.Installation{}, err
	}
	cleanup := func(opErr error) (installationdomain.Installation, error) {
		closeErr := r.externalProcesses.CloseInstallation(item.ID())
		removeErr := installationapp.NewRemoveInstallation(r.pluginInstallations, r.pluginPackages, nil).
			Execute(context.WithoutCancel(ctx), item.ID())
		return installationdomain.Installation{}, errors.Join(opErr, closeErr, removeErr)
	}
	if item.Extension().Kind != platformplugin.TunnelTransport || item.Extension().ID != "main" {
		return cleanup(fmt.Errorf("tunnel package is not tunnel.transport/main"))
	}
	raw, err := r.externalProcesses.ProcessHost().Ensure(ctx, item)
	if err != nil {
		return cleanup(fmt.Errorf("tunnel package executable: %w", err))
	}
	process, ok := raw.(*tunnelexternal.Process)
	if !ok {
		return cleanup(fmt.Errorf("tunnel process has mismatched extension type"))
	}
	descriptor := process.Descriptor()
	if err := r.pluginInstallations.SetVerifiedDescriptor(ctx, item.ID(), descriptor); err != nil {
		return cleanup(fmt.Errorf("persist Tunnel descriptor: %w", err))
	}
	item, err = item.WithVerifiedDescriptor(descriptor)
	if err != nil {
		return cleanup(err)
	}
	if err := r.externalProcesses.CloseInstallation(item.ID()); err != nil {
		return cleanup(fmt.Errorf("close Tunnel descriptor process: %w", err))
	}
	return item, nil
}
func (r *Runtime) SetTunnelPluginInstallationStatus(ctx context.Context, id platformplugin.InstallationID,
	status installationdomain.Status) (installationdomain.Installation, error) {
	item, err := r.pluginInstallations.Get(ctx, id)
	if err != nil {
		return installationdomain.Installation{}, err
	}
	if item.Extension().Kind != platformplugin.TunnelTransport {
		return installationdomain.Installation{}, fmt.Errorf("installation %s is not tunnel.transport", id)
	}
	count, err := r.Tunnels.ReferencedInstallation(ctx, id)
	if err != nil {
		return installationdomain.Installation{}, err
	}
	if count > 0 && status != installationdomain.StatusEnabled {
		return installationdomain.Installation{}, fmt.Errorf("tunnel transport %s referenced by %d Tunnels: %w", id, count, installationapp.ErrInstallationInUse)
	}
	changed, err := installationapp.NewSetInstallationStatus(r.pluginInstallations).Execute(ctx, id, status)
	if err != nil {
		return installationdomain.Installation{}, err
	}
	if status != installationdomain.StatusEnabled {
		if err := r.externalProcesses.CloseInstallation(id); err != nil {
			return changed, err
		}
	}
	return changed, nil
}
func (r *Runtime) RemoveTunnelPluginInstallation(ctx context.Context, id platformplugin.InstallationID) error {
	item, err := r.pluginInstallations.Get(ctx, id)
	if err != nil {
		return err
	}
	if item.Extension().Kind != platformplugin.TunnelTransport {
		return fmt.Errorf("installation %s is not tunnel.transport", id)
	}
	count, err := r.Tunnels.ReferencedInstallation(ctx, id)
	if err != nil {
		return err
	}
	if count != 0 {
		return fmt.Errorf("tunnel transport %s referenced by %d Tunnels: %w", id, count, installationapp.ErrInstallationInUse)
	}
	if err := r.externalProcesses.CloseInstallation(id); err != nil {
		return err
	}
	return installationapp.NewRemoveInstallation(r.pluginInstallations, r.pluginPackages, nil).Execute(ctx, id)
}

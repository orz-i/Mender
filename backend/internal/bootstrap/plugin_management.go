package bootstrap

import (
	"context"
	"fmt"

	platformplugin "github.com/orz-i/Mender/backend/internal/plugin"
	installationdomain "github.com/orz-i/Mender/backend/internal/plugin/installation/domain"
)

// SetPluginInstallationStatus dispatches one package administration action
// based solely on its persisted, verified extension kind. The owning resource
// domain enforces referenced resources and the SQLite CAS is authoritative.
func (r *Runtime) SetPluginInstallationStatus(ctx context.Context, id platformplugin.InstallationID,
	status installationdomain.Status) (installationdomain.Installation, error) {
	item, err := r.GetPluginInstallation(ctx, id)
	if err != nil {
		return installationdomain.Installation{}, err
	}
	switch item.Extension().Kind {
	case platformplugin.ProviderTools:
		return r.SetProviderPluginInstallationStatus(ctx, id, status)
	case platformplugin.SandboxBackend:
		return r.SetSandboxPluginInstallationStatus(ctx, id, status)
	case platformplugin.TunnelTransport:
		return r.SetTunnelPluginInstallationStatus(ctx, id, status)
	default:
		return installationdomain.Installation{}, fmt.Errorf("unsupported plugin extension kind %s", item.Extension().Kind)
	}
}
func (r *Runtime) RemovePluginInstallation(ctx context.Context, id platformplugin.InstallationID) error {
	item, err := r.GetPluginInstallation(ctx, id)
	if err != nil {
		return err
	}
	switch item.Extension().Kind {
	case platformplugin.ProviderTools:
		return r.RemoveProviderPluginInstallation(ctx, id)
	case platformplugin.SandboxBackend:
		return r.RemoveSandboxPluginInstallation(ctx, id)
	case platformplugin.TunnelTransport:
		return r.RemoveTunnelPluginInstallation(ctx, id)
	default:
		return fmt.Errorf("unsupported plugin extension kind %s", item.Extension().Kind)
	}
}
func (r *Runtime) InstallPlugin(ctx context.Context, source string, kind platformplugin.Kind) (installationdomain.Installation, error) {
	if err := kind.Validate(); err != nil {
		return installationdomain.Installation{}, err
	}
	switch kind {
	case platformplugin.ProviderTools:
		return r.InstallProviderPlugin(ctx, source)
	case platformplugin.SandboxBackend:
		return r.InstallSandboxPlugin(ctx, source)
	case platformplugin.TunnelTransport:
		return r.InstallTunnelPlugin(ctx, source)
	default:
		return installationdomain.Installation{}, fmt.Errorf("unsupported plugin kind %s", kind)
	}
}

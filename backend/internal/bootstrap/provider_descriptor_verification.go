package bootstrap

import (
	"context"
	"errors"
	"fmt"

	installationdomain "github.com/orz-i/Mender/backend/internal/plugin/installation/domain"
	"github.com/orz-i/Mender/backend/internal/providerplugin"
	providerapplication "github.com/orz-i/Mender/backend/internal/providerplugin/application"
	providerexternal "github.com/orz-i/Mender/backend/internal/providerplugin/external"
)

// VerifyProviderPluginInstallation is an explicit owner-requested trust
// operation. Catalog reads never call it or launch verification implicitly.
func (r *Runtime) VerifyProviderPluginInstallation(ctx context.Context, id providerplugin.InstallationID) (item installationdomain.Installation, resultErr error) {
	if r == nil || r.pluginPackages == nil || r.providerPlugins == nil || r.pluginInstallations == nil || r.RuntimeGovernance == nil {
		return installationdomain.Installation{}, fmt.Errorf("verify provider plugin: runtime unavailable")
	}
	// Each verification owns a separate process registry, with the same host
	// isolation and bounded options as runtime processes. Concurrent verification
	// and Provider execution can never close one another's package process.
	processes, err := providerexternal.NewSupervisor(r.pluginPackages,
		externalProviderIsolationLauncher{governance: r.RuntimeGovernance}, providerexternal.DefaultOptions())
	if err != nil {
		return item, fmt.Errorf("verify provider plugin: compose process host: %w", err)
	}
	defer func() {
		if err := processes.CloseAll(); err != nil {
			resultErr = errors.Join(resultErr, fmt.Errorf("verify provider plugin: close process host: %w", err))
		}
	}()
	resolver, err := providerexternal.NewResolver(r.providerPlugins, r.pluginInstallations, processes)
	if err != nil {
		return item, fmt.Errorf("verify provider plugin: compose resolver: %w", err)
	}
	return providerapplication.NewVerifyDescriptor(r.pluginInstallations, resolver, processes, nil).Execute(ctx, id)
}

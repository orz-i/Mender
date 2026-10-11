package bootstrap

import (
	"context"
	"fmt"

	credentialdomain "github.com/orz-i/Mender/backend/internal/credential/domain"
	platformplugin "github.com/orz-i/Mender/backend/internal/plugin"
	installationdomain "github.com/orz-i/Mender/backend/internal/plugin/installation/domain"
	tunnelapplication "github.com/orz-i/Mender/backend/internal/tunnel/application"
	tunneldomain "github.com/orz-i/Mender/backend/internal/tunnel/domain"
	tunnelexternal "github.com/orz-i/Mender/backend/internal/tunnel/infrastructure/external"
	tunnelidentity "github.com/orz-i/Mender/backend/internal/tunnel/infrastructure/identity"
	tunnelsqlite "github.com/orz-i/Mender/backend/internal/tunnel/infrastructure/sqlite"
)

type Tunnels struct {
	Create tunnelapplication.CreateOpenAITunnel
	Get    tunnelapplication.GetTunnel
	List   tunnelapplication.ListTunnels
	Delete tunnelapplication.DeleteTunnel
	Start  tunnelapplication.StartTunnel

	store                 *tunnelsqlite.Store
	ids                   tunnelapplication.TunnelIDGenerator
	credentials           tunnelapplication.CredentialLookup
	externalInstallations tunnelexternal.InstallationLookup
	externalRunner        *tunnelexternal.Runner
}

func NewTunnels(
	ctx context.Context,
	databasePath string,
	credentials tunnelapplication.CredentialLookup,
	resolver tunnelapplication.CredentialResolver,
	runner tunnelapplication.ProcessRunner,
) (*Tunnels, error) {
	store, err := tunnelsqlite.Open(ctx, databasePath)
	if err != nil {
		return nil, fmt.Errorf("compose tunnels: %w", err)
	}
	ids := tunnelidentity.NewGenerator()
	return &Tunnels{
		Create: tunnelapplication.NewCreateOpenAITunnel(store, ids, credentials),
		Get:    tunnelapplication.NewGetTunnel(store),
		List:   tunnelapplication.NewListTunnels(store),
		Delete: tunnelapplication.NewDeleteTunnel(store, nil),
		Start:  tunnelapplication.NewStartTunnel(store, resolver, runner),
		store:  store,
		ids:    ids, credentials: credentials,
	}, nil
}

// ConfigureExternal connects typed Tunnel package management only after the
// shared Plugin Core installation store and process host have been composed.
func (t *Tunnels) ConfigureExternal(installations tunnelexternal.InstallationLookup, runner *tunnelexternal.Runner) error {
	if t == nil || t.store == nil || installations == nil || runner == nil {
		return fmt.Errorf("tunnel transport runtime dependencies unavailable")
	}
	t.externalInstallations = installations
	t.externalRunner = runner
	return nil
}

type CreateExternalTunnelRequest struct {
	DisplayName          string
	InstallationID       platformplugin.InstallationID
	ExternalTunnelID     string
	RuntimeCredentialRef credentialdomain.Reference
}

func (t *Tunnels) CreateExternal(ctx context.Context, request CreateExternalTunnelRequest) (tunneldomain.Tunnel, error) {
	if t == nil || t.externalInstallations == nil || t.externalRunner == nil || t.credentials == nil {
		return tunneldomain.Tunnel{}, fmt.Errorf("external Tunnel not configured")
	}
	item, err := t.externalInstallations.Get(ctx, request.InstallationID)
	if err != nil {
		return tunneldomain.Tunnel{}, err
	}
	if item.Status() != installationdomain.StatusEnabled || item.Extension().Kind != platformplugin.TunnelTransport ||
		item.Extension().ID != "main" {
		return tunneldomain.Tunnel{}, fmt.Errorf("create external Tunnel: %w", tunnelapplication.ErrTunnelTransportUnavailable)
	}
	if _, err := t.credentials.GetCredential(ctx, request.RuntimeCredentialRef); err != nil {
		return tunneldomain.Tunnel{}, fmt.Errorf("tunnel Credential Reference unavailable: %w", err)
	}
	id, err := t.ids.NewTunnelID()
	if err != nil {
		return tunneldomain.Tunnel{}, err
	}
	binding := tunneldomain.Binding{PluginID: item.PluginID(), ExtensionID: item.Extension().ID,
		InstallationID: item.ID()}
	tunnel, err := tunneldomain.NewTunnel(id, request.DisplayName, tunneldomain.ProviderExternal,
		binding, request.ExternalTunnelID, request.RuntimeCredentialRef)
	if err != nil {
		return tunneldomain.Tunnel{}, err
	}
	// Explicit operator create only: validate the exact package's schema and
	// typed config protocol without resolving/sending the Credential secret.
	if err := t.externalRunner.VerifyConfiguration(ctx, item, tunnel.ExternalTunnelID()); err != nil {
		return tunneldomain.Tunnel{}, fmt.Errorf("external Tunnel config: %w", err)
	}
	if err := t.store.Create(ctx, tunnel); err != nil {
		return tunneldomain.Tunnel{}, err
	}
	return tunnel, nil
}

func (t *Tunnels) ReferencedInstallation(ctx context.Context, id platformplugin.InstallationID) (int, error) {
	if t == nil || t.store == nil {
		return 0, fmt.Errorf("tunnel inventory unavailable")
	}
	return t.store.ReferencedInstallation(ctx, id.String())
}

func (t *Tunnels) Close() error {
	if t == nil || t.store == nil {
		return nil
	}
	return t.store.Close()
}

type profileTunnelLookup struct {
	get tunnelapplication.GetTunnel
}

func (a profileTunnelLookup) GetTunnel(
	ctx context.Context,
	id tunneldomain.TunnelID,
) (tunneldomain.Tunnel, error) {
	return a.get.Execute(ctx, id)
}

type runtimeTunnelUsageChecker struct {
	profiles *Profiles
	active   func(tunneldomain.TunnelID) bool
}

func (a runtimeTunnelUsageChecker) Usages(
	ctx context.Context,
	id tunneldomain.TunnelID,
) ([]tunnelapplication.Usage, error) {
	profiles, err := a.profiles.List.Execute(ctx)
	if err != nil {
		return nil, fmt.Errorf("tunnel usage: list profiles: %w", err)
	}
	usages := make([]tunnelapplication.Usage, 0)
	if a.active != nil && a.active(id) {
		usages = append(usages, tunnelapplication.Usage{
			Kind: "managed_mcp_server", Description: fmt.Sprintf("active managed MCP Tunnel %s", id),
		})
	}
	for _, profile := range profiles {
		tunnelID, ok := profile.TunnelID()
		if ok && tunnelID == id {
			usages = append(usages, tunnelapplication.Usage{
				Kind:        "profile",
				Description: fmt.Sprintf("profile %s", profile.Slug()),
			})
		}
	}
	return usages, nil
}

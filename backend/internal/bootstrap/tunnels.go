package bootstrap

import (
	"context"
	"fmt"

	tunnelapplication "github.com/orz-i/Mender/backend/internal/tunnel/application"
	tunneldomain "github.com/orz-i/Mender/backend/internal/tunnel/domain"
	tunnelidentity "github.com/orz-i/Mender/backend/internal/tunnel/infrastructure/identity"
	tunnelsqlite "github.com/orz-i/Mender/backend/internal/tunnel/infrastructure/sqlite"
)

type Tunnels struct {
	Create tunnelapplication.CreateOpenAITunnel
	Get    tunnelapplication.GetTunnel
	List   tunnelapplication.ListTunnels
	Delete tunnelapplication.DeleteTunnel
	Start  tunnelapplication.StartTunnel

	store *tunnelsqlite.Store
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
	}, nil
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

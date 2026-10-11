// Package pluginregistry resolves exact, typed Tunnel Transports independently
// of Tool Providers and Sandbox backends. No discovery call launches a process.
package pluginregistry

import (
	"context"
	"fmt"

	platformplugin "github.com/orz-i/Mender/backend/internal/plugin"
	tunnelapplication "github.com/orz-i/Mender/backend/internal/tunnel/application"
	tunneldomain "github.com/orz-i/Mender/backend/internal/tunnel/domain"
	tunnelexternal "github.com/orz-i/Mender/backend/internal/tunnel/infrastructure/external"
)

// Registry is the host-owned Tunnel transport extension dispatcher.
type Registry struct {
	openai   tunnelapplication.ProcessRunner
	external *tunnelexternal.Runner
	builtin  platformplugin.Descriptor
}

func New(openai tunnelapplication.ProcessRunner, external *tunnelexternal.Runner) (*Registry, error) {
	if openai == nil || external == nil {
		return nil, fmt.Errorf("tunnel transport registry dependencies unavailable")
	}
	descriptor := platformplugin.Descriptor{
		PluginID: "io.mender.tunnel.openai", PackageVersion: "1.0.0",
		Extension:     platformplugin.Extension{ID: "main", Kind: platformplugin.TunnelTransport, ProtocolVersion: 1},
		DisplayName:   "OpenAI Secure MCP Tunnel",
		Description:   "Trusted OpenAI tunnel-client transport; external control-plane readiness requires observation",
		ConfigVersion: 1, Capabilities: []string{"managed-loopback-mcp"},
	}
	if err := descriptor.Validate(); err != nil {
		return nil, err
	}
	return &Registry{openai: openai, external: external, builtin: descriptor}, nil
}
func (r *Registry) BuiltinDescriptor() platformplugin.Descriptor {
	if r == nil {
		return platformplugin.Descriptor{}
	}
	return r.builtin.Clone()
}
func (r *Registry) Preflight(ctx context.Context, tunnel tunneldomain.Tunnel) error {
	if r == nil {
		return fmt.Errorf("tunnel transport registry unavailable")
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	switch tunnel.Provider() {
	case tunneldomain.ProviderOpenAI:
		if err := tunnel.Binding().ValidateProvider(tunneldomain.ProviderOpenAI); err != nil {
			return err
		}
		return r.openai.Preflight(ctx, tunnel)
	case tunneldomain.ProviderExternal:
		return r.external.Preflight(ctx, tunnel)
	default:
		return fmt.Errorf("unknown Tunnel transport provider %s", tunnel.Provider())
	}
}
func (r *Registry) Start(ctx context.Context, tunnel tunneldomain.Tunnel,
	endpoint string, secret []byte) (tunnelapplication.Process, error) {
	if err := r.Preflight(ctx, tunnel); err != nil {
		return nil, err
	}
	if err := tunnelapplication.ValidateManagedMCPEndpoint(endpoint); err != nil {
		return nil, err
	}
	switch tunnel.Provider() {
	case tunneldomain.ProviderOpenAI:
		return r.openai.Start(ctx, tunnel, endpoint, secret)
	case tunneldomain.ProviderExternal:
		return r.external.Start(ctx, tunnel, endpoint, secret)
	default:
		return nil, fmt.Errorf("tunnel transport unavailable")
	}
}

var _ tunnelapplication.ProcessRunner = (*Registry)(nil)

// Package external implements a typed Tunnel Transport extension without
// granting the plugin ownership of MCP listeners, Profile Policy or secrets.
package external

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os/exec"
	"slices"
	"time"

	platformplugin "github.com/orz-i/Mender/backend/internal/plugin"
	installationapp "github.com/orz-i/Mender/backend/internal/plugin/installation/application"
	installationdomain "github.com/orz-i/Mender/backend/internal/plugin/installation/domain"
	pluginprocess "github.com/orz-i/Mender/backend/internal/plugin/process"
	tunnelv1 "github.com/orz-i/Mender/sdk/tunnelplugin/go/protocol/v1"
	tunneltransport "github.com/orz-i/Mender/sdk/tunnelplugin/go/transport"
)

const (
	descriptorTimeout = 5 * time.Second
	startTimeout      = 15 * time.Second
	maxStderrBytes    = 64 * 1024
)

type ProcessLauncher interface {
	Command(context.Context, string, string) (*exec.Cmd, error)
}
type Dialer struct{ launcher ProcessLauncher }

func NewDialer(launcher ProcessLauncher) (*Dialer, error) {
	if launcher == nil {
		return nil, fmt.Errorf("tunnel transport: governed launcher unavailable")
	}
	return &Dialer{launcher: launcher}, nil
}

type Process struct {
	client     *tunneltransport.Client
	service    tunnelv1.TunnelTransportServiceClient
	descriptor platformplugin.Descriptor
	semantics  string
}

func (p *Process) Exited() bool { return p == nil || p.client == nil || p.client.Exited() }
func (p *Process) Close() error {
	if p == nil || p.client == nil {
		return nil
	}
	return p.client.Close()
}
func (p *Process) Service() tunnelv1.TunnelTransportServiceClient {
	if p == nil {
		return nil
	}
	return p.service
}
func (p *Process) Descriptor() platformplugin.Descriptor { return p.descriptor.Clone() }
func (p *Process) ReachabilitySemantics() string         { return p.semantics }

func (d *Dialer) Connect(ctx context.Context, item installationdomain.Installation,
	pkg installationapp.ResolvedPackage) (pluginprocess.Client, error) {
	if d == nil || d.launcher == nil {
		return nil, fmt.Errorf("tunnel transport: no launcher")
	}
	if item.Extension().Kind != platformplugin.TunnelTransport || item.Extension().ID != "main" ||
		item.Extension().ProtocolVersion != 1 || pkg.Manifest.Extensions[0] != item.Extension() {
		return nil, fmt.Errorf("tunnel transport: wrong extension kind/protocol")
	}
	command, err := d.launcher.Command(ctx, pkg.EntrypointPath, pkg.WorkingDirectory)
	if err != nil {
		return nil, fmt.Errorf("tunnel transport command: %w", err)
	}
	client, err := tunneltransport.StartClient(tunneltransport.ClientOptions{
		Executable: pkg.EntrypointPath, WorkingDirectory: pkg.WorkingDirectory,
		Versions: []uint32{1}, ExecutableChecksum: pkg.ExecutableChecksum,
		Command: command, StartTimeout: startTimeout, PluginLogBufferSize: maxStderrBytes,
	})
	if err != nil {
		return nil, fmt.Errorf("tunnel transport start: %w", err)
	}
	cleanup := func(err error) (pluginprocess.Client, error) { _ = client.Close(); return nil, err }
	probeCtx, cancel := context.WithTimeout(ctx, descriptorTimeout)
	defer cancel()
	reply, err := client.Service().GetDescriptor(probeCtx, &tunnelv1.GetDescriptorRequest{})
	if err != nil {
		return cleanup(fmt.Errorf("tunnel transport descriptor: %w", err))
	}
	remote := reply.GetTransport()
	if remote == nil || remote.GetPluginId() != item.PluginID().String() ||
		remote.GetPackageVersion() != item.PackageVersion() ||
		remote.GetProtocolVersion() != 1 || client.ProtocolVersion() != 1 {
		return cleanup(fmt.Errorf("tunnel transport descriptor identity/protocol mismatch"))
	}
	descriptor := platformplugin.Descriptor{
		PluginID: item.PluginID(), PackageVersion: item.PackageVersion(), Extension: item.Extension(),
		DisplayName: remote.GetDisplayName(), Description: remote.GetDescription(),
		ConfigVersion: remote.GetConfigVersion(),
		ConfigSchema:  append(json.RawMessage(nil), remote.GetConfigSchemaJson()...),
		Capabilities:  slices.Clone(remote.GetCapabilities()),
	}
	if err := descriptor.Validate(); err != nil {
		return cleanup(fmt.Errorf("tunnel transport descriptor: %w", err))
	}
	if !slices.Equal(descriptor.Capabilities, []string{"loopback-mcp-proxy"}) {
		return cleanup(fmt.Errorf("tunnel transport unsupported capabilities"))
	}
	if remote.GetReachabilitySemantics() == "" || len(remote.GetReachabilitySemantics()) > 1024 {
		return cleanup(fmt.Errorf("tunnel transport lacks bounded reachability semantics"))
	}
	if pinned, ok := item.VerifiedDescriptor(); ok {
		a, _ := json.Marshal(pinned)
		b, _ := json.Marshal(descriptor)
		if !bytes.Equal(a, b) {
			return cleanup(fmt.Errorf("tunnel transport differs from pinned descriptor"))
		}
	}
	return &Process{client: client, service: client.Service(), descriptor: descriptor,
		semantics: remote.GetReachabilitySemantics()}, nil
}

var _ pluginprocess.Dialer = (*Dialer)(nil)

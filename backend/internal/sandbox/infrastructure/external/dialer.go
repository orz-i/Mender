// Package external hosts installable Sandbox Backend plugins behind the typed
// public Sandbox Plugin Protocol. Mender remains the authority for governance.
package external

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os/exec"
	"time"

	platformplugin "github.com/orz-i/Mender/backend/internal/plugin"
	installationapp "github.com/orz-i/Mender/backend/internal/plugin/installation/application"
	installationdomain "github.com/orz-i/Mender/backend/internal/plugin/installation/domain"
	pluginprocess "github.com/orz-i/Mender/backend/internal/plugin/process"
	sandboxv1 "github.com/orz-i/Mender/sdk/sandboxplugin/go/protocol/v1"
	sandboxtransport "github.com/orz-i/Mender/sdk/sandboxplugin/go/transport"
)

const (
	descriptorTimeout = 5 * time.Second
	startTimeout      = 15 * time.Second
)

type ProcessLauncher interface {
	Command(context.Context, string, string) (*exec.Cmd, error)
}
type Dialer struct{ launcher ProcessLauncher }

func NewDialer(launcher ProcessLauncher) (*Dialer, error) {
	if launcher == nil {
		return nil, fmt.Errorf("sandbox external dialer: governed process launcher required")
	}
	return &Dialer{launcher: launcher}, nil
}

type Process struct {
	client     *sandboxtransport.Client
	service    sandboxv1.SandboxPluginServiceClient
	descriptor platformplugin.Descriptor
}

func (p *Process) Exited() bool { return p == nil || p.client == nil || p.client.Exited() }
func (p *Process) Close() error {
	if p == nil || p.client == nil {
		return nil
	}
	return p.client.Close()
}
func (p *Process) Service() sandboxv1.SandboxPluginServiceClient {
	if p == nil {
		return nil
	}
	return p.service
}
func (p *Process) Descriptor() platformplugin.Descriptor { return p.descriptor.Clone() }

func (d *Dialer) Connect(ctx context.Context, item installationdomain.Installation,
	pkg installationapp.ResolvedPackage) (pluginprocess.Client, error) {
	if d == nil || d.launcher == nil {
		return nil, fmt.Errorf("sandbox external dialer unavailable")
	}
	if item.Extension().Kind != platformplugin.SandboxBackend ||
		pkg.Manifest.Extensions[0] != item.Extension() || item.Extension().ProtocolVersion != 1 {
		return nil, fmt.Errorf("sandbox external dialer: unsupported extension or protocol")
	}
	command, err := d.launcher.Command(ctx, pkg.EntrypointPath, pkg.WorkingDirectory)
	if err != nil {
		return nil, fmt.Errorf("sandbox external dialer: launch command: %w", err)
	}
	client, err := sandboxtransport.StartClient(sandboxtransport.ClientOptions{
		Executable: pkg.EntrypointPath, WorkingDirectory: pkg.WorkingDirectory,
		Versions: []uint32{1}, ExecutableChecksum: pkg.ExecutableChecksum,
		Command: command, StartTimeout: startTimeout,
		PluginLogBufferSize: 64 * 1024,
	})
	if err != nil {
		return nil, fmt.Errorf("sandbox external dialer: start: %w", err)
	}
	closeFailure := func(err error) (pluginprocess.Client, error) { _ = client.Close(); return nil, err }
	probeCtx, cancel := context.WithTimeout(ctx, descriptorTimeout)
	defer cancel()
	response, err := client.Service().GetDescriptor(probeCtx, &sandboxv1.GetDescriptorRequest{})
	if err != nil {
		return closeFailure(fmt.Errorf("sandbox external dialer: descriptor: %w", err))
	}
	raw := response.GetDescriptor_()
	if raw == nil {
		return closeFailure(fmt.Errorf("sandbox external dialer: missing descriptor"))
	}
	if raw.GetPluginId() != item.PluginID().String() || raw.GetPackageVersion() != item.PackageVersion() ||
		raw.GetProtocolVersion() != 1 || client.ProtocolVersion() != 1 {
		return closeFailure(fmt.Errorf("sandbox external dialer: descriptor identity/protocol mismatch"))
	}
	descriptor := platformplugin.Descriptor{
		PluginID: item.PluginID(), PackageVersion: item.PackageVersion(),
		Extension: item.Extension(), DisplayName: raw.GetDisplayName(),
		Description: raw.GetDescription(), ConfigVersion: raw.GetConfigVersion(),
		ConfigSchema: append(json.RawMessage(nil), raw.GetConfigSchemaJson()...),
		Capabilities: append([]string(nil), raw.GetCapabilities()...),
	}
	if err := descriptor.Validate(); err != nil {
		return closeFailure(fmt.Errorf("sandbox external dialer: descriptor: %w", err))
	}
	if len(descriptor.Capabilities) == 0 {
		return closeFailure(fmt.Errorf("sandbox external dialer: no workspace capabilities"))
	}
	for _, cap := range descriptor.Capabilities {
		switch cap {
		case "file.read", "file.search", "file.edit", "process.shell":
		default:
			return closeFailure(fmt.Errorf("sandbox external dialer: unsupported capability %q", cap))
		}
	}
	if pinned, ok := item.VerifiedDescriptor(); ok {
		// Compare verified typed descriptor snapshots, not arbitrary code.
		expectedJSON, _ := json.Marshal(pinned)
		currentJSON, _ := json.Marshal(descriptor)
		if !bytes.Equal(expectedJSON, currentJSON) {
			return closeFailure(fmt.Errorf("sandbox external dialer: descriptor differs from pinned installation snapshot"))
		}
	}
	return &Process{client: client, service: client.Service(), descriptor: descriptor}, nil
}

var _ pluginprocess.Dialer = (*Dialer)(nil)

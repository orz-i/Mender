package external

import (
	"context"
	"fmt"
	"os/exec"
	"strings"
	"sync"
	"time"

	platformplugin "github.com/orz-i/Mender/backend/internal/plugin"
	installationapplication "github.com/orz-i/Mender/backend/internal/plugin/installation/application"
	installationdomain "github.com/orz-i/Mender/backend/internal/plugin/installation/domain"
	pluginprocess "github.com/orz-i/Mender/backend/internal/plugin/process"
	pluginprotocol "github.com/orz-i/Mender/sdk/providerplugin/go/protocol"
	providerv1 "github.com/orz-i/Mender/sdk/providerplugin/go/protocol/v1"
	plugintransport "github.com/orz-i/Mender/sdk/providerplugin/go/transport"
)

const (
	defaultStartTimeout          = 15 * time.Second
	defaultDescriptorTimeout     = 5 * time.Second
	defaultValidateConfigTimeout = 5 * time.Second
	defaultInspectConfigTimeout  = 5 * time.Second
	defaultOpenTimeout           = 15 * time.Second
	defaultListToolsTimeout      = 30 * time.Second
	defaultMaxStderrBytes        = 64 * 1024
)

// PackageResolver verifies an installed package before process launch.
type PackageResolver interface {
	Resolve(
		context.Context,
		installationdomain.Installation,
	) (installationapplication.ResolvedPackage, error)
}

type ProcessLauncher interface {
	Command(context.Context, string, string) (*exec.Cmd, error)
}

type HostProcessLauncher struct{}

func (HostProcessLauncher) Command(
	ctx context.Context,
	executable string,
	workingDirectory string,
) (*exec.Cmd, error) {
	if executable == "" || workingDirectory == "" {
		return nil, fmt.Errorf("external provider host launcher: executable and working directory are required")
	}
	cmd := exec.CommandContext(ctx, executable) // #nosec G204 -- exact verified plugin executable, no shell.
	cmd.Dir = workingDirectory
	return cmd, nil
}

// Options configure native external plugin process lifecycle.
type Options struct {
	StartTimeout          time.Duration
	DescriptorTimeout     time.Duration
	ValidateConfigTimeout time.Duration
	InspectConfigTimeout  time.Duration
	OpenTimeout           time.Duration
	ListToolsTimeout      time.Duration
	MaxStderrBytes        int
}

// DefaultOptions returns bounded native process defaults.
func DefaultOptions() Options {
	return Options{
		StartTimeout:          defaultStartTimeout,
		DescriptorTimeout:     defaultDescriptorTimeout,
		ValidateConfigTimeout: defaultValidateConfigTimeout,
		InspectConfigTimeout:  defaultInspectConfigTimeout,
		OpenTimeout:           defaultOpenTimeout,
		ListToolsTimeout:      defaultListToolsTimeout,
		MaxStderrBytes:        defaultMaxStderrBytes,
	}
}

// Process is one connected external plugin package process.
type Process struct {
	installationID installationdomain.InstallationID
	digest         string
	client         *plugintransport.Client
	service        providerv1.ProviderPluginServiceClient
	protocol       uint32
	descriptor     *providerv1.Descriptor
	stderr         *boundedWriter
}

// Service returns the connected Provider Plugin Protocol client.
func (p *Process) Service() providerv1.ProviderPluginServiceClient {
	if p == nil {
		return nil
	}
	return p.service
}

// ProtocolVersion returns the negotiated Provider Plugin Protocol version.
func (p *Process) ProtocolVersion() uint32 {
	if p == nil {
		return 0
	}
	return p.protocol
}

// Descriptor returns a defensive copy of the verified runtime descriptor.
func (p *Process) Descriptor() *providerv1.Descriptor {
	if p == nil || p.descriptor == nil {
		return nil
	}
	return cloneDescriptor(p.descriptor)
}

// Exited reports whether the native plugin process has terminated.
func (p *Process) Exited() bool {
	return p == nil || p.client == nil || p.client.Exited()
}

// Close terminates the native plugin process.
func (p *Process) Close() error {
	if p == nil || p.client == nil {
		return nil
	}
	return p.client.Close()
}

// Supervisor is the typed Provider protocol connector into one shared plugin
// process host. Process identity, synchronization and teardown live only in the
// generic host, not in this Provider extension adapter.
type Supervisor struct {
	host     *pluginprocess.Supervisor
	launcher ProcessLauncher
	options  Options
}

// NewSupervisor registers the Provider RPC dialect on the shared process host.
func NewSupervisor(packages PackageResolver, launcher ProcessLauncher, options Options) (*Supervisor, error) {
	if packages == nil {
		return nil, fmt.Errorf("new external plugin supervisor: package resolver must not be nil")
	}
	if launcher == nil {
		return nil, fmt.Errorf("new external plugin supervisor: launcher must not be nil")
	}
	if options.StartTimeout <= 0 {
		options.StartTimeout = defaultStartTimeout
	}
	if options.DescriptorTimeout <= 0 {
		options.DescriptorTimeout = defaultDescriptorTimeout
	}
	if options.ValidateConfigTimeout <= 0 {
		options.ValidateConfigTimeout = defaultValidateConfigTimeout
	}
	if options.InspectConfigTimeout <= 0 {
		options.InspectConfigTimeout = defaultInspectConfigTimeout
	}
	if options.OpenTimeout <= 0 {
		options.OpenTimeout = defaultOpenTimeout
	}
	if options.ListToolsTimeout <= 0 {
		options.ListToolsTimeout = defaultListToolsTimeout
	}
	if options.MaxStderrBytes <= 0 {
		options.MaxStderrBytes = defaultMaxStderrBytes
	}
	host, err := pluginprocess.NewSupervisor(packages)
	if err != nil {
		return nil, err
	}
	s := &Supervisor{host: host, launcher: launcher, options: options}
	if err := host.Register(platformplugin.ProviderTools, s); err != nil {
		return nil, err
	}
	return s, nil
}

// ProcessHost exposes the one owning process registry so additional typed
// dialers can be registered during Runtime composition, before first Ensure.
func (s *Supervisor) ProcessHost() *pluginprocess.Supervisor {
	if s == nil {
		return nil
	}
	return s.host
}

type controlPlaneTimeouts struct {
	validateConfig time.Duration
	inspectConfig  time.Duration
	open           time.Duration
	listTools      time.Duration
}

func defaultControlPlaneTimeouts() controlPlaneTimeouts {
	return controlPlaneTimeouts{validateConfig: defaultValidateConfigTimeout,
		inspectConfig: defaultInspectConfigTimeout, open: defaultOpenTimeout, listTools: defaultListToolsTimeout}
}
func (s *Supervisor) controlPlaneTimeouts() controlPlaneTimeouts {
	if s == nil {
		return defaultControlPlaneTimeouts()
	}
	return controlPlaneTimeouts{validateConfig: s.options.ValidateConfigTimeout,
		inspectConfig: s.options.InspectConfigTimeout, open: s.options.OpenTimeout, listTools: s.options.ListToolsTimeout}
}

// Ensure returns one protocol-verified Provider process from the generic host.
func (s *Supervisor) Ensure(ctx context.Context, item installationdomain.Installation) (*Process, error) {
	if s == nil {
		return nil, fmt.Errorf("ensure external plugin process: supervisor is nil")
	}
	c, err := s.host.Ensure(ctx, item)
	if err != nil {
		return nil, err
	}
	p, ok := c.(*Process)
	if !ok {
		return nil, fmt.Errorf("ensure external Provider process: unexpected typed connector %T", c)
	}
	return p, nil
}
func (s *Supervisor) Running(id installationdomain.InstallationID) bool {
	return s != nil && s.host.Running(id)
}
func (s *Supervisor) CloseInstallation(id installationdomain.InstallationID) error {
	if s == nil {
		return nil
	}
	return s.host.CloseInstallation(id)
}
func (s *Supervisor) CloseAll() error {
	if s == nil {
		return nil
	}
	return s.host.CloseAll()
}

// Connect owns Provider-specific transport, protocol negotiation and descriptor
// validation. The generic host already checked status, digest and package kind.
func (s *Supervisor) Connect(ctx context.Context, item installationdomain.Installation,
	resolved installationapplication.ResolvedPackage) (pluginprocess.Client, error) {
	negotiated, err := pluginprotocol.Negotiate([]uint32{resolved.Manifest.Extensions[0].ProtocolVersion})
	if err != nil {
		return nil, fmt.Errorf("ensure external plugin process: protocol: %w", err)
	}
	command, err := s.launcher.Command(ctx, resolved.EntrypointPath, resolved.WorkingDirectory)
	if err != nil {
		return nil, fmt.Errorf("ensure external plugin process: construct command: %w", err)
	}
	stderr := newBoundedWriter(s.options.MaxStderrBytes)
	startTimeout := timeoutWithinContext(ctx, s.options.StartTimeout)
	client, err := plugintransport.StartClient(plugintransport.ClientOptions{
		Executable: resolved.EntrypointPath, WorkingDirectory: resolved.WorkingDirectory,
		Versions: []uint32{negotiated}, ExecutableChecksum: resolved.ExecutableChecksum,
		StartTimeout: startTimeout, Stderr: stderr, PluginLogBufferSize: s.options.MaxStderrBytes, Command: command,
	})
	if err != nil {
		return nil, fmt.Errorf("ensure external plugin process: start: %w", err)
	}
	p := &Process{installationID: item.ID(), digest: item.Digest(), client: client,
		service: client.Service(), protocol: client.ProtocolVersion(), stderr: stderr}
	descriptor, err := s.verifyDescriptor(ctx, item, resolved, p)
	if err != nil {
		_ = p.Close()
		return nil, err
	}
	p.descriptor = descriptor
	return p, nil
}

func (s *Supervisor) verifyDescriptor(
	ctx context.Context,
	installation installationdomain.Installation,
	resolved installationapplication.ResolvedPackage,
	process *Process,
) (*providerv1.Descriptor, error) {
	timeout := timeoutWithinContext(ctx, s.options.DescriptorTimeout)
	probeCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	response, err := process.service.GetDescriptor(
		probeCtx,
		&providerv1.GetDescriptorRequest{},
	)
	if err != nil {
		return nil, fmt.Errorf("ensure external plugin process: descriptor: %w", err)
	}
	descriptor := response.GetDescriptor_()
	if descriptor == nil {
		return nil, fmt.Errorf("ensure external plugin process: descriptor is missing")
	}
	if descriptor.GetPluginId() != installation.PluginID().String() ||
		descriptor.GetPluginId() != resolved.Manifest.PluginID {
		return nil, fmt.Errorf("ensure external plugin process: descriptor plugin id mismatch")
	}
	if descriptor.GetPackageVersion() != installation.PackageVersion() ||
		descriptor.GetPackageVersion() != resolved.Manifest.Version {
		return nil, fmt.Errorf("ensure external plugin process: descriptor package version mismatch")
	}
	if descriptor.GetProtocolVersion() != process.protocol {
		return nil, fmt.Errorf(
			"ensure external plugin process: descriptor protocol version %d does not match negotiated version %d",
			descriptor.GetProtocolVersion(),
			process.protocol,
		)
	}
	if descriptor.GetConfigVersion() == 0 {
		return nil, fmt.Errorf("ensure external plugin process: descriptor config version must be positive")
	}
	if strings.TrimSpace(descriptor.GetDisplayName()) == "" {
		return nil, fmt.Errorf("ensure external plugin process: descriptor display name must not be empty")
	}
	return cloneDescriptor(descriptor), nil
}

func cloneDescriptor(value *providerv1.Descriptor) *providerv1.Descriptor {
	if value == nil {
		return nil
	}
	return &providerv1.Descriptor{
		PluginId:         value.GetPluginId(),
		PackageVersion:   value.GetPackageVersion(),
		DisplayName:      value.GetDisplayName(),
		Description:      value.GetDescription(),
		ProtocolVersion:  value.GetProtocolVersion(),
		ConfigVersion:    value.GetConfigVersion(),
		ConfigSchemaJson: append([]byte(nil), value.GetConfigSchemaJson()...),
		UiSchemaJson:     append([]byte(nil), value.GetUiSchemaJson()...),
		Capabilities:     append([]string(nil), value.GetCapabilities()...),
	}
}

func timeoutWithinContext(ctx context.Context, configured time.Duration) time.Duration {
	if configured <= 0 {
		configured = time.Second
	}
	if deadline, ok := ctx.Deadline(); ok {
		remaining := time.Until(deadline)
		if remaining > 0 && remaining < configured {
			return remaining
		}
	}
	return configured
}

type boundedWriter struct {
	mu    sync.Mutex
	limit int
	data  []byte
}

func newBoundedWriter(limit int) *boundedWriter {
	if limit < 1 {
		limit = 1
	}
	return &boundedWriter{limit: limit}
}

func (w *boundedWriter) Write(p []byte) (int, error) {
	w.mu.Lock()
	defer w.mu.Unlock()

	if len(p) >= w.limit {
		w.data = append(w.data[:0], p[len(p)-w.limit:]...)
		return len(p), nil
	}
	overflow := len(w.data) + len(p) - w.limit
	if overflow > 0 {
		copy(w.data, w.data[overflow:])
		w.data = w.data[:len(w.data)-overflow]
	}
	w.data = append(w.data, p...)
	return len(p), nil
}

func (w *boundedWriter) bytes() []byte {
	w.mu.Lock()
	defer w.mu.Unlock()
	return append([]byte(nil), w.data...)
}

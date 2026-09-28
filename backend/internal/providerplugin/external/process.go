package external

import (
	"context"
	"fmt"
	"strings"
	"sync"
	"time"

	installationapplication "github.com/orz-i/Mender/backend/internal/providerplugin/installation/application"
	installationdomain "github.com/orz-i/Mender/backend/internal/providerplugin/installation/domain"
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

type processSlot struct {
	mu      sync.Mutex
	process *Process
}

// Supervisor owns reusable external plugin package processes.
type Supervisor struct {
	packages PackageResolver
	options  Options

	mu     sync.Mutex
	closed bool
	slots  map[installationdomain.InstallationID]*processSlot
}

// NewSupervisor constructs an external plugin process supervisor.
func NewSupervisor(
	packages PackageResolver,
	options Options,
) (*Supervisor, error) {
	if packages == nil {
		return nil, fmt.Errorf("new external plugin supervisor: package resolver must not be nil")
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
	return &Supervisor{
		packages: packages,
		options:  options,
		slots:    make(map[installationdomain.InstallationID]*processSlot),
	}, nil
}

type controlPlaneTimeouts struct {
	validateConfig time.Duration
	inspectConfig  time.Duration
	open           time.Duration
	listTools      time.Duration
}

func defaultControlPlaneTimeouts() controlPlaneTimeouts {
	return controlPlaneTimeouts{
		validateConfig: defaultValidateConfigTimeout,
		inspectConfig:  defaultInspectConfigTimeout,
		open:           defaultOpenTimeout,
		listTools:      defaultListToolsTimeout,
	}
}

func (s *Supervisor) controlPlaneTimeouts() controlPlaneTimeouts {
	if s == nil {
		return defaultControlPlaneTimeouts()
	}
	return controlPlaneTimeouts{
		validateConfig: s.options.ValidateConfigTimeout,
		inspectConfig:  s.options.InspectConfigTimeout,
		open:           s.options.OpenTimeout,
		listTools:      s.options.ListToolsTimeout,
	}
}

// Ensure returns one reusable healthy process for an enabled installation.
func (s *Supervisor) Ensure(
	ctx context.Context,
	installation installationdomain.Installation,
) (*Process, error) {
	if err := installation.Validate(); err != nil {
		return nil, fmt.Errorf("ensure external plugin process: installation: %w", err)
	}
	if installation.Status() != installationdomain.StatusEnabled {
		return nil, fmt.Errorf(
			"ensure external plugin process: installation %s is %s",
			installation.ID(),
			installation.Status(),
		)
	}
	slot, err := s.slot(installation.ID())
	if err != nil {
		return nil, err
	}

	slot.mu.Lock()
	defer slot.mu.Unlock()

	if slot.process != nil {
		if slot.process.digest != installation.Digest() {
			return nil, fmt.Errorf(
				"ensure external plugin process: installation %s metadata changed",
				installation.ID(),
			)
		}
		if !slot.process.Exited() {
			return slot.process, nil
		}
		_ = slot.process.Close()
		slot.process = nil
	}

	process, err := s.start(ctx, installation)
	if err != nil {
		return nil, err
	}
	slot.process = process
	return process, nil
}

// Running reports whether one installation currently has a live package process.
// It never starts or probes a process.
func (s *Supervisor) Running(id installationdomain.InstallationID) bool {
	if s == nil {
		return false
	}
	s.mu.Lock()
	slot := s.slots[id]
	s.mu.Unlock()
	if slot == nil {
		return false
	}
	slot.mu.Lock()
	defer slot.mu.Unlock()
	return slot.process != nil && !slot.process.Exited()
}

// CloseInstallation closes the reusable process for one installation, if present.
func (s *Supervisor) CloseInstallation(
	id installationdomain.InstallationID,
) error {
	if _, err := installationdomain.ParseInstallationID(id.String()); err != nil {
		return fmt.Errorf("close external plugin process: %w", err)
	}

	s.mu.Lock()
	slot := s.slots[id]
	s.mu.Unlock()
	if slot == nil {
		return nil
	}

	slot.mu.Lock()
	defer slot.mu.Unlock()
	if slot.process == nil {
		return nil
	}
	err := slot.process.Close()
	slot.process = nil
	if err != nil {
		return fmt.Errorf("close external plugin process: %w", err)
	}
	return nil
}

// CloseAll permanently closes the supervisor and all started plugin processes.
func (s *Supervisor) CloseAll() error {
	if s == nil {
		return nil
	}

	s.mu.Lock()
	if s.closed {
		s.mu.Unlock()
		return nil
	}
	s.closed = true
	slots := make([]*processSlot, 0, len(s.slots))
	for _, slot := range s.slots {
		slots = append(slots, slot)
	}
	s.mu.Unlock()

	var firstErr error
	for _, slot := range slots {
		slot.mu.Lock()
		if slot.process != nil {
			if err := slot.process.Close(); err != nil && firstErr == nil {
				firstErr = err
			}
			slot.process = nil
		}
		slot.mu.Unlock()
	}
	if firstErr != nil {
		return fmt.Errorf("close external plugin processes: %w", firstErr)
	}
	return nil
}

func (s *Supervisor) slot(
	id installationdomain.InstallationID,
) (*processSlot, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.closed {
		return nil, fmt.Errorf("ensure external plugin process: supervisor is closed")
	}
	slot := s.slots[id]
	if slot == nil {
		slot = &processSlot{}
		s.slots[id] = slot
	}
	return slot, nil
}

func (s *Supervisor) start(
	ctx context.Context,
	installation installationdomain.Installation,
) (*Process, error) {
	resolved, err := s.packages.Resolve(ctx, installation)
	if err != nil {
		return nil, fmt.Errorf("ensure external plugin process: resolve package: %w", err)
	}
	if resolved.Manifest.PluginID != installation.PluginID().String() ||
		resolved.Manifest.Version != installation.PackageVersion() {
		return nil, fmt.Errorf(
			"ensure external plugin process: resolved package identity mismatch",
		)
	}

	negotiated, err := pluginprotocol.Negotiate(resolved.Manifest.ProtocolVersions)
	if err != nil {
		return nil, fmt.Errorf("ensure external plugin process: protocol: %w", err)
	}

	stderr := newBoundedWriter(s.options.MaxStderrBytes)
	startTimeout := timeoutWithinContext(ctx, s.options.StartTimeout)
	client, err := plugintransport.StartClient(plugintransport.ClientOptions{
		Executable:          resolved.EntrypointPath,
		WorkingDirectory:    resolved.WorkingDirectory,
		Versions:            []uint32{negotiated},
		ExecutableChecksum:  resolved.ExecutableChecksum,
		StartTimeout:        startTimeout,
		Stderr:              stderr,
		PluginLogBufferSize: s.options.MaxStderrBytes,
	})
	if err != nil {
		return nil, fmt.Errorf("ensure external plugin process: start: %w", err)
	}

	process := &Process{
		installationID: installation.ID(),
		digest:         installation.Digest(),
		client:         client,
		service:        client.Service(),
		protocol:       client.ProtocolVersion(),
		stderr:         stderr,
	}
	descriptor, err := s.verifyDescriptor(ctx, installation, resolved, process)
	if err != nil {
		_ = process.Close()
		return nil, err
	}
	process.descriptor = descriptor
	return process, nil
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

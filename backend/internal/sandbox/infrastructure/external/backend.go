package external

import (
	"context"
	"fmt"
	"slices"
	"time"

	platformplugin "github.com/orz-i/Mender/backend/internal/plugin"
	installationdomain "github.com/orz-i/Mender/backend/internal/plugin/installation/domain"
	pluginprocess "github.com/orz-i/Mender/backend/internal/plugin/process"
	sandboxapp "github.com/orz-i/Mender/backend/internal/sandbox/application"
	sandboxdomain "github.com/orz-i/Mender/backend/internal/sandbox/domain"
	sandboxsqlite "github.com/orz-i/Mender/backend/internal/sandbox/infrastructure/sqlite"
	sandboxv1 "github.com/orz-i/Mender/sdk/sandboxplugin/go/protocol/v1"
)

const (
	maxSessionID  = 256
	maxResultJSON = 2 * 1024 * 1024
	closeTimeout  = 5 * time.Second
)

type InstallationLookup interface {
	Get(context.Context, platformplugin.InstallationID) (installationdomain.Installation, error)
}
type ConfigLookup interface {
	GetExternalConfig(context.Context, sandboxdomain.ID) (sandboxsqlite.ExternalConfig, error)
}
type Backend struct {
	installationID platformplugin.InstallationID
	descriptor     platformplugin.Descriptor
	installations  InstallationLookup
	processes      *pluginprocess.Supervisor
	configs        ConfigLookup
	capabilities   sandboxdomain.Capabilities
}

func NewBackend(item installationdomain.Installation, lookup InstallationLookup,
	processes *pluginprocess.Supervisor, configs ConfigLookup) (*Backend, error) {
	if lookup == nil || processes == nil || configs == nil {
		return nil, fmt.Errorf("external sandbox backend dependencies unavailable")
	}
	if err := item.Validate(); err != nil {
		return nil, err
	}
	if item.Extension().Kind != platformplugin.SandboxBackend || item.Extension().ID != "main" {
		return nil, fmt.Errorf("external sandbox requires sandbox.backend/main")
	}
	descriptor, ok := item.VerifiedDescriptor()
	if !ok {
		return nil, fmt.Errorf("external sandbox needs verified descriptor")
	}
	c, err := capabilitySet(descriptor.Capabilities)
	if err != nil {
		return nil, err
	}
	return &Backend{installationID: item.ID(), descriptor: descriptor, installations: lookup,
		processes: processes, configs: configs, capabilities: c}, nil
}

func capabilitySet(names []string) (sandboxdomain.Capabilities, error) {
	c := sandboxdomain.Capabilities{}
	for _, name := range names {
		switch sandboxdomain.Capability(name) {
		case sandboxdomain.CapabilityRead:
			c.Read = true
		case sandboxdomain.CapabilitySearch:
			c.Search = true
		case sandboxdomain.CapabilityEdit:
			c.Edit = true
		case sandboxdomain.CapabilityShell:
			c.Shell = true
		default:
			return c, fmt.Errorf("external Sandbox capability not supported: %s", name)
		}
	}
	if len(names) == 0 {
		return c, fmt.Errorf("external sandbox declares no capabilities")
	}
	return c, nil
}
func (*Backend) Kind() sandboxdomain.BackendKind                 { return sandboxdomain.BackendExternal }
func (b *Backend) Capabilities() sandboxdomain.Capabilities      { return b.capabilities }
func (b *Backend) Descriptor() platformplugin.Descriptor         { return b.descriptor.Clone() }
func (b *Backend) InstallationID() platformplugin.InstallationID { return b.installationID }

func (b *Backend) installation(ctx context.Context, binding sandboxdomain.BackendBinding) (installationdomain.Installation, error) {
	if b == nil || binding.InstallationID != b.installationID || binding.PluginID != b.descriptor.PluginID ||
		binding.ExtensionID != b.descriptor.Extension.ID {
		return installationdomain.Installation{}, sandboxdomain.ErrBackendUnavailable
	}
	item, err := b.installations.Get(ctx, binding.InstallationID)
	if err != nil {
		return installationdomain.Installation{}, fmt.Errorf("sandbox installation unavailable: %w", err)
	}
	if item.Status() != installationdomain.StatusEnabled || item.PluginID() != binding.PluginID ||
		item.Extension() != b.descriptor.Extension || item.Digest() == "" {
		return installationdomain.Installation{}, sandboxdomain.ErrBackendUnavailable
	}
	if pinned, ok := item.VerifiedDescriptor(); !ok || !slices.Equal(pinned.Capabilities, b.descriptor.Capabilities) ||
		pinned.PluginID != b.descriptor.PluginID || pinned.ConfigVersion != b.descriptor.ConfigVersion {
		return installationdomain.Installation{}, sandboxdomain.ErrBackendUnavailable
	}
	return item, nil
}
func (b *Backend) process(ctx context.Context, binding sandboxdomain.BackendBinding) (*Process, error) {
	item, err := b.installation(ctx, binding)
	if err != nil {
		return nil, err
	}
	started, err := b.processes.Ensure(ctx, item)
	if err != nil {
		return nil, fmt.Errorf("sandbox plugin process unavailable: %w", err)
	}
	typed, ok := started.(*Process)
	if !ok {
		return nil, fmt.Errorf("sandbox plugin host returned wrong extension process")
	}
	return typed, nil
}
func (b *Backend) config(ctx context.Context, item sandboxdomain.Sandbox) (sandboxsqlite.ExternalConfig, error) {
	if err := item.Validate(); err != nil {
		return sandboxsqlite.ExternalConfig{}, err
	}
	if item.Backend() != sandboxdomain.BackendExternal || item.Spec().Binding.InstallationID != b.installationID {
		return sandboxsqlite.ExternalConfig{}, sandboxdomain.ErrBackendUnavailable
	}
	config, err := b.configs.GetExternalConfig(ctx, item.ID())
	if err != nil {
		return sandboxsqlite.ExternalConfig{}, err
	}
	if config.ConfigVersion != b.descriptor.ConfigVersion {
		return sandboxsqlite.ExternalConfig{}, fmt.Errorf("sandbox plugin config version mismatch")
	}
	return config, nil
}

type Observation struct {
	Run        sandboxdomain.Run
	Diagnostic string
}

func parseObservation(item sandboxdomain.Sandbox, expectedGeneration uint64, reply *sandboxv1.LifecycleResponse) (Observation, error) {
	if reply == nil || reply.GetSandboxId() != item.ID().String() || reply.GetGeneration() != expectedGeneration {
		return Observation{}, fmt.Errorf("sandbox plugin returned mismatched identity/generation")
	}
	state := sandboxdomain.State(reply.GetState())
	if err := state.Validate(); err != nil {
		return Observation{}, err
	}
	switch state {
	case sandboxdomain.StateReady, sandboxdomain.StateStopped, sandboxdomain.StateDestroyed,
		sandboxdomain.StateUnknown, sandboxdomain.StateFailed, sandboxdomain.StateOrphaned:
	default:
		return Observation{}, fmt.Errorf("sandbox plugin returned unsupported observed state %q", state)
	}
	run, err := sandboxdomain.NewRun(item.ID(), expectedGeneration, state, reply.GetExternalId())
	if err != nil {
		return Observation{}, err
	}
	if len(reply.GetDiagnostic()) > 2048 {
		return Observation{}, fmt.Errorf("sandbox plugin diagnostic exceeds budget")
	}
	return Observation{Run: run, Diagnostic: reply.GetDiagnostic()}, nil
}
func (b *Backend) lifecycleArgs(ctx context.Context, item sandboxdomain.Sandbox, run sandboxdomain.Run, opID string) (*Process, *sandboxv1.LifecycleRequest, error) {
	config, err := b.config(ctx, item)
	if err != nil {
		return nil, nil, err
	}
	p, err := b.process(ctx, item.Spec().Binding)
	if err != nil {
		return nil, nil, err
	}
	if opID == "" {
		return nil, nil, fmt.Errorf("missing host lifecycle idempotency key")
	}
	req := &sandboxv1.LifecycleRequest{SandboxId: item.ID().String(), Generation: run.Generation,
		ExternalId: run.ExternalID, OperationId: opID, ConfigJson: config.JSON}
	return p, req, nil
}

func (b *Backend) Provision(ctx context.Context, item sandboxdomain.Sandbox, run sandboxdomain.Run, opID string) (Observation, error) {
	p, req, err := b.lifecycleArgs(ctx, item, run, opID)
	if err != nil {
		return Observation{}, err
	}
	if req.ExternalId != "" {
		return Observation{}, fmt.Errorf("new sandbox provisioning cannot target existing run")
	}
	response, err := p.Service().Provision(ctx, &sandboxv1.ProvisionRequest{Operation: req})
	if err != nil {
		return Observation{}, fmt.Errorf("sandbox plugin provision uncertain: %w", err)
	}
	return parseObservation(item, run.Generation, response.GetResult())
}
func (b *Backend) Change(ctx context.Context, item sandboxdomain.Sandbox, run sandboxdomain.Run,
	opID string, action string) (Observation, error) {
	p, req, err := b.lifecycleArgs(ctx, item, run, opID)
	if err != nil {
		return Observation{}, err
	}
	var raw *sandboxv1.LifecycleResponse
	switch action {
	case "start":
		res, e := p.Service().Start(ctx, &sandboxv1.StartRequest{Operation: req})
		err = e
		if res != nil {
			raw = res.GetResult()
		}
	case "stop":
		res, e := p.Service().Stop(ctx, &sandboxv1.StopRequest{Operation: req})
		err = e
		if res != nil {
			raw = res.GetResult()
		}
	case "destroy":
		res, e := p.Service().Destroy(ctx, &sandboxv1.DestroyRequest{Operation: req})
		err = e
		if res != nil {
			raw = res.GetResult()
		}
	case "reconcile":
		res, e := p.Service().Reconcile(ctx, &sandboxv1.ReconcileRequest{Operation: req})
		err = e
		if res != nil {
			raw = res.GetResult()
		}
	default:
		return Observation{}, fmt.Errorf("sandbox plugin lifecycle action unsupported: %s", action)
	}
	if err != nil {
		return Observation{}, fmt.Errorf("sandbox plugin %s uncertain: %w", action, err)
	}
	return parseObservation(item, run.Generation, raw)
}
func (b *Backend) Open(ctx context.Context, item sandboxdomain.Sandbox, run sandboxdomain.Run) (sandboxapp.Session, error) {
	if run.SandboxID != item.ID() || run.State != sandboxdomain.StateReady {
		return nil, sandboxdomain.ErrConflict
	}
	config, err := b.config(ctx, item)
	if err != nil {
		return nil, err
	}
	p, err := b.process(ctx, item.Spec().Binding)
	if err != nil {
		return nil, err
	}
	response, err := p.Service().OpenSession(ctx, &sandboxv1.OpenSessionRequest{
		SandboxId: item.ID().String(), Generation: run.Generation, ExternalId: run.ExternalID, ConfigJson: config.JSON,
	})
	if err != nil {
		return nil, fmt.Errorf("sandbox plugin session open: %w", err)
	}
	id := response.GetSessionId()
	if len(id) == 0 || len(id) > maxSessionID {
		return nil, fmt.Errorf("sandbox plugin invalid session ID")
	}
	return &Session{service: p.Service(), id: id, capabilities: b.capabilities}, nil
}

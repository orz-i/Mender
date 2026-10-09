package dockerengine

import (
	"context"
	"errors"
	"fmt"
	"time"

	mobyclient "github.com/moby/moby/client"
	governancedomain "github.com/orz-i/Mender/backend/internal/governance/domain"
	sandboxdomain "github.com/orz-i/Mender/backend/internal/sandbox/domain"
)

// RunStore provides persisted compare-and-swap transitions and Sandbox identity.
// It is implemented by the existing Sandbox SQLite repository.
type RunStore interface {
	Get(context.Context, sandboxdomain.ID) (sandboxdomain.Sandbox, error)
	GetRun(context.Context, sandboxdomain.ID) (sandboxdomain.Run, error)
	TransitionRun(context.Context, sandboxdomain.Run, sandboxdomain.State, string) (sandboxdomain.Run, error)
}

// TemplateLookup must return a host-trusted, already persisted Docker template.
// The caller cannot choose an Engine socket, image or resource limits via Tool
// JSON. A production repository is a separate D1d management responsibility.
type TemplateLookup interface {
	GetDockerTemplate(context.Context, sandboxdomain.ID) (Template, error)
}

// ActionAdmitter implements the same global Policy/Approval/Audit admission
// as all other sandbox.provision operations. There is no permissive default.
type ActionAdmitter interface {
	AdmitAction(context.Context, governancedomain.Context, sandboxdomain.Sandbox, governancedomain.Action) error
}

type mutateAPI interface {
	ContainerCreate(context.Context, mobyclient.ContainerCreateOptions) (mobyclient.ContainerCreateResult, error)
	ContainerStart(context.Context, string, mobyclient.ContainerStartOptions) (mobyclient.ContainerStartResult, error)
	ContainerInspect(context.Context, string, mobyclient.ContainerInspectOptions) (mobyclient.ContainerInspectResult, error)
	ContainerStop(context.Context, string, mobyclient.ContainerStopOptions) (mobyclient.ContainerStopResult, error)
	ContainerRemove(context.Context, string, mobyclient.ContainerRemoveOptions) (mobyclient.ContainerRemoveResult, error)
}

// Provisioner is an internal, not Runtime-registered, D1b controller. It must
// not be offered through HTTP/CLI/Profile until D1c/D1d file and exec parity.
type Provisioner struct {
	observer  *Inspector
	daemon    mutateAPI
	store     RunStore
	templates TemplateLookup
	admit     ActionAdmitter
}

func NewProvisioner(endpoint EngineEndpoint, store RunStore, templates TemplateLookup, admit ActionAdmitter) (*Provisioner, error) {
	if store == nil || templates == nil || admit == nil {
		return nil, fmt.Errorf("docker provisioning: mandatory governance or durable configuration unavailable")
	}
	inspector, err := NewInspector(endpoint)
	if err != nil {
		return nil, err
	}
	daemon, ok := inspector.api.(mutateAPI)
	if !ok {
		_ = inspector.Close()
		return nil, sandboxdomain.ErrBackendUnavailable
	}
	return &Provisioner{observer: inspector, daemon: daemon, store: store, templates: templates, admit: admit}, nil
}
func (p *Provisioner) Close() error {
	if p == nil || p.observer == nil {
		return nil
	}
	return p.observer.Close()
}

func (p *Provisioner) preflight(ctx context.Context, caller governancedomain.Context, id sandboxdomain.ID, required sandboxdomain.State) (sandboxdomain.Sandbox, sandboxdomain.Run, Template, error) {
	if p == nil || p.observer == nil || p.daemon == nil || p.store == nil || p.templates == nil || p.admit == nil {
		return sandboxdomain.Sandbox{}, sandboxdomain.Run{}, Template{}, sandboxdomain.ErrBackendUnavailable
	}
	if err := caller.Validate(); err != nil {
		return sandboxdomain.Sandbox{}, sandboxdomain.Run{}, Template{}, err
	}
	sandbox, err := p.store.Get(ctx, id)
	if err != nil {
		return sandboxdomain.Sandbox{}, sandboxdomain.Run{}, Template{}, err
	}
	if sandbox.Backend() != sandboxdomain.BackendDocker || sandbox.Isolation() != sandboxdomain.IsolationContainer {
		return sandboxdomain.Sandbox{}, sandboxdomain.Run{}, Template{}, sandboxdomain.ErrBackendUnavailable
	}
	principal := caller.Principal()
	if sandbox.OwnerKind() != string(principal.Kind()) || sandbox.OwnerRef() != principal.Reference() {
		return sandboxdomain.Sandbox{}, sandboxdomain.Run{}, Template{}, sandboxdomain.ErrNotAuthorized
	}
	run, err := p.store.GetRun(ctx, id)
	if err != nil {
		return sandboxdomain.Sandbox{}, sandboxdomain.Run{}, Template{}, err
	}
	if run.State != required && (required != sandboxdomain.StateUnknown || run.State != sandboxdomain.StateProvisioning) {
		return sandboxdomain.Sandbox{}, sandboxdomain.Run{}, Template{}, sandboxdomain.ErrConflict
	}
	// Denial occurs before connecting to the daemon, inspecting local images or
	// changing durable state; ordinary Tool invocation has separate admission.
	if err := p.admit.AdmitAction(ctx, caller, sandbox, governancedomain.ActionSandboxProvision); err != nil {
		return sandboxdomain.Sandbox{}, sandboxdomain.Run{}, Template{}, err
	}
	template, err := p.templates.GetDockerTemplate(ctx, id)
	if err != nil {
		return sandboxdomain.Sandbox{}, sandboxdomain.Run{}, Template{}, fmt.Errorf("docker provisioning: trusted template unavailable: %w", sandboxdomain.ErrBackendUnavailable)
	}
	if err := template.Validate(); err != nil {
		return sandboxdomain.Sandbox{}, sandboxdomain.Run{}, Template{}, err
	}
	return sandbox, run, template, nil
}

// Provision executes at most one create attempt for a previously persisted
// defined Docker resource. A Transport error is *always* ambiguous: the run
// becomes unknown, and Provision must never auto-retry that generation.
func (p *Provisioner) Provision(ctx context.Context, caller governancedomain.Context, id sandboxdomain.ID) (sandboxdomain.Run, error) {
	sandbox, previous, template, err := p.preflight(ctx, caller, id, sandboxdomain.StateDefined)
	if err != nil {
		return sandboxdomain.Run{}, err
	}
	image, err := p.observer.InspectLocalImage(ctx, template)
	if err != nil {
		return sandboxdomain.Run{}, err
	}
	staged, err := p.store.TransitionRun(ctx, previous, sandboxdomain.StateProvisioning, "")
	if err != nil {
		return sandboxdomain.Run{}, err
	}
	return p.provisionStaged(ctx, caller, sandbox, staged, template, image, false)

}

// provisionStaged makes one Docker Create attempt against a fenced persisted
// provisioning Run. Unknown responses are never retried.
func (p *Provisioner) provisionStaged(ctx context.Context, caller governancedomain.Context, sandbox sandboxdomain.Sandbox, staged sandboxdomain.Run, template Template, image LocalImage, isRestart bool) (sandboxdomain.Run, error) {
	plan, err := NewContainerPlan(sandbox, staged, template, image)
	if err != nil {
		return sandboxdomain.Run{}, errors.Join(err, p.makeUnknown(ctx, staged, ""))
	}
	result, err := p.daemon.ContainerCreate(ctx, plan.Options)
	if err != nil {
		return sandboxdomain.Run{}, errors.Join(safeDaemonError(ctx, err), p.makeUnknown(ctx, staged, ""))
	}
	if !containerIDPattern.MatchString(result.ID) || len(result.Warnings) > 0 {
		return sandboxdomain.Run{}, errors.Join(sandboxdomain.ErrBackendUnavailable, p.makeUnknown(ctx, staged, ifValidContainerID(result.ID)))
	}
	if _, err := p.daemon.ContainerStart(ctx, result.ID, mobyclient.ContainerStartOptions{}); err != nil {
		return sandboxdomain.Run{}, errors.Join(safeDaemonError(ctx, err), p.makeUnknown(ctx, staged, result.ID))
	}
	inspect, err := p.daemon.ContainerInspect(ctx, result.ID, mobyclient.ContainerInspectOptions{})
	if err != nil {
		return sandboxdomain.Run{}, errors.Join(safeDaemonError(ctx, err), p.makeUnknown(ctx, staged, result.ID))
	}
	if err := plan.VerifyInspect(inspect.Container); err != nil || !inspect.Container.State.Running {
		return sandboxdomain.Run{}, errors.Join(sandboxdomain.ErrBackendUnavailable, p.makeUnknown(ctx, staged, result.ID))
	}
	var next sandboxdomain.Run
	if isRestart {
		lifecycle, ok := p.store.(dockerLifecycleStore)
		if !ok {
			return sandboxdomain.Run{}, sandboxdomain.ErrBackendUnavailable
		}
		next, err = lifecycle.FinishDockerLifecycle(ctx, staged, sandboxdomain.StateReady, result.ID, governancedomain.ActionSandboxStart, caller)
	} else {
		next, err = p.store.TransitionRun(ctx, staged, sandboxdomain.StateReady, result.ID)
	}
	if err != nil {
		return sandboxdomain.Run{}, errors.Join(err, p.makeUnknown(ctx, staged, result.ID))
	}
	return next, nil
}

// Reconcile is strictly read-only against the Engine: it inspects the exact
// deterministic container name, verifies immutable identity and restrictions,
// and may then recover the durable run. It never creates, starts or removes.
func (p *Provisioner) Reconcile(ctx context.Context, caller governancedomain.Context, id sandboxdomain.ID) (sandboxdomain.Run, error) {
	sandbox, run, template, err := p.preflight(ctx, caller, id, sandboxdomain.StateUnknown)
	if err != nil {
		return sandboxdomain.Run{}, err
	}
	image, err := p.observer.InspectLocalImage(ctx, template)
	if err != nil {
		return sandboxdomain.Run{}, err
	}
	synthetic, err := sandboxdomain.NewRun(id, run.Generation, sandboxdomain.StateProvisioning, "")
	if err != nil {
		return sandboxdomain.Run{}, err
	}
	plan, err := NewContainerPlan(sandbox, synthetic, template, image)
	if err != nil {
		return sandboxdomain.Run{}, err
	}
	response, err := p.daemon.ContainerInspect(ctx, plan.Name, mobyclient.ContainerInspectOptions{})
	if err != nil {
		return sandboxdomain.Run{}, safeDaemonError(ctx, err)
	}
	if run.ExternalID != "" && run.ExternalID != response.Container.ID {
		return sandboxdomain.Run{}, sandboxdomain.ErrConflict
	}
	if err := plan.VerifyInspect(response.Container); err != nil || !response.Container.State.Running {
		return sandboxdomain.Run{}, sandboxdomain.ErrBackendUnavailable
	}
	return p.store.TransitionRun(ctx, run, sandboxdomain.StateReady, response.Container.ID)
}

func (p *Provisioner) makeUnknown(ctx context.Context, staged sandboxdomain.Run, externalID string) error {
	cleanup, cancel := context.WithTimeout(context.WithoutCancel(ctx), 5*time.Second)
	defer cancel()
	_, err := p.store.TransitionRun(cleanup, staged, sandboxdomain.StateUnknown, externalID)
	if err != nil {
		return fmt.Errorf("docker provisioning: could not retain unknown outcome: %w", err)
	}
	return nil
}
func ifValidContainerID(id string) string {
	if containerIDPattern.MatchString(id) {
		return id
	}
	return ""
}

func (p *Provisioner) WorkspaceBackend() (*Backend, error) {
	if p == nil || p.observer == nil || p.templates == nil {
		return nil, sandboxdomain.ErrBackendUnavailable
	}
	return NewBackend(p.observer, p.templates)
}
func (p *Provisioner) VerifyLocalImage(ctx context.Context, template Template) (LocalImage, error) {
	if p == nil || p.observer == nil {
		return LocalImage{}, sandboxdomain.ErrBackendUnavailable
	}
	return p.observer.InspectLocalImage(ctx, template)
}

// Probe validates live local Engine capabilities without mutating resources.
func (p *Provisioner) Probe(ctx context.Context) (Probe, error) {
	if p == nil || p.observer == nil {
		return Probe{}, sandboxdomain.ErrBackendUnavailable
	}
	return p.observer.Probe(ctx)
}

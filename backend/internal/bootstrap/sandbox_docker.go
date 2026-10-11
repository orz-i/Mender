package bootstrap

import (
	"context"
	"fmt"
	"os"

	governancedomain "github.com/orz-i/Mender/backend/internal/governance/domain"
	sandboxapplication "github.com/orz-i/Mender/backend/internal/sandbox/application"
	sandboxdomain "github.com/orz-i/Mender/backend/internal/sandbox/domain"
	sandboxdocker "github.com/orz-i/Mender/backend/internal/sandbox/infrastructure/dockerengine"
	sandboxidentity "github.com/orz-i/Mender/backend/internal/sandbox/infrastructure/identity"
	sandboxpluginregistry "github.com/orz-i/Mender/backend/internal/sandbox/infrastructure/pluginregistry"
)

// Docker is opt-in through a host-owned explicit UNIX endpoint. Never honor
// DOCKER_HOST, Docker contexts, Tool arguments or remote Docker TCP endpoints.
const DockerSocketEnvironment = "MENDER_DOCKER_SOCKET"

func (s *Sandboxes) prepareDocker(ctx context.Context, raw string) (*sandboxdocker.Backend, *sandboxdocker.Provisioner, *sandboxdocker.TemplateStore, error) {
	endpoint, err := sandboxdocker.ParseEngineEndpoint(raw)
	if err != nil {
		return nil, nil, nil, fmt.Errorf("docker sandbox: explicitly configured endpoint invalid")
	}
	templates, err := sandboxdocker.OpenTemplateStore(ctx, s.dockerDatabasePath)
	if err != nil {
		return nil, nil, nil, fmt.Errorf("docker sandbox: immutable templates unavailable")
	}
	// Only a trusted Runtime operation may select a daemon. No model/Tool
	// argument, Docker context or DOCKER_HOST fallback reaches this path.
	controller, err := sandboxdocker.NewProvisioner(endpoint, s.store, templates, s.admission)
	if err != nil {
		_ = templates.Close()
		return nil, nil, nil, err
	}
	if _, err := controller.Probe(ctx); err != nil {
		_ = controller.Close()
		_ = templates.Close()
		return nil, nil, nil, sandboxdomain.ErrBackendUnavailable
	}
	backend, err := controller.WorkspaceBackend()
	if err != nil {
		_ = controller.Close()
		_ = templates.Close()
		return nil, nil, nil, err
	}
	return backend, controller, templates, nil
}

func (s *Sandboxes) configureDocker(ctx context.Context, _ string, _ sandboxapplication.AdmissionGate) (sandboxapplication.Backend, error) {
	raw := os.Getenv(DockerSocketEnvironment)
	if raw == "" {
		return nil, nil
	}
	backend, controller, templates, err := s.prepareDocker(ctx, raw)
	if err != nil {
		return nil, err
	}
	s.docker = controller
	s.dockerTemplates = templates
	s.dockerUnavailableReason = ""
	return backend, nil
}

// ActivateDocker enables the existing trusted Sandbox backend in this Runtime
// process, after an explicit *local operator* action. Neither Web management
// nor a Tool/Provider can activate Docker or choose its daemon endpoint.
// An unsuccessful activation leaves the Router without a Docker registration.
func (s *Sandboxes) ActivateDocker(ctx context.Context, rawEndpoint string) error {
	if s == nil || s.store == nil || s.backendPlugins == nil {
		return sandboxdomain.ErrBackendUnavailable
	}
	s.dockerMu.Lock()
	defer s.dockerMu.Unlock()
	if s.dockerClosed {
		return sandboxdomain.ErrBackendUnavailable
	}
	if s.docker != nil && s.dockerTemplates != nil {
		return nil
	}
	backend, controller, templates, err := s.prepareDocker(ctx, rawEndpoint)
	if err != nil {
		s.dockerUnavailableReason = "Docker Engine unavailable or insecure"
		return err
	}
	descriptor, err := sandboxpluginregistry.BuiltinDescriptor(backend)
	if err == nil {
		// Publish backend and lifecycle together, after the Engine preflight.
		// The registry holds the exact identity/capability binding.
		s.docker = controller
		s.dockerTemplates = templates
		err = s.backendPlugins.RegisterBuiltinWithLifecycle(backend, descriptor, dockerLifecycle{sandboxes: s})
	}
	if err != nil {
		s.docker = nil
		s.dockerTemplates = nil
		s.dockerUnavailableReason = "Docker Engine unavailable or insecure"
		_ = controller.Close()
		_ = templates.Close()
		return err
	}
	s.dockerUnavailableReason = ""
	return nil
}

func (s *Sandboxes) dockerController() *sandboxdocker.Provisioner {
	if s == nil {
		return nil
	}
	s.dockerMu.RLock()
	defer s.dockerMu.RUnlock()
	return s.docker
}

// ListDockerImages surfaces verified read-only local image choices to an
// authorized management UI. It does not create Sandboxes or pull images.
func (s *Sandboxes) ListDockerImages(ctx context.Context) ([]sandboxdocker.CachedImage, error) {
	if s == nil {
		return nil, sandboxdomain.ErrBackendUnavailable
	}
	s.dockerMu.RLock()
	defer s.dockerMu.RUnlock()
	if s.dockerClosed || s.docker == nil || s.dockerTemplates == nil {
		return nil, sandboxdomain.ErrBackendUnavailable
	}
	return s.docker.ListLocalImages(ctx)
}

func (s *Sandboxes) DockerAvailable() bool {
	if s == nil {
		return false
	}
	s.dockerMu.RLock()
	defer s.dockerMu.RUnlock()
	return s.docker != nil && s.dockerTemplates != nil
}
func (s *Sandboxes) DockerStatus() string {
	if s == nil {
		return "Docker Engine unavailable"
	}
	s.dockerMu.RLock()
	defer s.dockerMu.RUnlock()
	if s.docker != nil && s.dockerTemplates != nil {
		return "ready"
	}
	return s.dockerUnavailableReason
}
func (s *Sandboxes) BackendAvailable(item sandboxdomain.Sandbox) bool {
	return s != nil && s.backendPlugins != nil && s.backendPlugins.Available(item.Backend(), item.Spec().Binding)
}

type CreateDockerSandboxRequest struct {
	Name     string
	Template sandboxdocker.Template
}

// CreateDockerSandbox admits before any Engine I/O or resource creation, then
// persists Sandbox/Run/immutable Template atomically. A post-create ambiguous
// failure leaves a visible unknown run for reviewed operator reconciliation.
func (s *Sandboxes) CreateDockerSandbox(ctx context.Context, request CreateDockerSandboxRequest, caller governancedomain.Context) (sandboxdomain.Sandbox, error) {
	if s == nil {
		return sandboxdomain.Sandbox{}, sandboxdomain.ErrBackendUnavailable
	}
	s.dockerMu.RLock()
	defer s.dockerMu.RUnlock()
	if s.docker == nil || s.dockerTemplates == nil {
		return sandboxdomain.Sandbox{}, sandboxdomain.ErrBackendUnavailable
	}
	if err := caller.Validate(); err != nil {
		return sandboxdomain.Sandbox{}, err
	}
	if err := request.Template.Validate(); err != nil {
		return sandboxdomain.Sandbox{}, err
	}
	id, err := sandboxidentity.Generator{}.NewSandboxID()
	if err != nil {
		return sandboxdomain.Sandbox{}, err
	}
	subject := caller.Principal()
	item, err := sandboxdomain.NewSandbox(sandboxdomain.Spec{ID: id, Name: request.Name, Backend: sandboxdomain.BackendDocker, Binding: sandboxdomain.BuiltinBinding(sandboxdomain.BackendDocker), Isolation: sandboxdomain.IsolationContainer, OwnerKind: string(subject.Kind()), OwnerRef: subject.Reference()})
	if err != nil {
		return sandboxdomain.Sandbox{}, err
	}
	if err := s.admission.AdmitAction(ctx, caller, item, governancedomain.ActionSandboxProvision); err != nil {
		return sandboxdomain.Sandbox{}, err
	}
	if _, err := s.docker.VerifyLocalImage(ctx, request.Template); err != nil {
		return sandboxdomain.Sandbox{}, err
	}
	if err := s.dockerTemplates.CreateDefinedSandbox(ctx, item, request.Template); err != nil {
		return sandboxdomain.Sandbox{}, err
	}
	if _, err := s.docker.Provision(ctx, caller, id); err != nil {
		return item, fmt.Errorf("docker provisioning status must be reviewed for Sandbox %s: %w", id, err)
	}
	return item, nil
}

func (s *Sandboxes) ChangeLifecycle(ctx context.Context, id sandboxdomain.ID,
	action governancedomain.Action, caller governancedomain.Context) (sandboxdomain.Run, error) {
	if s == nil || s.store == nil || s.backendPlugins == nil {
		return sandboxdomain.Run{}, sandboxdomain.ErrBackendUnavailable
	}
	item, err := s.store.Get(ctx, id)
	if err != nil {
		return sandboxdomain.Run{}, err
	}
	controller, err := s.backendPlugins.ResolveLifecycle(item.Backend(), item.Spec().Binding)
	if err != nil {
		return sandboxdomain.Run{}, err
	}
	return controller.Change(ctx, id, action, caller)
}

func (s *Sandboxes) ReconcileDocker(ctx context.Context, id sandboxdomain.ID, caller governancedomain.Context) (sandboxdomain.Run, error) {
	if s.dockerController() == nil {
		return sandboxdomain.Run{}, sandboxdomain.ErrBackendUnavailable
	}
	item, err := s.store.Get(ctx, id)
	if err != nil {
		return sandboxdomain.Run{}, err
	}
	if item.Backend() != sandboxdomain.BackendDocker {
		return sandboxdomain.Run{}, sandboxdomain.ErrBackendUnavailable
	}
	return s.Reconcile(ctx, id, caller)
}

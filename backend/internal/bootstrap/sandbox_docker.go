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
)

// Docker is opt-in through a host-owned explicit UNIX endpoint. Never honor
// DOCKER_HOST, Docker contexts, Tool arguments or remote Docker TCP endpoints.
const DockerSocketEnvironment = "MENDER_DOCKER_SOCKET"

func (s *Sandboxes) configureDocker(ctx context.Context, databasePath string, admitter sandboxapplication.AdmissionGate) (sandboxapplication.Backend, error) {
	raw := os.Getenv(DockerSocketEnvironment)
	if raw == "" {
		return nil, nil
	}
	endpoint, err := sandboxdocker.ParseEngineEndpoint(raw)
	if err != nil {
		return nil, fmt.Errorf("docker sandbox: explicitly configured endpoint invalid")
	}
	templates, err := sandboxdocker.OpenTemplateStore(ctx, databasePath)
	if err != nil {
		return nil, fmt.Errorf("docker sandbox: immutable templates unavailable")
	}
	// The existing sandboxapplication.Admit implements both acquire and action
	// admission; no Provider activation path may start a Docker container.
	controller, err := sandboxdocker.NewProvisioner(endpoint, s.store, templates, s.admission)
	if err != nil {
		_ = templates.Close()
		return nil, err
	}
	if _, err := controller.Probe(ctx); err != nil {
		_ = controller.Close()
		_ = templates.Close()
		return nil, sandboxdomain.ErrBackendUnavailable
	}
	backend, err := controller.WorkspaceBackend()
	if err != nil {
		_ = controller.Close()
		_ = templates.Close()
		return nil, err
	}
	s.docker = controller
	s.dockerTemplates = templates
	s.dockerUnavailableReason = ""
	return backend, nil
}

func (s *Sandboxes) DockerAvailable() bool {
	return s != nil && s.docker != nil && s.dockerTemplates != nil
}
func (s *Sandboxes) DockerStatus() string {
	if s == nil {
		return "Docker Engine unavailable"
	}
	if s.DockerAvailable() {
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
	if !s.DockerAvailable() {
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

func (s *Sandboxes) ChangeLifecycle(ctx context.Context, id sandboxdomain.ID, action governancedomain.Action, caller governancedomain.Context) (sandboxdomain.Run, error) {
	if s == nil || s.store == nil {
		return sandboxdomain.Run{}, sandboxdomain.ErrBackendUnavailable
	}
	item, err := s.store.Get(ctx, id)
	if err != nil {
		return sandboxdomain.Run{}, err
	}
	switch item.Backend() {
	case sandboxdomain.BackendLocalWorkspace:
		return s.ChangeLocalLifecycle(ctx, id, action, caller)
	case sandboxdomain.BackendDocker:
		if !s.DockerAvailable() {
			return sandboxdomain.Run{}, sandboxdomain.ErrBackendUnavailable
		}
		return s.docker.ChangeLifecycle(ctx, caller, id, action)
	case sandboxdomain.BackendDaytona:
		if !s.DaytonaConfigured() {
			return sandboxdomain.Run{}, sandboxdomain.ErrBackendUnavailable
		}
		return s.daytona.ChangeLifecycle(ctx, caller, id, action)
	default:
		return sandboxdomain.Run{}, sandboxdomain.ErrBackendUnavailable
	}
}

func (s *Sandboxes) ReconcileDocker(ctx context.Context, id sandboxdomain.ID, caller governancedomain.Context) (sandboxdomain.Run, error) {
	if !s.DockerAvailable() {
		return sandboxdomain.Run{}, sandboxdomain.ErrBackendUnavailable
	}
	return s.docker.Reconcile(ctx, caller, id)
}

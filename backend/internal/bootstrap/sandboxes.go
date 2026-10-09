package bootstrap

import (
	"context"
	"fmt"
	"os"
	"path/filepath"

	governancedomain "github.com/orz-i/Mender/backend/internal/governance/domain"

	governanceapplication "github.com/orz-i/Mender/backend/internal/governance/application"
	invocationapplication "github.com/orz-i/Mender/backend/internal/invocation/application"
	sandboxapplication "github.com/orz-i/Mender/backend/internal/sandbox/application"
	sandboxdomain "github.com/orz-i/Mender/backend/internal/sandbox/domain"
	sandboxdaytona "github.com/orz-i/Mender/backend/internal/sandbox/infrastructure/daytona"
	sandboxdocker "github.com/orz-i/Mender/backend/internal/sandbox/infrastructure/dockerengine"
	sandboxidentity "github.com/orz-i/Mender/backend/internal/sandbox/infrastructure/identity"
	sandboxlocalworkspace "github.com/orz-i/Mender/backend/internal/sandbox/infrastructure/localworkspace"
	sandboxrouter "github.com/orz-i/Mender/backend/internal/sandbox/infrastructure/router"
	sandboxsqlite "github.com/orz-i/Mender/backend/internal/sandbox/infrastructure/sqlite"
)

// Sandboxes owns the persistent Sandbox inventory and a fail-closed Execution
// Router. Provider/Tool execution is not redirected until the Workspace hard cut.
type Sandboxes struct {
	store                   *sandboxsqlite.Store
	router                  *sandboxrouter.Router
	admission               sandboxapplication.Admit
	launcher                sandboxWorkspaceShellLauncher
	docker                  *sandboxdocker.Provisioner
	dockerTemplates         *sandboxdocker.TemplateStore
	dockerUnavailableReason string
	daytona                 *sandboxdaytona.Controller
	daytonaTemplates        *sandboxdaytona.TemplateStore
	daytonaExperimental     bool
}

func NewSandboxes(ctx context.Context, databasePath string, policy sandboxapplication.AdmissionPolicy, approval governanceapplication.ApprovalGate, governance *RuntimeGovernance, credentials invocationapplication.CredentialResolver) (*Sandboxes, error) {
	if policy == nil || approval == nil || governance == nil || governance.processIsolation == nil {
		return nil, fmt.Errorf("compose sandboxes: policy/approval unavailable")
	}
	store, err := sandboxsqlite.Open(ctx, databasePath)
	if err != nil {
		return nil, fmt.Errorf("compose sandboxes: %w", err)
	}
	gate := sandboxapplication.NewAdmit(policy, approval, store, sandboxidentity.Generator{}, systemClock{})
	launcher := sandboxWorkspaceShellLauncher{governance: governance}
	localBackend, err := sandboxlocalworkspace.New(launcher)
	if err != nil {
		_ = store.Close()
		return nil, fmt.Errorf("compose sandboxes: local workspace: %w", err)
	}
	component := &Sandboxes{store: store, admission: gate, launcher: launcher, dockerUnavailableReason: "Docker Engine is not configured"}
	backends := []sandboxapplication.Backend{localBackend}
	if dockerBackend, err := component.configureDocker(ctx, databasePath, gate); err == nil && dockerBackend != nil {
		backends = append(backends, dockerBackend)
	} else if err != nil {
		component.dockerUnavailableReason = "Docker Engine unavailable or insecure"
	}
	daytonaBackend, daytonaErr := component.configureDaytona(ctx, databasePath, credentials)
	if daytonaErr != nil {
		_ = component.Close()
		return nil, fmt.Errorf("compose remote Sandbox provider: %w", daytonaErr)
	}
	if daytonaBackend != nil {
		backends = append(backends, daytonaBackend)
	}
	router, err := sandboxrouter.New(store, store, gate, store, backends...)
	if err != nil {
		_ = component.Close()
		return nil, fmt.Errorf("compose sandboxes: router: %w", err)
	}
	component.router = router
	return component, nil
}

// List returns inventory metadata, not a privilege to start a Sandbox.
func (s *Sandboxes) List(ctx context.Context) ([]sandboxdomain.Sandbox, error) {
	if s == nil || s.store == nil {
		return nil, fmt.Errorf("sandbox inventory unavailable")
	}
	return s.store.List(ctx)
}
func (s *Sandboxes) Get(ctx context.Context, id sandboxdomain.ID) (sandboxdomain.Sandbox, error) {
	if s == nil || s.store == nil {
		return sandboxdomain.Sandbox{}, fmt.Errorf("sandbox inventory unavailable")
	}
	return s.store.Get(ctx, id)
}
func (s *Sandboxes) Close() error {
	if s == nil || s.store == nil {
		return nil
	}
	if s.docker != nil {
		_ = s.docker.Close()
	}
	if s.dockerTemplates != nil {
		_ = s.dockerTemplates.Close()
	}
	if s.daytonaTemplates != nil {
		_ = s.daytonaTemplates.Close()
	}
	return s.store.Close()
}

// CreateLocalSandboxRequest describes a local execution resource, never a Tool
// argument or Provider plugin configuration.
type CreateLocalSandboxRequest struct {
	Name      string
	Root      string
	Isolation sandboxdomain.IsolationClass
}

// CreateLocalSandbox admits sandbox.provision before resolving or opening a host
// path. Restricted host never downgrades to an unisolated host.
func (s *Sandboxes) CreateLocalSandbox(ctx context.Context, request CreateLocalSandboxRequest, caller governancedomain.Context) (sandboxdomain.Sandbox, error) {
	if s == nil || s.store == nil {
		return sandboxdomain.Sandbox{}, fmt.Errorf("sandbox inventory unavailable")
	}
	if err := caller.Validate(); err != nil {
		return sandboxdomain.Sandbox{}, err
	}
	if !filepath.IsAbs(request.Root) {
		return sandboxdomain.Sandbox{}, fmt.Errorf("local Sandbox root must be absolute")
	}
	id, err := sandboxidentity.Generator{}.NewSandboxID()
	if err != nil {
		return sandboxdomain.Sandbox{}, err
	}
	principal := caller.Principal()
	spec := sandboxdomain.Spec{ID: id, Name: request.Name, Backend: sandboxdomain.BackendLocalWorkspace, Isolation: request.Isolation, LocalRoot: filepath.Clean(request.Root), OwnerKind: string(principal.Kind()), OwnerRef: principal.Reference()}
	item, err := sandboxdomain.NewSandbox(spec)
	if err != nil {
		return sandboxdomain.Sandbox{}, err
	}
	if err := s.admission.AdmitAction(ctx, caller, item, governancedomain.ActionSandboxProvision); err != nil {
		return sandboxdomain.Sandbox{}, err
	}
	if err := ctx.Err(); err != nil {
		return sandboxdomain.Sandbox{}, err
	}
	root, err := filepath.EvalSymlinks(spec.LocalRoot)
	if err != nil {
		return sandboxdomain.Sandbox{}, fmt.Errorf("create local Sandbox: resolve root: %w", err)
	}
	info, err := os.Stat(root)
	if err != nil {
		return sandboxdomain.Sandbox{}, fmt.Errorf("create local Sandbox: inspect root: %w", err)
	}
	if !info.IsDir() {
		return sandboxdomain.Sandbox{}, fmt.Errorf("create local Sandbox: root must be a directory")
	}
	if err := s.launcher.CheckIsolation(request.Isolation); err != nil {
		return sandboxdomain.Sandbox{}, fmt.Errorf("create local Sandbox: %w", err)
	}
	spec.LocalRoot = filepath.Clean(root)
	item, err = sandboxdomain.NewSandbox(spec)
	if err != nil {
		return sandboxdomain.Sandbox{}, err
	}
	run, err := sandboxdomain.NewRun(id, 1, sandboxdomain.StateReady, "local_workspace:"+id.String())
	if err != nil {
		return sandboxdomain.Sandbox{}, err
	}
	if err := s.store.CreateReady(ctx, item, run); err != nil {
		return sandboxdomain.Sandbox{}, err
	}
	return item, nil
}

// GetRun reports the last persisted observed backend state. It does not probe,
// start or create instances while serving the management control plane.
func (s *Sandboxes) GetRun(ctx context.Context, id sandboxdomain.ID) (sandboxdomain.Run, error) {
	if s == nil || s.store == nil {
		return sandboxdomain.Run{}, fmt.Errorf("sandbox inventory unavailable")
	}
	return s.store.GetRun(ctx, id)
}

// ChangeLocalLifecycle governs local availability, never deleting workspace data.
// Remote/container adapters require their own explicit lifecycle implementations.
func (s *Sandboxes) ChangeLocalLifecycle(ctx context.Context, id sandboxdomain.ID, action governancedomain.Action, caller governancedomain.Context) (sandboxdomain.Run, error) {
	if s == nil || s.store == nil {
		return sandboxdomain.Run{}, sandboxdomain.ErrBackendUnavailable
	}
	if err := caller.Validate(); err != nil {
		return sandboxdomain.Run{}, err
	}
	item, err := s.store.Get(ctx, id)
	if err != nil {
		return sandboxdomain.Run{}, err
	}
	if item.Backend() != sandboxdomain.BackendLocalWorkspace {
		return sandboxdomain.Run{}, sandboxdomain.ErrBackendUnavailable
	}
	switch action {
	case governancedomain.ActionSandboxStart, governancedomain.ActionSandboxStop, governancedomain.ActionSandboxDestroy:
	default:
		return sandboxdomain.Run{}, fmt.Errorf("unsupported local Sandbox lifecycle action %q", action)
	}
	expected, err := s.store.GetRun(ctx, id)
	if err != nil {
		return sandboxdomain.Run{}, err
	}
	if err := s.admission.AdmitAction(ctx, caller, item, action); err != nil {
		return sandboxdomain.Run{}, err
	}
	if err := ctx.Err(); err != nil {
		return sandboxdomain.Run{}, err
	}
	if action == governancedomain.ActionSandboxStart {
		root := item.Spec().LocalRoot
		canonical, err := filepath.EvalSymlinks(root)
		if err != nil {
			return sandboxdomain.Run{}, fmt.Errorf("start local Sandbox: root unavailable: %w", err)
		}
		if canonical != root {
			return sandboxdomain.Run{}, fmt.Errorf("start local Sandbox: canonical root changed")
		}
		info, err := os.Stat(root)
		if err != nil {
			return sandboxdomain.Run{}, fmt.Errorf("start local Sandbox: root inspection: %w", err)
		}
		if !info.IsDir() {
			return sandboxdomain.Run{}, fmt.Errorf("start local Sandbox: root is not a directory")
		}
		if err := s.launcher.CheckIsolation(item.Isolation()); err != nil {
			return sandboxdomain.Run{}, fmt.Errorf("start local Sandbox: pinned isolation unavailable: %w", err)
		}
	}
	return s.store.ChangeLocalLifecycle(ctx, expected, action, caller)
}

func (s *Sandboxes) ActiveLeases(ctx context.Context, id sandboxdomain.ID) (int, error) {
	if s == nil || s.store == nil {
		return 0, sandboxdomain.ErrBackendUnavailable
	}
	return s.store.ActiveLeaseCount(ctx, id)
}

func (s *Sandboxes) LifecycleEvents(ctx context.Context, id sandboxdomain.ID, limit int) ([]sandboxsqlite.LifecycleEvent, error) {
	if s == nil || s.store == nil {
		return nil, sandboxdomain.ErrBackendUnavailable
	}
	return s.store.ListLifecycleEvents(ctx, id, limit)
}

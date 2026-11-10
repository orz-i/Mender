package bootstrap

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sync"

	governanceapplication "github.com/orz-i/Mender/backend/internal/governance/application"
	governancedomain "github.com/orz-i/Mender/backend/internal/governance/domain"
	invocationapplication "github.com/orz-i/Mender/backend/internal/invocation/application"
	pluginprocess "github.com/orz-i/Mender/backend/internal/plugin/process"
	profiledomain "github.com/orz-i/Mender/backend/internal/profile/domain"
	sandboxapplication "github.com/orz-i/Mender/backend/internal/sandbox/application"
	sandboxdomain "github.com/orz-i/Mender/backend/internal/sandbox/domain"
	sandboxdaytona "github.com/orz-i/Mender/backend/internal/sandbox/infrastructure/daytona"
	sandboxdocker "github.com/orz-i/Mender/backend/internal/sandbox/infrastructure/dockerengine"
	sandboxexternal "github.com/orz-i/Mender/backend/internal/sandbox/infrastructure/external"
	sandboxidentity "github.com/orz-i/Mender/backend/internal/sandbox/infrastructure/identity"
	sandboxlocalworkspace "github.com/orz-i/Mender/backend/internal/sandbox/infrastructure/localworkspace"
	sandboxpluginregistry "github.com/orz-i/Mender/backend/internal/sandbox/infrastructure/pluginregistry"
	sandboxrouter "github.com/orz-i/Mender/backend/internal/sandbox/infrastructure/router"
	sandboxsqlite "github.com/orz-i/Mender/backend/internal/sandbox/infrastructure/sqlite"
	sandboxssh "github.com/orz-i/Mender/backend/internal/sandbox/infrastructure/sshremote"
	"github.com/orz-i/Mender/backend/internal/sandbox/infrastructure/typedlease"
	v3schema "github.com/orz-i/Mender/backend/internal/schema/v3"
)

// Sandboxes owns the persistent Sandbox inventory and a fail-closed Execution
// Router. Provider/Tool execution is not redirected until the Workspace hard cut.
type Sandboxes struct {
	store                      *sandboxsqlite.Store
	leaseDB                    *sql.DB
	router                     *sandboxrouter.Router
	backendPlugins             *sandboxpluginregistry.Registry
	externalInstallationLookup sandboxexternal.InstallationLookup
	externalProcesses          *pluginprocess.Supervisor
	admission                  sandboxapplication.Admit
	launcher                   sandboxWorkspaceShellLauncher
	dockerMu                   sync.RWMutex
	dockerDatabasePath         string
	dockerAutoCancel           context.CancelFunc
	dockerAutoDone             chan struct{}
	dockerClosed               bool
	dockerReady                bool
	docker                     *sandboxdocker.Provisioner
	dockerTemplates            *sandboxdocker.TemplateStore
	dockerUnavailableReason    string
	sshRemote                  *sandboxssh.Backend
	daytona                    *sandboxdaytona.Controller
	daytonaTemplates           *sandboxdaytona.TemplateStore
	daytonaExperimental        bool
	profileExists              func(context.Context, profiledomain.ProfileID) error
}

// NewTypedSandboxes composes the official Sandbox application/router/backends
// and admits native leases only with reviewed v3 Profile/Workspace grants.
// Neither calling this constructor nor opening a typed DB starts a remote job.
func NewTypedSandboxes(ctx context.Context, databasePath string, policy sandboxapplication.AdmissionPolicy, approval governanceapplication.ApprovalGate, governance *RuntimeGovernance, credentials invocationapplication.CredentialResolver, profileExists func(context.Context, profiledomain.ProfileID) error) (*Sandboxes, error) {
	if policy == nil || approval == nil || governance == nil || governance.processIsolation == nil || profileExists == nil {
		return nil, fmt.Errorf("compose sandboxes: policy/approval unavailable")
	}
	store, err := sandboxsqlite.OpenTyped(ctx, databasePath)
	if err != nil {
		return nil, fmt.Errorf("compose typed sandboxes: %w", err)
	}
	leaseDB, err := v3schema.OpenContextStore(ctx, databasePath)
	if err != nil {
		_ = store.Close()
		return nil, fmt.Errorf("compose typed Sandbox lease: %w", err)
	}
	leaseStore, err := typedlease.New(ctx, leaseDB, invocationapplication.ApprovedAttemptVerifier{})
	if err != nil {
		_ = leaseDB.Close()
		_ = store.Close()
		return nil, fmt.Errorf("compose typed Sandbox lease: %w", err)
	}
	instance, err := composeSandboxes(ctx, databasePath, store, store, policy, approval, governance, credentials, profileExists)
	if err != nil {
		_ = leaseDB.Close()
		return nil, err
	}
	if err := instance.router.ConfigureNativeLeases(leaseStore); err != nil {
		_ = instance.Close()
		_ = leaseDB.Close()
		return nil, fmt.Errorf("compose typed Sandbox Native lease: %w", err)
	}
	instance.leaseDB = leaseDB
	return instance, nil
}

func composeSandboxes(ctx context.Context, databasePath string, store *sandboxsqlite.Store, leases sandboxapplication.ExecutionLeaseStore, policy sandboxapplication.AdmissionPolicy, approval governanceapplication.ApprovalGate, governance *RuntimeGovernance, credentials invocationapplication.CredentialResolver, profileExists func(context.Context, profiledomain.ProfileID) error) (*Sandboxes, error) {
	gate := sandboxapplication.NewAdmit(policy, approval, store, sandboxidentity.Generator{}, systemClock{}, store)
	launcher := sandboxWorkspaceShellLauncher{governance: governance}
	localBackend, err := sandboxlocalworkspace.New(launcher)
	if err != nil {
		_ = store.Close()
		return nil, fmt.Errorf("compose sandboxes: local workspace: %w", err)
	}
	component := &Sandboxes{store: store, admission: gate, launcher: launcher, profileExists: profileExists, dockerDatabasePath: databasePath, dockerUnavailableReason: "Docker Engine is not configured"}
	// The SSH built-in registers without connecting, resolving a key, or
	// contacting a remote host. Only an admitted Sandbox create/open does so.
	component.sshRemote = &sandboxssh.Backend{
		Dial: sandboxssh.Dialer{
			Credentials: credentials,
			Trust:       sandboxssh.NewHostTrustStore(filepath.Join(filepath.Dir(databasePath), "ssh_known_hosts")),
		},
		Templates: store,
	}
	backends := []sandboxapplication.Backend{localBackend, component.sshRemote}
	if dockerBackend, err := component.configureDocker(ctx, databasePath, gate); err == nil && dockerBackend != nil {
		backends = append(backends, dockerBackendGate{owner: component, backend: dockerBackend})
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
	registry := sandboxpluginregistry.NewRegistry()
	for _, backend := range backends {
		descriptor, descriptorErr := sandboxpluginregistry.BuiltinDescriptor(backend)
		if descriptorErr != nil {
			_ = component.Close()
			return nil, fmt.Errorf("compose sandbox backend extension descriptor: %w", descriptorErr)
		}
		if registerErr := registry.Register(backend, descriptor); registerErr != nil {
			_ = component.Close()
			return nil, fmt.Errorf("compose sandbox backend extension: %w", registerErr)
		}
	}
	router, err := sandboxrouter.New(store, store, gate, leases, registry)
	if err != nil {
		_ = component.Close()
		return nil, fmt.Errorf("compose sandboxes: router: %w", err)
	}
	component.router = router
	component.backendPlugins = registry
	if err := component.registerBuiltinLifecycle(backends); err != nil {
		_ = component.Close()
		return nil, fmt.Errorf("compose sandbox lifecycle controllers: %w", err)
	}
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
	s.dockerMu.Lock()
	s.dockerClosed = true
	s.dockerReady = false
	cancel, done := s.dockerAutoCancel, s.dockerAutoDone
	s.dockerMu.Unlock()
	if cancel != nil {
		cancel()
	}
	if done != nil {
		<-done
	}
	s.dockerMu.Lock()
	if s.docker != nil {
		_ = s.docker.Close()
	}
	if s.dockerTemplates != nil {
		_ = s.dockerTemplates.Close()
	}
	s.dockerMu.Unlock()
	if s.daytonaTemplates != nil {
		_ = s.daytonaTemplates.Close()
	}
	if s.leaseDB != nil {
		return errors.Join(s.store.Close(), s.leaseDB.Close())
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
	spec := sandboxdomain.Spec{ID: id, Name: request.Name, Backend: sandboxdomain.BackendLocalWorkspace, Binding: sandboxdomain.BuiltinBinding(sandboxdomain.BackendLocalWorkspace), Isolation: request.Isolation, LocalRoot: filepath.Clean(request.Root), OwnerKind: string(principal.Kind()), OwnerRef: principal.Reference()}
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

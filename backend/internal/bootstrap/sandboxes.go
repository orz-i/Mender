package bootstrap

import (
	"context"
	"fmt"
	"os"
	"path/filepath"

	governancedomain "github.com/orz-i/Mender/backend/internal/governance/domain"

	governanceapplication "github.com/orz-i/Mender/backend/internal/governance/application"
	sandboxapplication "github.com/orz-i/Mender/backend/internal/sandbox/application"
	sandboxdomain "github.com/orz-i/Mender/backend/internal/sandbox/domain"
	sandboxidentity "github.com/orz-i/Mender/backend/internal/sandbox/infrastructure/identity"
	sandboxlocalworkspace "github.com/orz-i/Mender/backend/internal/sandbox/infrastructure/localworkspace"
	sandboxrouter "github.com/orz-i/Mender/backend/internal/sandbox/infrastructure/router"
	sandboxsqlite "github.com/orz-i/Mender/backend/internal/sandbox/infrastructure/sqlite"
)

// Sandboxes owns the persistent Sandbox inventory and a fail-closed Execution
// Router. Provider/Tool execution is not redirected until the Workspace hard cut.
type Sandboxes struct {
	store     *sandboxsqlite.Store
	router    *sandboxrouter.Router
	admission sandboxapplication.Admit
	launcher  sandboxWorkspaceShellLauncher
}

func NewSandboxes(ctx context.Context, databasePath string, policy sandboxapplication.AdmissionPolicy, approval governanceapplication.ApprovalGate, governance *RuntimeGovernance) (*Sandboxes, error) {
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
	router, err := sandboxrouter.New(store, store, gate, store, localBackend)
	if err != nil {
		_ = store.Close()
		return nil, fmt.Errorf("compose sandboxes: router: %w", err)
	}
	return &Sandboxes{store: store, router: router, admission: gate, launcher: launcher}, nil
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

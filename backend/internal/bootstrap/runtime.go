package bootstrap

import (
	"context"
	"errors"
	"fmt"
	"io"
	"path/filepath"
	"strings"
	"sync"
	"time"

	agentapplication "github.com/orz-i/Mender/backend/internal/agent/application"
	agentdomain "github.com/orz-i/Mender/backend/internal/agent/domain"
	agentacp "github.com/orz-i/Mender/backend/internal/agent/infrastructure/acp"
	agentdelegation "github.com/orz-i/Mender/backend/internal/agent/infrastructure/delegationprovider"
	agentsqlite "github.com/orz-i/Mender/backend/internal/agent/infrastructure/sqlite"
	auditapplication "github.com/orz-i/Mender/backend/internal/audit/application"
	auditsqlite "github.com/orz-i/Mender/backend/internal/audit/infrastructure/sqlite"
	catalogdomain "github.com/orz-i/Mender/backend/internal/catalog/domain"
	credentialapplication "github.com/orz-i/Mender/backend/internal/credential/application"
	governancedomain "github.com/orz-i/Mender/backend/internal/governance/domain"
	governancelocaluser "github.com/orz-i/Mender/backend/internal/governance/infrastructure/localuser"
	invocationapplication "github.com/orz-i/Mender/backend/internal/invocation/application"
	invocationrouter "github.com/orz-i/Mender/backend/internal/invocation/infrastructure/router"
	platformplugin "github.com/orz-i/Mender/backend/internal/plugin"
	installationdomain "github.com/orz-i/Mender/backend/internal/plugin/installation/domain"
	installationlocalfs "github.com/orz-i/Mender/backend/internal/plugin/installation/infrastructure/localfs"
	installationsqlite "github.com/orz-i/Mender/backend/internal/plugin/installation/infrastructure/sqlite"
	policylocal "github.com/orz-i/Mender/backend/internal/policy/infrastructure/local"
	profiledomain "github.com/orz-i/Mender/backend/internal/profile/domain"
	provideractivationapplication "github.com/orz-i/Mender/backend/internal/provideractivation/application"
	providerplugin "github.com/orz-i/Mender/backend/internal/providerplugin"
	providerbuiltin "github.com/orz-i/Mender/backend/internal/providerplugin/builtin"
	providerconfigstore "github.com/orz-i/Mender/backend/internal/providerplugin/configstore"
	providerexternal "github.com/orz-i/Mender/backend/internal/providerplugin/external"
	sandboxdomain "github.com/orz-i/Mender/backend/internal/sandbox/domain"
	sandboxexternal "github.com/orz-i/Mender/backend/internal/sandbox/infrastructure/external"
	tunnelapplication "github.com/orz-i/Mender/backend/internal/tunnel/application"
	tunnelexternal "github.com/orz-i/Mender/backend/internal/tunnel/infrastructure/external"
	tunnelregistry "github.com/orz-i/Mender/backend/internal/tunnel/infrastructure/pluginregistry"
	tunnelprocess "github.com/orz-i/Mender/backend/internal/tunnel/infrastructure/process"
)

// RuntimeOptions configures the shared local execution runtime.
type RuntimeOptions struct {
	DatabasePath          string
	ProviderPluginRoot    string
	SkillRoot             string
	DefaultTimeout        time.Duration
	BindingExecutionGuard invocationapplication.BindingExecutionGuard
	InvocationObserver    invocationapplication.InvocationObserver
	TunnelProcessRunner   tunnelapplication.ProcessRunner
	// AgentExecutor is an in-process trusted testing seam; never expose it via
	// API, Provider config, persisted metadata, or untrusted Tool arguments.
	AgentExecutor agentapplication.Executor
}

// Runtime composes Catalog, Profile persistence, Provider config persistence,
// and Mender's shared invocation pipeline.
type Runtime struct {
	Catalog                     *Catalog
	Credentials                 *Credentials
	Profiles                    *Profiles
	Tunnels                     *Tunnels
	Policy                      *Policies
	Skills                      *Skills
	Sandboxes                   *Sandboxes
	Agents                      *agentapplication.Service
	RuntimeGovernance           *RuntimeGovernance
	InvokeTool                  invocationapplication.InvokeTool
	ListAudit                   auditapplication.ListRecords
	ListProviderActivationAudit auditapplication.ListProviderActivations

	audit                *auditsqlite.Store
	executors            *invocationrouter.Router
	providerPlugins      *providerplugin.Registry
	providerResolver     providerplugin.Resolver
	externalResolver     *providerexternal.Resolver
	providerConfigs      *providerconfigstore.Store
	pluginInstallations  *installationsqlite.Store
	pluginPackages       *installationlocalfs.Store
	externalProcesses    *providerexternal.Supervisor
	tunnelPlugins        *tunnelregistry.Registry
	providerInstances    *providerplugin.Manager
	providerDatabasePath string
	providerActivation   provideractivationapplication.Admit
	ownerPrincipal       governancedomain.Principal
	agentStore           *agentsqlite.Store
	agentLauncher        *agentacp.LocalFactory

	closersMu sync.Mutex
	closers   []io.Closer

	mcpServersMu sync.Mutex
	mcpServers   map[string]*managedMCPServer
}

// NewRuntime constructs the protocol-independent local runtime.
func NewRuntime(ctx context.Context, options RuntimeOptions) (*Runtime, error) {
	ownerPrincipal, err := governancelocaluser.CurrentPrincipal()
	if err != nil {
		return nil, fmt.Errorf("compose runtime: principal: %w", err)
	}

	catalog, err := NewCatalog(ctx, CatalogOptions{DatabasePath: options.DatabasePath})
	if err != nil {
		return nil, fmt.Errorf("compose runtime: catalog: %w", err)
	}

	auditStore, err := auditsqlite.Open(ctx, options.DatabasePath)
	if err != nil {
		_ = catalog.Close()
		return nil, fmt.Errorf("compose runtime: audit: %w", err)
	}

	credentials, err := NewCredentials(ctx, options.DatabasePath)
	if err != nil {
		_ = auditStore.Close()
		_ = catalog.Close()
		return nil, fmt.Errorf("compose runtime: credentials: %w", err)
	}

	tunnelRunner := options.TunnelProcessRunner
	tunnels, err := NewTunnels(
		ctx,
		options.DatabasePath,
		profileCredentialLookup{get: credentials.Get},
		credentials.Resolver,
		tunnelRunner,
	)
	if err != nil {
		_ = credentials.Close()
		_ = auditStore.Close()
		_ = catalog.Close()
		return nil, fmt.Errorf("compose runtime: tunnels: %w", err)
	}
	tunnelsOwned := true
	defer func() {
		if tunnelsOwned {
			_ = tunnels.Close()
		}
	}()

	profiles, err := NewProfilesWithTunnelLookup(
		ctx,
		options.DatabasePath,
		catalog.store,
		profileCredentialLookup{get: credentials.Get},
		profileTunnelLookup{get: tunnels.Get},
	)
	if err != nil {
		_ = credentials.Close()
		_ = auditStore.Close()
		_ = catalog.Close()
		return nil, fmt.Errorf("compose runtime: profiles: %w", err)
	}
	tunnels.Delete = tunnelapplication.NewDeleteTunnel(
		tunnels.store,
		runtimeTunnelUsageChecker{profiles: profiles},
	)

	policies, err := NewPolicies(ctx, options.DatabasePath, catalog, profiles)
	if err != nil {
		_ = profiles.Close()
		_ = credentials.Close()
		_ = auditStore.Close()
		_ = catalog.Close()
		return nil, fmt.Errorf("compose runtime: policy: %w", err)
	}

	providerConfigs, err := providerconfigstore.Open(ctx, options.DatabasePath)
	if err != nil {
		_ = policies.Close()
		_ = profiles.Close()
		_ = credentials.Close()
		_ = auditStore.Close()
		_ = catalog.Close()
		return nil, fmt.Errorf("compose runtime: provider configs: %w", err)
	}

	runtimeGovernance, err := NewRuntimeGovernance(ctx, options.DatabasePath)
	if err != nil {
		_ = providerConfigs.Close()
		_ = policies.Close()
		_ = profiles.Close()
		_ = credentials.Close()
		_ = auditStore.Close()
		_ = catalog.Close()
		return nil, fmt.Errorf("compose runtime: runtime governance: %w", err)
	}
	runtimeGovernanceOwned := true
	defer func() {
		if runtimeGovernanceOwned {
			_ = runtimeGovernance.Close()
		}
	}()
	if tunnelRunner == nil {
		tunnelRunner = tunnelprocess.NewRunner(
			tunnelClientIsolationLauncher{governance: runtimeGovernance},
		)
		tunnels.Start = tunnelapplication.NewStartTunnel(
			tunnels.store,
			credentials.Resolver,
			tunnelRunner,
		)
	}

	// Only the early Sandbox/migration failure paths use this cleanup. Once
	// normal composition proceeds, the established owner/closer chain applies.
	closeBootstrapStores := func() {
		_ = providerConfigs.Close()
		_ = policies.Close()
		_ = profiles.Close()
		_ = credentials.Close()
		_ = auditStore.Close()
		_ = catalog.Close()
	}

	// Sandboxes are composed before Provider plugins so Workspace activation
	// only observes post-cut Sandbox bindings. Migration is one DB transaction.
	approvalGate := policylocal.RejectApproval{}
	sandboxes, err := NewSandboxes(ctx, options.DatabasePath, policies.Evaluate, approvalGate, runtimeGovernance, credentials.Resolver, func(ctx context.Context, id profiledomain.ProfileID) error {
		_, err := profiles.GetByID.Execute(ctx, id)
		return err
	})
	if err != nil {
		closeBootstrapStores()
		return nil, fmt.Errorf("compose runtime: sandbox inventory: %w", err)
	}
	sandboxesOwned := true
	defer func() {
		if sandboxesOwned {
			_ = sandboxes.Close()
		}
	}()
	if err := installSandboxProviderBindingGuards(ctx, options.DatabasePath); err != nil {
		closeBootstrapStores()
		return nil, fmt.Errorf("compose runtime: Workspace Sandbox binding guards: %w", err)
	}
	executors := invocationrouter.New()
	providerPlugins := providerplugin.NewRegistry()
	if err := providerbuiltin.Register(
		providerPlugins,
		sandboxes.router,
		sandboxes,
	); err != nil {
		_ = providerConfigs.Close()
		_ = policies.Close()
		_ = profiles.Close()
		_ = credentials.Close()
		_ = auditStore.Close()
		_ = catalog.Close()
		return nil, fmt.Errorf("compose runtime: provider plugins: %w", err)
	}
	pluginInstallations, err := installationsqlite.Open(ctx, options.DatabasePath)
	if err != nil {
		_ = providerConfigs.Close()
		_ = policies.Close()
		_ = profiles.Close()
		_ = credentials.Close()
		_ = auditStore.Close()
		_ = catalog.Close()
		return nil, fmt.Errorf("compose runtime: plugin installations: %w", err)
	}
	pluginRoot, err := resolveProviderPluginRoot(
		options.DatabasePath,
		options.ProviderPluginRoot,
	)
	if err != nil {
		_ = pluginInstallations.Close()
		_ = providerConfigs.Close()
		_ = policies.Close()
		_ = profiles.Close()
		_ = credentials.Close()
		_ = auditStore.Close()
		_ = catalog.Close()
		return nil, fmt.Errorf("compose runtime: plugin root: %w", err)
	}
	pluginPackages, err := installationlocalfs.New(pluginRoot)
	if err != nil {
		_ = pluginInstallations.Close()
		_ = providerConfigs.Close()
		_ = policies.Close()
		_ = profiles.Close()
		_ = credentials.Close()
		_ = auditStore.Close()
		_ = catalog.Close()
		return nil, fmt.Errorf("compose runtime: plugin packages: %w", err)
	}
	externalProcesses, err := providerexternal.NewSupervisor(
		pluginPackages,
		externalProviderIsolationLauncher{governance: runtimeGovernance},
		providerexternal.DefaultOptions(),
	)
	if err != nil {
		_ = pluginInstallations.Close()
		_ = providerConfigs.Close()
		_ = policies.Close()
		_ = profiles.Close()
		_ = credentials.Close()
		_ = auditStore.Close()
		_ = catalog.Close()
		return nil, fmt.Errorf("compose runtime: external plugin processes: %w", err)
	}
	// Register the independent Sandbox Plugin Protocol on the same package
	// supervisor before any runtime Provider activation opens a plugin.
	sandboxDialer, err := sandboxexternal.NewDialer(externalProviderIsolationLauncher{governance: runtimeGovernance})
	if err == nil {
		err = externalProcesses.ProcessHost().Register(platformplugin.SandboxBackend, sandboxDialer)
	}
	if err != nil {
		_ = externalProcesses.CloseAll()
		_ = pluginInstallations.Close()
		closeBootstrapStores()
		return nil, fmt.Errorf("compose sandbox plugin dialer: %w", err)
	}
	tunnelDialer, err := tunnelexternal.NewDialer(externalProviderIsolationLauncher{governance: runtimeGovernance})
	if err == nil {
		err = externalProcesses.ProcessHost().Register(platformplugin.TunnelTransport, tunnelDialer)
	}
	if err != nil {
		_ = externalProcesses.CloseAll()
		_ = pluginInstallations.Close()
		closeBootstrapStores()
		return nil, fmt.Errorf("compose Tunnel Transport protocol: %w", err)
	}
	externalTunnelRunner, err := tunnelexternal.NewRunner(pluginInstallations, externalProcesses.ProcessHost())
	if err != nil {
		_ = externalProcesses.CloseAll()
		_ = pluginInstallations.Close()
		closeBootstrapStores()
		return nil, err
	}
	tunnelRegistry, err := tunnelregistry.New(tunnelRunner, externalTunnelRunner)
	if err != nil {
		_ = externalProcesses.CloseAll()
		_ = pluginInstallations.Close()
		closeBootstrapStores()
		return nil, err
	}
	if err := tunnels.ConfigureExternal(pluginInstallations, externalTunnelRunner); err != nil {
		_ = externalProcesses.CloseAll()
		_ = pluginInstallations.Close()
		closeBootstrapStores()
		return nil, err
	}
	tunnels.Start = tunnelapplication.NewStartTunnel(tunnels.store, credentials.Resolver, tunnelRegistry)
	sandboxes.externalInstallationLookup = pluginInstallations
	sandboxes.externalProcesses = externalProcesses.ProcessHost()
	installedPlugins, err := pluginInstallations.List(ctx)
	if err != nil {
		_ = externalProcesses.CloseAll()
		_ = pluginInstallations.Close()
		closeBootstrapStores()
		return nil, fmt.Errorf("compose installed plugins: %w", err)
	}
	for _, item := range installedPlugins {
		if item.Extension().Kind != platformplugin.SandboxBackend ||
			item.Status() != installationdomain.StatusEnabled {
			continue
		}
		if err := sandboxes.RegisterExternalSandboxBackend(item, pluginInstallations, externalProcesses.ProcessHost()); err != nil {
			_ = externalProcesses.CloseAll()
			_ = pluginInstallations.Close()
			closeBootstrapStores()
			return nil, fmt.Errorf("compose external Sandbox backend %s: %w", item.ID(), err)
		}
	}
	externalResolver, err := providerexternal.NewResolver(
		providerPlugins,
		pluginInstallations,
		externalProcesses,
	)
	if err != nil {
		_ = externalProcesses.CloseAll()
		_ = pluginInstallations.Close()
		_ = providerConfigs.Close()
		_ = policies.Close()
		_ = profiles.Close()
		_ = credentials.Close()
		_ = auditStore.Close()
		_ = catalog.Close()
		return nil, fmt.Errorf("compose runtime: provider resolver: %w", err)
	}
	var providerResolver providerplugin.Resolver = externalResolver

	credentials.Delete = credentialapplication.NewDeleteCredential(
		credentials.store,
		credentials.secrets,
		runtimeCredentialUsageChecker{
			profiles:  profiles,
			tunnels:   tunnels,
			sandboxes: sandboxes,
			configs:   providerConfigs,
		},
	)

	providerInstances, err := providerplugin.NewManager(
		providerResolver,
		providerConfigs,
		executors,
		credentials.Resolver,
	)
	if err != nil {
		_ = externalProcesses.CloseAll()
		_ = pluginInstallations.Close()
		_ = providerConfigs.Close()
		_ = policies.Close()
		_ = profiles.Close()
		_ = credentials.Close()
		_ = auditStore.Close()
		_ = catalog.Close()
		return nil, fmt.Errorf("compose runtime: provider instances: %w", err)
	}
	skillRoot, err := resolveSkillRoot(options.DatabasePath, options.SkillRoot)
	if err != nil {
		_ = providerInstances.CloseAll()
		_ = externalProcesses.CloseAll()
		_ = pluginInstallations.Close()
		_ = providerConfigs.Close()
		_ = policies.Close()
		_ = profiles.Close()
		_ = credentials.Close()
		_ = auditStore.Close()
		_ = catalog.Close()
		return nil, fmt.Errorf("compose runtime: skill root: %w", err)
	}
	skills, err := NewSkills(ctx, options.DatabasePath, skillRoot)
	if err != nil {
		_ = providerInstances.CloseAll()
		_ = externalProcesses.CloseAll()
		_ = pluginInstallations.Close()
		_ = providerConfigs.Close()
		_ = policies.Close()
		_ = profiles.Close()
		_ = credentials.Close()
		_ = auditStore.Close()
		_ = catalog.Close()
		return nil, fmt.Errorf("compose runtime: skills: %w", err)
	}

	providerActivation, invokeTool := (runtimeInvocationWiring{
		options:         options,
		catalog:         catalog,
		profiles:        profiles,
		providerConfigs: providerConfigs,
		plugins:         providerPlugins,
		policy:          policies,
		governance:      runtimeGovernance,
		executors:       executors,
		credentials:     credentials,
		audit:           auditStore,
		approval:        approvalGate,
	}).compose()

	agents, agentStore, agentLauncher, err := newAgents(ctx, options.DatabasePath, ownerPrincipal, profiles, policies, runtimeGovernance, options.AgentExecutor)
	if err != nil {
		_ = skills.Close()
		_ = providerInstances.CloseAll()
		_ = externalProcesses.CloseAll()
		_ = pluginInstallations.Close()
		closeBootstrapStores()
		return nil, fmt.Errorf("compose runtime: Agent backend: %w", err)
	}

	if err := agents.SetWorkspaceResolver(func(ctx context.Context, id string) (agentapplication.WorkspaceTarget, error) {
		parsed, err := sandboxdomain.ParseID(id)
		if err != nil {
			return agentapplication.WorkspaceTarget{}, err
		}
		sandbox, err := sandboxes.Get(ctx, parsed)
		if err != nil {
			return agentapplication.WorkspaceTarget{}, err
		}
		run, err := sandboxes.GetRun(ctx, parsed)
		if err != nil || run.State != sandboxdomain.StateReady {
			return agentapplication.WorkspaceTarget{}, fmt.Errorf("%w: local workspace is not ready", agentdomain.ErrUnavailable)
		}
		sp := sandbox.Spec()
		return agentapplication.WorkspaceTarget{ID: id, Root: sp.LocalRoot, Backend: string(sp.Backend), Isolation: string(sp.Isolation), OwnerKind: sp.OwnerKind, OwnerRef: sp.OwnerRef}, nil
	}); err != nil {
		return nil, err
	}

	// The Tool facade is only registered after its Agent service is ready.
	// Registration is inert; Provider activation and Task submission retain
	// their independent governance gates.
	if err := providerPlugins.Register(agentdelegation.New(agents, ownerPrincipal)); err != nil {
		_ = agents.Close()
		_ = agentStore.Close()
		_ = skills.Close()
		_ = providerInstances.CloseAll()
		_ = externalProcesses.CloseAll()
		_ = pluginInstallations.Close()
		closeBootstrapStores()
		return nil, fmt.Errorf("compose runtime: Agent delegation Provider: %w", err)
	}
	tunnelsOwned = false
	runtimeGovernanceOwned = false
	sandboxesOwned = false
	result := &Runtime{
		Catalog:                     catalog,
		Credentials:                 credentials,
		Profiles:                    profiles,
		Tunnels:                     tunnels,
		Policy:                      policies,
		Skills:                      skills,
		Sandboxes:                   sandboxes,
		Agents:                      agents,
		RuntimeGovernance:           runtimeGovernance,
		InvokeTool:                  invokeTool,
		ListAudit:                   auditapplication.NewListRecords(auditStore),
		ListProviderActivationAudit: auditapplication.NewListProviderActivations(auditStore),
		audit:                       auditStore,
		executors:                   executors,
		providerPlugins:             providerPlugins,
		providerResolver:            providerResolver,
		externalResolver:            externalResolver,
		providerConfigs:             providerConfigs,
		pluginInstallations:         pluginInstallations,
		pluginPackages:              pluginPackages,
		externalProcesses:           externalProcesses,
		tunnelPlugins:               tunnelRegistry,
		providerInstances:           providerInstances,
		providerDatabasePath:        options.DatabasePath,
		providerActivation:          providerActivation,
		ownerPrincipal:              ownerPrincipal,
		agentStore:                  agentStore,
		agentLauncher:               agentLauncher,
		mcpServers:                  make(map[string]*managedMCPServer),
	}
	tunnels.Delete = tunnelapplication.NewDeleteTunnel(tunnels.store, runtimeTunnelUsageChecker{
		profiles: profiles, active: result.isTunnelActive,
	})
	// The shared DB is the final authority for cross-context Tool/Policy refs.
	// Register persistence guards only after every bounded-context table exists.
	if err := installProviderReferenceGuards(ctx, options.DatabasePath); err != nil {
		return nil, errors.Join(fmt.Errorf("compose runtime: provider reference guards: %w", err), result.Close())
	}
	return result, nil
}

// RegisterExecutor binds a canonical Provider to its execution adapter.
func (r *Runtime) RegisterExecutor(
	providerID catalogdomain.ProviderID,
	executor invocationapplication.ToolExecutor,
) error {
	return r.executors.Register(providerID, executor)
}

func resolveSkillRoot(databasePath, explicit string) (string, error) {
	return resolveManagedRoot(databasePath, explicit, "skills", "skill")
}

func resolveProviderPluginRoot(databasePath, explicit string) (string, error) {
	return resolveManagedRoot(databasePath, explicit, "plugins", "provider plugin")
}

func resolveManagedRoot(
	databasePath string,
	explicit string,
	defaultName string,
	label string,
) (string, error) {
	if value := strings.TrimSpace(explicit); value != "" {
		absolute, err := filepath.Abs(value)
		if err != nil {
			return "", fmt.Errorf("resolve %s root: %w", label, err)
		}
		return absolute, nil
	}
	if strings.TrimSpace(databasePath) == "" {
		return "", fmt.Errorf("resolve %s root: database path must not be empty", label)
	}
	absoluteDatabase, err := filepath.Abs(strings.TrimSpace(databasePath))
	if err != nil {
		return "", fmt.Errorf("resolve %s database path: %w", label, err)
	}
	return filepath.Join(filepath.Dir(absoluteDatabase), defaultName), nil
}

func (r *Runtime) addCloser(closer io.Closer) {
	if closer == nil {
		return
	}
	r.closersMu.Lock()
	defer r.closersMu.Unlock()
	r.closers = append(r.closers, closer)
}

// Close releases all runtime-owned infrastructure.
func (r *Runtime) Close() error {
	agentErr := r.Agents.Close()
	serverErr := r.closeManagedMCPServers()
	providerErr := r.providerInstances.CloseAll()
	processErr := r.externalProcesses.CloseAll()

	r.closersMu.Lock()
	closers := append([]io.Closer(nil), r.closers...)
	r.closers = nil
	r.closersMu.Unlock()

	errs := make([]error, 0, len(closers)+9)
	errs = append(errs, agentErr, serverErr, providerErr, processErr)
	for i := len(closers) - 1; i >= 0; i-- {
		errs = append(errs, closers[i].Close())
	}
	errs = append(
		errs,
		r.agentStore.Close(),
		r.Skills.Close(),
		r.Sandboxes.Close(),
		r.RuntimeGovernance.Close(),
		r.pluginInstallations.Close(),
		r.providerConfigs.Close(),
		r.Policy.Close(),
		r.Profiles.Close(),
		r.Tunnels.Close(),
		r.Credentials.Close(),
		r.audit.Close(),
		r.Catalog.Close(),
	)
	return errors.Join(errs...)
}

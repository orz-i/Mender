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
	mcpmanaged "github.com/orz-i/Mender/backend/internal/mcp/infrastructure/managed"
	mcpplugin "github.com/orz-i/Mender/backend/internal/mcp/infrastructure/plugin"
	hardcut "github.com/orz-i/Mender/backend/internal/migration/toolhardcut"
	platformplugin "github.com/orz-i/Mender/backend/internal/plugin"
	installationdomain "github.com/orz-i/Mender/backend/internal/plugin/installation/domain"
	installationlocalfs "github.com/orz-i/Mender/backend/internal/plugin/installation/infrastructure/localfs"
	installationsqlite "github.com/orz-i/Mender/backend/internal/plugin/installation/infrastructure/sqlite"
	policylocal "github.com/orz-i/Mender/backend/internal/policy/infrastructure/local"
	profileapplication "github.com/orz-i/Mender/backend/internal/profile/application"
	profiledelivery "github.com/orz-i/Mender/backend/internal/profile/application/delivery"
	profiledomain "github.com/orz-i/Mender/backend/internal/profile/domain"
	profilesupply "github.com/orz-i/Mender/backend/internal/profile/infrastructure/typedsupply"
	provideractivationapplication "github.com/orz-i/Mender/backend/internal/provideractivation/application"
	providerplugin "github.com/orz-i/Mender/backend/internal/providerplugin"
	providerbuiltin "github.com/orz-i/Mender/backend/internal/providerplugin/builtin"
	providerconfigstore "github.com/orz-i/Mender/backend/internal/providerplugin/configstore"
	providerexternal "github.com/orz-i/Mender/backend/internal/providerplugin/external"
	sandboxexternal "github.com/orz-i/Mender/backend/internal/sandbox/infrastructure/external"
	sshjobsplugin "github.com/orz-i/Mender/backend/internal/sshjobs/infrastructure/plugin"
	tunnelapplication "github.com/orz-i/Mender/backend/internal/tunnel/application"
	tunnelexternal "github.com/orz-i/Mender/backend/internal/tunnel/infrastructure/external"
	tunnelregistry "github.com/orz-i/Mender/backend/internal/tunnel/infrastructure/pluginregistry"
	tunnelprocess "github.com/orz-i/Mender/backend/internal/tunnel/infrastructure/process"
	workspaceapp "github.com/orz-i/Mender/backend/internal/workspace/application"
	workspaceplugin "github.com/orz-i/Mender/backend/internal/workspace/infrastructure/plugin"
	"github.com/orz-i/Mender/backend/internal/workspace/infrastructure/sandboxbridge"
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
	MCPDelivery                 *MCPDelivery
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
	profileDelivery      *profiledelivery.Service
	ownerPrincipal       governancedomain.Principal
	agentStore           *agentsqlite.Store
	agentLauncher        *agentacp.LocalFactory
	agentToolGateway     agentTaskToolGateway

	closersMu sync.Mutex
	closers   []io.Closer

	typedSupply        *profilesupply.Reader
	workspaceInventory *sandboxbridge.TypedInventory
	managedMCP         *mcpmanaged.Manager
}

// NewRuntime constructs the protocol-independent local runtime.
func NewRuntime(ctx context.Context, options RuntimeOptions) (*Runtime, error) {
	ownerPrincipal, err := governancelocaluser.CurrentPrincipal()
	if err != nil {
		return nil, fmt.Errorf("compose runtime: principal: %w", err)
	}

	// The official CLI, menderd and Wails entry accepts the COMPLETE typed v3
	// schema only. A v2 or partially migrated DB is never repaired in place:
	// operators must back up and explicitly replace v2 data; no implicit migration.
	if err := ensureFreshTypedDatabase(options.DatabasePath); err != nil {
		return nil, fmt.Errorf("compose runtime: v3 fresh schema: %w", err)
	}
	preflight, err := hardcut.OpenTypedContextStore(ctx, options.DatabasePath)
	if err != nil {
		return nil, fmt.Errorf("compose runtime: v3 schema required: %w", err)
	}
	_ = preflight.Close()

	catalog, err := NewTypedCatalog(ctx, CatalogOptions{DatabasePath: options.DatabasePath})
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

	profiles, err := NewTypedProfiles(
		ctx,
		options.DatabasePath,
		catalog.store,
		profileCredentialLookup{get: credentials.Get},
	)
	if err != nil {
		_ = credentials.Close()
		_ = auditStore.Close()
		_ = catalog.Close()
		return nil, fmt.Errorf("compose runtime: profiles: %w", err)
	}
	mcpDelivery, err := newTypedMCPDelivery(ctx, options.DatabasePath, profiles.GetByID, tunnels.Get)
	if err != nil {
		_ = profiles.Close()
		_ = credentials.Close()
		_ = auditStore.Close()
		_ = catalog.Close()
		return nil, fmt.Errorf("compose runtime: MCP delivery: %w", err)
	}
	mcpDeliveryOwned := true
	defer func() {
		if mcpDeliveryOwned {
			_ = mcpDelivery.Close()
		}
	}()
	tunnels.Delete = tunnelapplication.NewDeleteTunnel(
		tunnels.store,
		runtimeTunnelUsageChecker{profiles: profiles, delivery: mcpDelivery},
	)

	policies, err := NewTypedPolicies(ctx, options.DatabasePath, catalog, profiles)
	if err != nil {
		_ = profiles.Close()
		_ = credentials.Close()
		_ = auditStore.Close()
		_ = catalog.Close()
		return nil, fmt.Errorf("compose runtime: policy: %w", err)
	}

	providerConfigs, err := providerconfigstore.OpenTyped(ctx, options.DatabasePath)
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
	sandboxes, err := NewTypedSandboxes(ctx, options.DatabasePath, policies.Evaluate, approvalGate, runtimeGovernance, credentials.Resolver, func(ctx context.Context, id profiledomain.ProfileID) error {
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
	executors := invocationrouter.New()
	providerPlugins := providerplugin.NewRegistry()
	if err := providerbuiltin.Register(providerPlugins); err != nil {
		_ = providerConfigs.Close()
		_ = policies.Close()
		_ = profiles.Close()
		_ = credentials.Close()
		_ = auditStore.Close()
		_ = catalog.Close()
		return nil, fmt.Errorf("compose runtime: provider plugins: %w", err)
	}
	// OAuth is a host-managed Credential lifecycle, not a generic Provider
	// secret resolver. Inject its per-request token source into the built-in
	// MCP plugin once before any Provider activation.
	builtinMCP, err := providerPlugins.Lookup(mcpplugin.ID)
	if err != nil {
		closeBootstrapStores()
		return nil, fmt.Errorf("compose runtime: locate MCP plugin: %w", err)
	}
	mcpInstance, ok := builtinMCP.(*mcpplugin.Plugin)
	if !ok {
		closeBootstrapStores()
		return nil, fmt.Errorf("compose runtime: unexpected built-in MCP plugin type")
	}
	mcpInstance.SetOAuthTokenProvider(credentials.OAuthFlow)
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

	if err := profiles.store.EnsureSkillReferenceGuards(ctx); err != nil {
		_ = skills.Close()
		_ = providerInstances.CloseAll()
		_ = externalProcesses.CloseAll()
		_ = pluginInstallations.Close()
		_ = providerConfigs.Close()
		_ = policies.Close()
		_ = profiles.Close()
		_ = credentials.Close()
		_ = auditStore.Close()
		_ = catalog.Close()
		return nil, fmt.Errorf("compose runtime: Profile Skill reference guards: %w", err)
	}
	profiles.ReplaceSkills = profileapplication.NewReplaceSkills(profiles.store, profileSkillStatus{skills: skills})
	skills.profileSkillUsage = profiles.store.ListSkillProfiles

	// Compose a single SourceRef-aware Invocation, using the same persisted
	// grants and lease ledger as the typed Catalog/Profiles/Sandboxes.
	typedReader, err := profilesupply.Open(ctx, options.DatabasePath)
	if err != nil {
		_ = skills.Close()
		_ = providerInstances.CloseAll()
		_ = externalProcesses.CloseAll()
		_ = pluginInstallations.Close()
		closeBootstrapStores()
		return nil, fmt.Errorf("compose runtime: typed Profile supply: %w", err)
	}
	typedInventoryDB, err := hardcut.OpenTypedContextStore(ctx, options.DatabasePath)
	if err != nil {
		_ = typedReader.Close()
		_ = skills.Close()
		_ = providerInstances.CloseAll()
		_ = externalProcesses.CloseAll()
		_ = pluginInstallations.Close()
		closeBootstrapStores()
		return nil, fmt.Errorf("compose runtime: Workspace placement authority: %w", err)
	}
	inventory, err := sandboxbridge.NewTypedInventory(ctx, typedInventoryDB)
	if err != nil {
		_ = typedInventoryDB.Close()
		_ = typedReader.Close()
		_ = skills.Close()
		_ = providerInstances.CloseAll()
		_ = externalProcesses.CloseAll()
		_ = pluginInstallations.Close()
		closeBootstrapStores()
		return nil, fmt.Errorf("compose runtime: Workspace inventory: %w", err)
	}
	selection, err := workspaceapp.NewWorkspaceSelection(inventory)
	if err != nil {
		return nil, fmt.Errorf("compose runtime: workspace selection: %w", err)
	}
	placements, err := workspaceapp.NewPlacementQuery(inventory)
	if err != nil {
		return nil, fmt.Errorf("compose runtime: Workspace placement: %w", err)
	}
	guarded, err := sandboxbridge.NewGuardedRouter(sandboxbridge.GuardedDependencies{
		Placements: placements, Router: sandboxes.router, Backends: sandboxes.backendPlugins,
		Grants: sandboxes.store, Workspaces: inventory, Bindings: inventory,
		Roots: sandboxbridge.LocalRootAttestor{}, Admissions: invocationapplication.ApprovedAttemptVerifier{},
	})
	if err != nil {
		return nil, fmt.Errorf("compose runtime: guarded Workspace Router: %w", err)
	}
	workspaceSource, err := catalogdomain.NativeSource("workspace")
	if err != nil {
		return nil, err
	}
	if err := executors.RegisterSource(workspaceSource, workspaceplugin.NativeWorkspaceTools{
		Selection: selection, Router: guarded,
	}); err != nil {
		return nil, fmt.Errorf("compose runtime: Native Workspace Tool: %w", err)
	}
	// Native SSH Jobs use the existing Sandbox JobID/remote recovery service,
	// with the typed Profile TargetScope checked independently of Sandbox grants.
	jobSource, err := catalogdomain.NativeSource("jobs")
	if err != nil {
		return nil, err
	}
	jobExecutor, err := sshjobsplugin.NewNativeJobs(ctx, typedInventoryDB, sandboxes, sandboxes.Get)
	if err != nil {
		return nil, fmt.Errorf("compose runtime: Native SSH Job Tool: %w", err)
	}
	if err = executors.RegisterSource(jobSource, jobExecutor); err != nil {
		return nil, fmt.Errorf("compose runtime: register Native SSH Job Tool: %w", err)
	}

	providerActivation, invokeTool := (runtimeInvocationWiring{
		options:       options,
		tools:         catalog.store,
		profileScopes: typedReader,
		facts:         newInvocationGovernanceFactExtractor(providerConfigs, providerPlugins, executors),
		policy:        policies,
		governance:    runtimeGovernance,
		executors:     executors,
		credentials:   credentials,
		audit:         auditStore,
		approval:      approvalGate,
	}).compose()

	agents, agentStore, agentLauncher, err := newTypedAgents(ctx, options.DatabasePath, ownerPrincipal, profiles, policies, runtimeGovernance, options.AgentExecutor)
	if err != nil {
		_ = skills.Close()
		_ = providerInstances.CloseAll()
		_ = externalProcesses.CloseAll()
		_ = pluginInstallations.Close()
		closeBootstrapStores()
		return nil, fmt.Errorf("compose runtime: Agent backend: %w", err)
	}

	listedAgents, err := agentStore.ListAgents(ctx)
	if err != nil {
		return nil, fmt.Errorf("compose runtime: list owned Agents: %w", err)
	}
	for _, owned := range listedAgents {
		source, sourceErr := catalogdomain.AgentSource(string(owned.ID()))
		if sourceErr != nil {
			return nil, fmt.Errorf("compose runtime: Agent source: %w", sourceErr)
		}
		ownedExecutor, sourceErr := agentdelegation.NewOwnedExecutor(agents, owned.ID())
		if sourceErr != nil {
			return nil, fmt.Errorf("compose runtime: Agent executor: %w", sourceErr)
		}
		if sourceErr = executors.RegisterSource(source, ownedExecutor); sourceErr != nil {
			return nil, fmt.Errorf("compose runtime: bind Agent source: %w", sourceErr)
		}
	}
	tunnelsOwned = false
	runtimeGovernanceOwned = false
	sandboxesOwned = false
	result := &Runtime{
		Catalog:                     catalog,
		Credentials:                 credentials,
		Profiles:                    profiles,
		MCPDelivery:                 mcpDelivery,
		Tunnels:                     tunnels,
		Policy:                      policies,
		Skills:                      skills,
		Sandboxes:                   sandboxes,
		Agents:                      agents,
		RuntimeGovernance:           runtimeGovernance,
		InvokeTool:                  invokeTool,
		ListAudit:                   auditapplication.NewListRecords(auditStore),
		typedSupply:                 typedReader,
		workspaceInventory:          inventory,
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
		profileDelivery:             newProfileDelivery(profiles, skills, options.DatabasePath),
		ownerPrincipal:              ownerPrincipal,
		agentStore:                  agentStore,
		agentLauncher:               agentLauncher,
	}
	mcpDeliveryOwned = false
	result.addCloser(typedInventoryDB)
	result.addCloser(typedReader)
	if err := result.configureManagedMCP(result.MCPDelivery.Get.Execute, result.MCPDelivery.GetRevision.Execute); err != nil {
		return nil, errors.Join(fmt.Errorf("compose runtime: managed MCP: %w", err), result.Close())
	}
	if err := result.Agents.ConfigureToolSupplyResolver(result.resolveAgentToolSupply); err != nil {
		return nil, errors.Join(fmt.Errorf("compose runtime: Agent tool delivery: %w", err), result.Close())
	}
	tunnels.Delete = tunnelapplication.NewDeleteTunnel(tunnels.store, runtimeTunnelUsageChecker{
		profiles: profiles, delivery: mcpDelivery, active: result.isTunnelActive,
	})
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
	r.closeAgentTaskToolMCPServers()
	r.closeAgentTaskToolGateway()
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
		r.MCPDelivery.Close(),
		r.Tunnels.Close(),
		r.Credentials.Close(),
		r.audit.Close(),
		r.Catalog.Close(),
	)
	return errors.Join(errs...)
}

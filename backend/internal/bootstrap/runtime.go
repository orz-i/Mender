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

	auditapplication "github.com/orz-i/Mender/backend/internal/audit/application"
	auditsqlite "github.com/orz-i/Mender/backend/internal/audit/infrastructure/sqlite"
	catalogdomain "github.com/orz-i/Mender/backend/internal/catalog/domain"
	credentialapplication "github.com/orz-i/Mender/backend/internal/credential/application"
	governancedomain "github.com/orz-i/Mender/backend/internal/governance/domain"
	governancelocaluser "github.com/orz-i/Mender/backend/internal/governance/infrastructure/localuser"
	invocationapplication "github.com/orz-i/Mender/backend/internal/invocation/application"
	invocationevents "github.com/orz-i/Mender/backend/internal/invocation/infrastructure/events"
	invocationexecutionguard "github.com/orz-i/Mender/backend/internal/invocation/infrastructure/executionguard"
	invocationidentity "github.com/orz-i/Mender/backend/internal/invocation/infrastructure/identity"
	invocationjsonschema "github.com/orz-i/Mender/backend/internal/invocation/infrastructure/jsonschema"
	invocationresultguard "github.com/orz-i/Mender/backend/internal/invocation/infrastructure/resultguard"
	invocationrouter "github.com/orz-i/Mender/backend/internal/invocation/infrastructure/router"
	mcpconfigstore "github.com/orz-i/Mender/backend/internal/mcp/infrastructure/configstore"
	policylocal "github.com/orz-i/Mender/backend/internal/policy/infrastructure/local"
	provideractivationapplication "github.com/orz-i/Mender/backend/internal/provideractivation/application"
	provideractivationidentity "github.com/orz-i/Mender/backend/internal/provideractivation/infrastructure/identity"
	providerplugin "github.com/orz-i/Mender/backend/internal/providerplugin"
	providerbuiltin "github.com/orz-i/Mender/backend/internal/providerplugin/builtin"
	providerconfigstore "github.com/orz-i/Mender/backend/internal/providerplugin/configstore"
	providerexternal "github.com/orz-i/Mender/backend/internal/providerplugin/external"
	installationlocalfs "github.com/orz-i/Mender/backend/internal/providerplugin/installation/infrastructure/localfs"
	installationsqlite "github.com/orz-i/Mender/backend/internal/providerplugin/installation/infrastructure/sqlite"
	tunnelapplication "github.com/orz-i/Mender/backend/internal/tunnel/application"
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
	RuntimeGovernance           *RuntimeGovernance
	InvokeTool                  invocationapplication.InvokeTool
	ListAudit                   auditapplication.ListRecords
	ListProviderActivationAudit auditapplication.ListProviderActivations

	audit               *auditsqlite.Store
	executors           *invocationrouter.Router
	mcpConfigs          *mcpconfigstore.Store
	providerPlugins     *providerplugin.Registry
	providerResolver    providerplugin.Resolver
	externalResolver    *providerexternal.Resolver
	providerConfigs     *providerconfigstore.Store
	pluginInstallations *installationsqlite.Store
	pluginPackages      *installationlocalfs.Store
	externalProcesses   *providerexternal.Supervisor
	providerInstances   *providerplugin.Manager
	providerActivation  provideractivationapplication.Admit
	ownerPrincipal      governancedomain.Principal

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

	configs, err := mcpconfigstore.Open(ctx, options.DatabasePath)
	if err != nil {
		_ = policies.Close()
		_ = profiles.Close()
		_ = credentials.Close()
		_ = auditStore.Close()
		_ = catalog.Close()
		return nil, fmt.Errorf("compose runtime: MCP configs: %w", err)
	}

	providerConfigs, err := providerconfigstore.Open(ctx, options.DatabasePath)
	if err != nil {
		_ = configs.Close()
		_ = policies.Close()
		_ = profiles.Close()
		_ = credentials.Close()
		_ = auditStore.Close()
		_ = catalog.Close()
		return nil, fmt.Errorf("compose runtime: provider configs: %w", err)
	}
	if err := importLegacyMCPProviderConfigs(ctx, configs, providerConfigs); err != nil {
		_ = providerConfigs.Close()
		_ = configs.Close()
		_ = policies.Close()
		_ = profiles.Close()
		_ = credentials.Close()
		_ = auditStore.Close()
		_ = catalog.Close()
		return nil, fmt.Errorf("compose runtime: migrate provider configs: %w", err)
	}

	runtimeGovernance, err := NewRuntimeGovernance(ctx, options.DatabasePath)
	if err != nil {
		_ = providerConfigs.Close()
		_ = configs.Close()
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

	executors := invocationrouter.New()
	providerPlugins := providerplugin.NewRegistry()
	if err := providerbuiltin.Register(
		providerPlugins,
		workspaceShellIsolationLauncher{governance: runtimeGovernance},
	); err != nil {
		_ = providerConfigs.Close()
		_ = configs.Close()
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
		_ = configs.Close()
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
		_ = configs.Close()
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
		_ = configs.Close()
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
		_ = configs.Close()
		_ = policies.Close()
		_ = profiles.Close()
		_ = credentials.Close()
		_ = auditStore.Close()
		_ = catalog.Close()
		return nil, fmt.Errorf("compose runtime: external plugin processes: %w", err)
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
		_ = configs.Close()
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
			profiles: profiles,
			tunnels:  tunnels,
			configs:  providerConfigs,
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
		_ = configs.Close()
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
		_ = configs.Close()
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
		_ = configs.Close()
		_ = policies.Close()
		_ = profiles.Close()
		_ = credentials.Close()
		_ = auditStore.Close()
		_ = catalog.Close()
		return nil, fmt.Errorf("compose runtime: skills: %w", err)
	}

	ids := invocationidentity.NewGenerator()
	approvalGate := policylocal.RejectApproval{}
	providerActivation := provideractivationapplication.NewAdmit(
		policies.Evaluate,
		approvalGate,
		runtimeGovernance.controller,
		auditStore,
		provideractivationidentity.NewGenerator(),
		systemClock{},
	)

	schemaValidator := invocationjsonschema.Validator{}
	bindingExecutionGuard := options.BindingExecutionGuard
	if bindingExecutionGuard == nil {
		bindingExecutionGuard = invocationexecutionguard.NewProfileBindingSerial()
	}
	observer := options.InvocationObserver
	if observer == nil {
		observer = invocationevents.NoopObserver{}
	}
	invokeTool := invocationapplication.NewInvokeTool(
		catalog.store,
		newInvocationProfileScopeResolver(profiles.GetByID),
		schemaValidator,
		newInvocationGovernanceFactExtractor(providerConfigs, providerPlugins),
		newInvocationPolicyEvaluator(policies.Evaluate),
		approvalGate,
		runtimeGovernance.controller,
		bindingExecutionGuard,
		invocationresultguard.New(schemaValidator),
		observer,
		executors,
		credentials.Resolver,
		auditStore,
		ids,
		systemClock{},
		options.DefaultTimeout,
	)

	tunnelsOwned = false
	runtimeGovernanceOwned = false
	return &Runtime{
		Catalog:                     catalog,
		Credentials:                 credentials,
		Profiles:                    profiles,
		Tunnels:                     tunnels,
		Policy:                      policies,
		Skills:                      skills,
		RuntimeGovernance:           runtimeGovernance,
		InvokeTool:                  invokeTool,
		ListAudit:                   auditapplication.NewListRecords(auditStore),
		ListProviderActivationAudit: auditapplication.NewListProviderActivations(auditStore),
		audit:                       auditStore,
		executors:                   executors,
		mcpConfigs:                  configs,
		providerPlugins:             providerPlugins,
		providerResolver:            providerResolver,
		externalResolver:            externalResolver,
		providerConfigs:             providerConfigs,
		pluginInstallations:         pluginInstallations,
		pluginPackages:              pluginPackages,
		externalProcesses:           externalProcesses,
		providerInstances:           providerInstances,
		providerActivation:          providerActivation,
		ownerPrincipal:              ownerPrincipal,
		mcpServers:                  make(map[string]*managedMCPServer),
	}, nil
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
	serverErr := r.closeManagedMCPServers()
	providerErr := r.providerInstances.CloseAll()
	processErr := r.externalProcesses.CloseAll()

	r.closersMu.Lock()
	closers := append([]io.Closer(nil), r.closers...)
	r.closers = nil
	r.closersMu.Unlock()

	errs := make([]error, 0, len(closers)+9)
	errs = append(errs, serverErr, providerErr, processErr)
	for i := len(closers) - 1; i >= 0; i-- {
		errs = append(errs, closers[i].Close())
	}
	errs = append(
		errs,
		r.Skills.Close(),
		r.RuntimeGovernance.Close(),
		r.pluginInstallations.Close(),
		r.providerConfigs.Close(),
		r.mcpConfigs.Close(),
		r.Policy.Close(),
		r.Profiles.Close(),
		r.Tunnels.Close(),
		r.Credentials.Close(),
		r.audit.Close(),
		r.Catalog.Close(),
	)
	return errors.Join(errs...)
}

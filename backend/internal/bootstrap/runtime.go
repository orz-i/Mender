package bootstrap

import (
	"context"
	"errors"
	"fmt"
	"io"
	"sync"
	"time"

	auditapplication "github.com/orz-i/Mender/backend/internal/audit/application"
	auditsqlite "github.com/orz-i/Mender/backend/internal/audit/infrastructure/sqlite"
	catalogdomain "github.com/orz-i/Mender/backend/internal/catalog/domain"
	credentialapplication "github.com/orz-i/Mender/backend/internal/credential/application"
	invocationapplication "github.com/orz-i/Mender/backend/internal/invocation/application"
	invocationevents "github.com/orz-i/Mender/backend/internal/invocation/infrastructure/events"
	invocationexecutionguard "github.com/orz-i/Mender/backend/internal/invocation/infrastructure/executionguard"
	invocationgovernance "github.com/orz-i/Mender/backend/internal/invocation/infrastructure/governance"
	invocationidentity "github.com/orz-i/Mender/backend/internal/invocation/infrastructure/identity"
	invocationjsonschema "github.com/orz-i/Mender/backend/internal/invocation/infrastructure/jsonschema"
	invocationresultguard "github.com/orz-i/Mender/backend/internal/invocation/infrastructure/resultguard"
	invocationrouter "github.com/orz-i/Mender/backend/internal/invocation/infrastructure/router"
	mcpconfigstore "github.com/orz-i/Mender/backend/internal/mcp/infrastructure/configstore"
	mcpplugin "github.com/orz-i/Mender/backend/internal/mcp/infrastructure/plugin"
	policylocal "github.com/orz-i/Mender/backend/internal/policy/infrastructure/local"
	providerplugin "github.com/orz-i/Mender/backend/internal/providerplugin"
	providerconfigstore "github.com/orz-i/Mender/backend/internal/providerplugin/configstore"
)

// RuntimeOptions configures the shared local execution runtime.
type RuntimeOptions struct {
	DatabasePath          string
	DefaultTimeout        time.Duration
	MaxResultBytes        int
	ExecutionGuard        invocationapplication.ExecutionGuard
	BindingExecutionGuard invocationapplication.BindingExecutionGuard
	InvocationObserver    invocationapplication.InvocationObserver
}

// Runtime composes Catalog, Profile persistence, Provider config persistence,
// and Mender's shared invocation pipeline.
type Runtime struct {
	Catalog     *Catalog
	Credentials *Credentials
	Profiles    *Profiles
	Policy      *Policies
	InvokeTool  invocationapplication.InvokeTool
	ListAudit   auditapplication.ListRecords

	audit             *auditsqlite.Store
	executors         *invocationrouter.Router
	mcpConfigs        *mcpconfigstore.Store
	providerPlugins   *providerplugin.Registry
	providerConfigs   *providerconfigstore.Store
	providerInstances *providerplugin.Manager

	closersMu sync.Mutex
	closers   []io.Closer

	mcpServersMu sync.Mutex
	mcpServers   map[string]*managedMCPServer
}

// NewRuntime constructs the protocol-independent local runtime.
func NewRuntime(ctx context.Context, options RuntimeOptions) (*Runtime, error) {
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

	profiles, err := NewProfiles(
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

	executors := invocationrouter.New()
	providerPlugins := providerplugin.NewRegistry()
	if err := providerPlugins.Register(mcpplugin.New()); err != nil {
		_ = providerConfigs.Close()
		_ = configs.Close()
		_ = policies.Close()
		_ = profiles.Close()
		_ = credentials.Close()
		_ = auditStore.Close()
		_ = catalog.Close()
		return nil, fmt.Errorf("compose runtime: provider plugins: %w", err)
	}
	credentials.Delete = credentialapplication.NewDeleteCredential(
		credentials.store,
		credentials.secrets,
		runtimeCredentialUsageChecker{
			profiles: profiles,
			configs:  providerConfigs,
			plugins:  providerPlugins,
		},
	)

	providerInstances, err := providerplugin.NewManager(
		providerPlugins,
		providerConfigs,
		executors,
		credentials.Resolver,
	)
	if err != nil {
		_ = providerConfigs.Close()
		_ = configs.Close()
		_ = policies.Close()
		_ = profiles.Close()
		_ = credentials.Close()
		_ = auditStore.Close()
		_ = catalog.Close()
		return nil, fmt.Errorf("compose runtime: provider instances: %w", err)
	}
	ids := invocationidentity.NewGenerator()

	schemaValidator := invocationjsonschema.Validator{}
	executionGuard := options.ExecutionGuard
	if executionGuard == nil {
		executionGuard = invocationexecutionguard.Unlimited{}
	}
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
		invocationgovernance.UnknownExtractor{},
		newInvocationPolicyEvaluator(policies.Evaluate),
		policylocal.RejectApproval{},
		executionGuard,
		bindingExecutionGuard,
		invocationresultguard.New(schemaValidator, options.MaxResultBytes),
		observer,
		executors,
		credentials.Resolver,
		auditStore,
		ids,
		systemClock{},
		options.DefaultTimeout,
	)

	return &Runtime{
		Catalog:           catalog,
		Credentials:       credentials,
		Profiles:          profiles,
		Policy:            policies,
		InvokeTool:        invokeTool,
		ListAudit:         auditapplication.NewListRecords(auditStore),
		audit:             auditStore,
		executors:         executors,
		mcpConfigs:        configs,
		providerPlugins:   providerPlugins,
		providerConfigs:   providerConfigs,
		providerInstances: providerInstances,
		mcpServers:        make(map[string]*managedMCPServer),
	}, nil
}

// RegisterExecutor binds a canonical Provider to its execution adapter.
func (r *Runtime) RegisterExecutor(
	providerID catalogdomain.ProviderID,
	executor invocationapplication.ToolExecutor,
) error {
	return r.executors.Register(providerID, executor)
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

	r.closersMu.Lock()
	closers := append([]io.Closer(nil), r.closers...)
	r.closers = nil
	r.closersMu.Unlock()

	errs := make([]error, 0, len(closers)+6)
	errs = append(errs, serverErr, providerErr)
	for i := len(closers) - 1; i >= 0; i-- {
		errs = append(errs, closers[i].Close())
	}
	errs = append(
		errs,
		r.providerConfigs.Close(),
		r.mcpConfigs.Close(),
		r.Policy.Close(),
		r.Profiles.Close(),
		r.Credentials.Close(),
		r.audit.Close(),
		r.Catalog.Close(),
	)
	return errors.Join(errs...)
}

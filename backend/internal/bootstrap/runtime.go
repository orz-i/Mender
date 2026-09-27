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
	credentialunavailable "github.com/orz-i/Mender/backend/internal/credential/infrastructure/unavailable"
	invocationapplication "github.com/orz-i/Mender/backend/internal/invocation/application"
	invocationgovernance "github.com/orz-i/Mender/backend/internal/invocation/infrastructure/governance"
	invocationidentity "github.com/orz-i/Mender/backend/internal/invocation/infrastructure/identity"
	invocationjsonschema "github.com/orz-i/Mender/backend/internal/invocation/infrastructure/jsonschema"
	invocationrouter "github.com/orz-i/Mender/backend/internal/invocation/infrastructure/router"
	mcpconfigstore "github.com/orz-i/Mender/backend/internal/mcp/infrastructure/configstore"
	policylocal "github.com/orz-i/Mender/backend/internal/policy/infrastructure/local"
)

// RuntimeOptions configures the shared local execution runtime.
type RuntimeOptions struct {
	DatabasePath   string
	DefaultTimeout time.Duration
}

// Runtime composes Catalog, Profile persistence, Provider config persistence,
// and Mender's shared invocation pipeline.
type Runtime struct {
	Catalog    *Catalog
	Profiles   *Profiles
	InvokeTool invocationapplication.InvokeTool
	ListAudit  auditapplication.ListRecords

	audit      *auditsqlite.Store
	executors  *invocationrouter.Router
	mcpConfigs *mcpconfigstore.Store

	closersMu sync.Mutex
	closers   []io.Closer

	providerActivationMu sync.Mutex
	mcpServersMu         sync.Mutex
	mcpServers           map[string]*managedMCPServer
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

	profiles, err := NewProfiles(ctx, options.DatabasePath, catalog.store)
	if err != nil {
		_ = auditStore.Close()
		_ = catalog.Close()
		return nil, fmt.Errorf("compose runtime: profiles: %w", err)
	}

	configs, err := mcpconfigstore.Open(ctx, options.DatabasePath)
	if err != nil {
		_ = profiles.Close()
		_ = auditStore.Close()
		_ = catalog.Close()
		return nil, fmt.Errorf("compose runtime: MCP configs: %w", err)
	}

	executors := invocationrouter.New()
	ids := invocationidentity.NewGenerator()

	invokeTool := invocationapplication.NewInvokeTool(
		catalog.store,
		newInvocationProfileScopeAuthorizer(profiles.GetByID),
		invocationjsonschema.Validator{},
		invocationgovernance.UnknownExtractor{},
		policylocal.AllowAll{},
		policylocal.RejectApproval{},
		executors,
		credentialunavailable.Resolver{},
		auditStore,
		ids,
		systemClock{},
		options.DefaultTimeout,
	)

	return &Runtime{
		Catalog:    catalog,
		Profiles:   profiles,
		InvokeTool: invokeTool,
		ListAudit:  auditapplication.NewListRecords(auditStore),
		audit:      auditStore,
		executors:  executors,
		mcpConfigs: configs,
		mcpServers: make(map[string]*managedMCPServer),
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

	r.closersMu.Lock()
	closers := append([]io.Closer(nil), r.closers...)
	r.closers = nil
	r.closersMu.Unlock()

	errs := make([]error, 0, len(closers)+5)
	errs = append(errs, serverErr)
	for i := len(closers) - 1; i >= 0; i-- {
		errs = append(errs, closers[i].Close())
	}
	errs = append(
		errs,
		r.mcpConfigs.Close(),
		r.Profiles.Close(),
		r.audit.Close(),
		r.Catalog.Close(),
	)
	return errors.Join(errs...)
}

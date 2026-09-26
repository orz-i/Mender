package bootstrap

import (
	"context"
	"errors"
	"fmt"
	"time"

	auditsqlite "github.com/orz-i/Mender/backend/internal/audit/infrastructure/sqlite"
	catalogdomain "github.com/orz-i/Mender/backend/internal/catalog/domain"
	credentialunavailable "github.com/orz-i/Mender/backend/internal/credential/infrastructure/unavailable"
	invocationapplication "github.com/orz-i/Mender/backend/internal/invocation/application"
	invocationidentity "github.com/orz-i/Mender/backend/internal/invocation/infrastructure/identity"
	invocationrouter "github.com/orz-i/Mender/backend/internal/invocation/infrastructure/router"
	policylocal "github.com/orz-i/Mender/backend/internal/policy/infrastructure/local"
)

// RuntimeOptions configures the shared local execution runtime.
type RuntimeOptions struct {
	DatabasePath   string
	DefaultTimeout time.Duration
}

// Runtime composes Catalog with Mender's shared invocation pipeline.
type Runtime struct {
	Catalog    *Catalog
	InvokeTool invocationapplication.InvokeTool

	audit     *auditsqlite.Store
	executors *invocationrouter.Router
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

	executors := invocationrouter.New()
	ids := invocationidentity.NewGenerator()

	invokeTool := invocationapplication.NewInvokeTool(
		catalog.store,
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
		InvokeTool: invokeTool,
		audit:      auditStore,
		executors:  executors,
	}, nil
}

// RegisterExecutor binds a canonical Provider to its execution adapter.
func (r *Runtime) RegisterExecutor(
	providerID catalogdomain.ProviderID,
	executor invocationapplication.ToolExecutor,
) error {
	return r.executors.Register(providerID, executor)
}

// Close releases all runtime-owned infrastructure.
func (r *Runtime) Close() error {
	return errors.Join(
		r.audit.Close(),
		r.Catalog.Close(),
	)
}

// Package application coordinates configured Provider management through
// consumer-owned ports. Canonical identity remains Catalog-owned and plugin
// configuration remains in the existing providerplugin model.
package application

import (
	"context"
	"errors"

	catalogapplication "github.com/orz-i/Mender/backend/internal/catalog/application"
	catalogdomain "github.com/orz-i/Mender/backend/internal/catalog/domain"
	invocationapplication "github.com/orz-i/Mender/backend/internal/invocation/application"
	policydomain "github.com/orz-i/Mender/backend/internal/policy/domain"
	profiledomain "github.com/orz-i/Mender/backend/internal/profile/domain"
	provideractivationapplication "github.com/orz-i/Mender/backend/internal/provideractivation/application"
	"github.com/orz-i/Mender/backend/internal/providerplugin"
)

var ErrProviderInUse = errors.New("provider is referenced by a Profile or Policy Rule")

// Catalog supplies canonical Provider operations and explicit Tool snapshot
// compensation without exposing another context's repository.
type Catalog interface {
	Register(context.Context, catalogapplication.RegisterProviderRequest) (catalogdomain.Provider, error)
	Update(context.Context, catalogapplication.UpdateProviderRequest) (catalogdomain.Provider, error)
	Get(context.Context, catalogdomain.ProviderID) (catalogdomain.Provider, error)
	Remove(context.Context, catalogdomain.ProviderID) error
	DescribeTool(context.Context, catalogdomain.ToolID) (catalogdomain.Tool, error)
	Sync(context.Context, catalogdomain.ProviderID, catalogapplication.ToolSource) (catalogapplication.SyncProviderToolsResult, error)
	Snapshot(context.Context, catalogdomain.ProviderID) ([]catalogdomain.Tool, error)
	Restore(context.Context, catalogdomain.ProviderID, []catalogdomain.Tool) error
}

type Configurations interface {
	GetProviderConfig(context.Context, catalogdomain.ProviderID) (providerplugin.Configuration, error)
	Save(context.Context, providerplugin.Configuration) error
}

type BuiltinPlugins interface {
	Lookup(providerplugin.ID) (providerplugin.Plugin, error)
}

type InstallationResolver interface {
	ResolveInstallation(context.Context, providerplugin.InstallationID) (providerplugin.Resolution, error)
}

type Instances interface {
	Ensure(context.Context, catalogdomain.ProviderID) (providerplugin.Instance, error)
	Reload(context.Context, catalogdomain.ProviderID) (providerplugin.Instance, error)
	CloseProvider(catalogdomain.ProviderID) error
}

type Activation interface {
	Execute(context.Context, provideractivationapplication.Request, provideractivationapplication.Operation) error
}

type ProfileQuery interface {
	Execute(context.Context) ([]profiledomain.Profile, error)
}

type PolicyQuery interface {
	Execute(context.Context) (policydomain.PolicySet, error)
}

// OwnershipQuery is implemented by each resource context's narrow application
// query. Provider management never reaches into Agent or Sandbox repositories.
type OwnershipQuery interface {
	Execute(context.Context, catalogdomain.ProviderID) (bool, error)
}

// Retirement commits the existing cross-context reference contract at one
// SQLite writer gate; preflight queries do not replace that commit-point guard.
type Retirement interface {
	Delete(context.Context, catalogdomain.ProviderID) error
}

type Dependencies struct {
	Catalog          Catalog
	Configurations   Configurations
	Builtins         BuiltinPlugins
	Resolver         providerplugin.Resolver
	Installations    InstallationResolver
	Instances        Instances
	Credentials      invocationapplication.CredentialResolver
	Activation       Activation
	Profiles         ProfileQuery
	Policy           PolicyQuery
	AgentOwnership   OwnershipQuery
	SandboxOwnership OwnershipQuery
	Retirement       Retirement
	Clock            catalogapplication.Clock
	IsManagedFacade  func(providerplugin.ID) bool
}

// Service is stateless; the composition root retains connection, instance and
// process ownership while these use cases own ordering and compensation.
type Service struct{ deps Dependencies }

func NewService(deps Dependencies) Service { return Service{deps: deps} }

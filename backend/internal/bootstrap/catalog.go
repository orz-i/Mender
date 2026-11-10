package bootstrap

import (
	"context"
	"database/sql"
	"fmt"
	"time"

	"github.com/orz-i/Mender/backend/internal/catalog/application"
	catalogdomain "github.com/orz-i/Mender/backend/internal/catalog/domain"
	"github.com/orz-i/Mender/backend/internal/catalog/infrastructure/identity"
	sourcecatalog "github.com/orz-i/Mender/backend/internal/catalog/infrastructure/sourcecatalog"
	v3schema "github.com/orz-i/Mender/backend/internal/schema/v3"
)

// CatalogOptions configures the Catalog composition root.
type CatalogOptions struct {
	DatabasePath string
}

// The same Catalog application use cases use one canonical persistence port.
// Provider metadata and Tool sources are distinct identities; no per-resource
// Provider Facade, duplicate search use case, or second Invocation is created.
type catalogStorage interface {
	application.ProviderCreator
	application.ProviderUpdater
	application.ProviderGetter
	application.ProviderLister
	application.ProviderNamespaceFinder
	application.ProviderRemover
	application.ProviderToolRepository
	application.ProviderToolCountLister
	application.ToolGetter
	application.ToolBrowser
	application.ToolPager
	application.ToolSearch
	Close() error
}

// Catalog contains the currently composed Catalog use cases.
// Tool execution and protocol adapters do not belong in this Catalog root.
type Catalog struct {
	RegisterProvider        application.RegisterProvider
	UpdateProvider          application.UpdateProvider
	GetProvider             application.GetProvider
	ListProviders           application.ListProviders
	FindProviderByNamespace application.FindProviderByNamespace
	RemoveProvider          application.RemoveProvider
	DescribeTool            application.DescribeTool
	BrowseTools             application.BrowseTools
	PageTools               application.PageTools
	SearchTools             application.SearchTools
	ListProviderToolCounts  application.ListProviderToolCounts
	ProviderToolSnapshots   application.ProviderToolSnapshots

	store catalogStorage
	ids   *identity.Generator
	clock systemClock
}

// NewTypedCatalog composes canonical Catalog services over the only supported
// v3 schema. It requires an existing database and verifies the exact current
// Agent/Provider identity; no Sandbox/Agent can grant itself a Tool source. This
// constructor is used by the official Runtime composition.
func NewTypedCatalog(ctx context.Context, options CatalogOptions) (*Catalog, error) {
	db, err := v3schema.OpenContextStore(ctx, options.DatabasePath)
	if err != nil {
		return nil, fmt.Errorf("compose typed catalog: %w", err)
	}
	repository, err := sourcecatalog.NewRepository(ctx, db, verifiedCatalogSourceOwner{db: db})
	if err != nil {
		_ = db.Close()
		return nil, fmt.Errorf("compose typed catalog: %w", err)
	}
	return composeCatalog(&typedCatalogStorage{Repository: repository, db: db}), nil
}

// verifiedCatalogSourceOwner checks the actual current resource inventory,
// not an Agent/Sandbox label or Tool-supplied annotation. The precise Tool
// contract remains pinned by SourceCatalog and by the migrated SQL guards.
// This check is read-only: Provider activation, credentials and ACP process
// lifecycle are governed later by their own application services.
type verifiedCatalogSourceOwner struct{ db *sql.DB }

func (v verifiedCatalogSourceOwner) VerifySourceOwner(ctx context.Context, source catalogdomain.SourceRef) error {
	if v.db == nil || source.Validate() != nil {
		return sourcecatalog.ErrUnverifiedSource
	}
	var exists int
	var err error
	switch source.Kind() {
	case catalogdomain.SourceProvider:
		err = v.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM providers p
 JOIN provider_configs c ON c.provider_id=p.id WHERE p.id=?`, source.ID()).Scan(&exists)
	case catalogdomain.SourceAgent:
		err = v.db.QueryRowContext(ctx, "SELECT COUNT(*) FROM agents WHERE agent_id=?", source.ID()).Scan(&exists)
	case catalogdomain.SourceNative:
		// Native identity is fixed to workspace/jobs by SourceRef validation;
		// catalog UpsertSourceSnapshot validates every immutable Tool schema.
		return nil
	default:
		return sourcecatalog.ErrUnverifiedSource
	}
	if err != nil {
		return fmt.Errorf("verify canonical Tool source: %w", err)
	}
	if exists != 1 {
		return sourcecatalog.ErrUnverifiedSource
	}
	return nil
}

// The typed Catalog owns the one SQLite handle it opened, not a separate
// Provider connection. A caller who supplied a read-only verifier is never
// delegated DB lifecycle or actual Tool invocation permission.
type typedCatalogStorage struct {
	*sourcecatalog.Repository
	db *sql.DB
}

func (s *typedCatalogStorage) Close() error { return s.db.Close() }

func composeCatalog(store catalogStorage) *Catalog {
	ids := identity.NewGenerator()
	clock := systemClock{}
	return &Catalog{
		RegisterProvider:        application.NewRegisterProvider(store, ids, clock),
		UpdateProvider:          application.NewUpdateProvider(store, store, clock),
		GetProvider:             application.NewGetProvider(store),
		ListProviders:           application.NewListProviders(store),
		FindProviderByNamespace: application.NewFindProviderByNamespace(store),
		RemoveProvider:          application.NewRemoveProvider(store),
		DescribeTool:            application.NewDescribeTool(store),
		BrowseTools:             application.NewBrowseTools(store),
		PageTools:               application.NewPageTools(store),
		SearchTools:             application.NewSearchTools(store),
		ListProviderToolCounts:  application.NewListProviderToolCounts(store),
		ProviderToolSnapshots:   application.NewProviderToolSnapshots(store),
		store:                   store,
		ids:                     ids,
		clock:                   clock,
	}
}

// SyncProviderTools creates the synchronization use case for one configured
// external Provider. The exact Tool SourceRef is retained across refreshes.
func (c *Catalog) SyncProviderTools(source application.ToolSource) application.SyncProviderTools {
	return application.NewSyncProviderTools(c.store, c.store, source, c.ids, c.clock)
}

func (c *Catalog) Close() error { return c.store.Close() }

type systemClock struct{}

func (systemClock) Now() time.Time { return time.Now().UTC() }

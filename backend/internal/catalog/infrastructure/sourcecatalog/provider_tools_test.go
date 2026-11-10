package sourcecatalog

import (
	"context"
	"errors"
	"testing"
	"time"

	app "github.com/orz-i/Mender/backend/internal/catalog/application"
	catalog "github.com/orz-i/Mender/backend/internal/catalog/domain"
)

type providerContractIDs struct{}

func (providerContractIDs) NewProviderID() (catalog.ProviderID, error) {
	return catalog.ParseProviderID("prov_99999999999999999999999999999999")
}
func (providerContractIDs) NewToolID() (catalog.ToolID, error) {
	return catalog.ParseToolID("tool_99999999999999999999999999999999")
}

type providerContractClock struct{}

func (providerContractClock) Now() time.Time { return time.Date(2026, 11, 8, 12, 0, 0, 0, time.UTC) }

type providerContractSource struct{ discovered []app.DiscoveredTool }

func (s providerContractSource) DiscoverTools(context.Context, catalog.Provider) ([]app.DiscoveredTool, error) {
	return s.discovered, nil
}

func TestTypedCatalogReusesExternalProviderApplicationWithoutFabricatingNativeProvider(t *testing.T) {
	ctx := context.Background()
	db := testDatabase(t)
	providerSource, _ := catalog.ProviderSource("prov_99999999999999999999999999999999")
	nativeSource, _ := catalog.NativeSource("workspace")
	owner := approvedOwners{map[catalog.SourceRef]bool{providerSource: true, nativeSource: true}}
	repo, err := NewRepository(ctx, db, owner)
	if err != nil {
		t.Fatal(err)
	}
	created, err := app.NewRegisterProvider(repo, providerContractIDs{}, providerContractClock{}).Execute(ctx, app.RegisterProviderRequest{Namespace: "external-api", DisplayName: "External API", Description: "Verified provider"})
	if err != nil || created.ID().String() != providerSource.ID() {
		t.Fatalf("Provider registration changed: %+v %v", created, err)
	}
	if _, err := app.NewRegisterProvider(repo, providerContractIDs{}, providerContractClock{}).Execute(ctx, app.RegisterProviderRequest{Namespace: "external-api", DisplayName: "Duplicate"}); !errors.Is(err, app.ErrNamespaceConflict) {
		t.Fatalf("Provider Namespace conflict not preserved: %v", err)
	}
	fetched, err := app.NewGetProvider(repo).Execute(ctx, created.ID())
	if err != nil || fetched.ID() != created.ID() || fetched.Namespace() != created.Namespace() {
		t.Fatalf("Provider Get lost identity %+v %v", fetched, err)
	}
	found, err := app.NewFindProviderByNamespace(repo).Execute(ctx, created.Namespace())
	if err != nil || found.ID() != created.ID() {
		t.Fatalf("Provider Namespace lookup failed: %+v %v", found, err)
	}
	providers, err := app.NewListProviders(repo).Execute(ctx)
	if err != nil || len(providers) != 1 || providers[0].ID() != created.ID() {
		t.Fatalf("Provider list failed: %+v %v", providers, err)
	}

	nativeTools := nativeWorkspaceTools(t, db, nativeSource)
	schema, err := catalog.NewSchema([]byte(`{"type":"object"}`))
	if err != nil {
		t.Fatal(err)
	}
	discover := providerContractSource{[]app.DiscoveredTool{{SourceKey: "api.search", Name: "search", Title: "Search remote API", Description: "External search", InputSchema: schema.Bytes(), Tags: []string{"external"}}}}
	syncer := app.NewSyncProviderTools(repo, repo, discover, providerContractIDs{}, providerContractClock{})
	result, err := syncer.Execute(ctx, created.ID())
	if err != nil || result.Created != 1 || result.Total != 1 {
		t.Fatalf("Provider source sync failed: %+v %v", result, err)
	}
	external, err := repo.ListProviderTools(ctx, created.ID())
	if err != nil || len(external) != 1 || external[0].SourceRef() != providerSource || external[0].SourceKey() != "api.search" {
		t.Fatalf("typed provider Tool source incorrect: %+v %v", external, err)
	}
	counts, err := app.NewListProviderToolCounts(repo).Execute(ctx)
	if err != nil || len(counts) != 1 || counts[created.ID()] != 1 {
		t.Fatalf("native Tool leaked into Provider counts: %+v %v", counts, err)
	}
	current, err := repo.GetTool(ctx, external[0].ID())
	if err != nil || current.SourceRef() != providerSource {
		t.Fatalf("typed Catalog lookup lost Provider: %+v %v", current, err)
	}
	refreshed, err := current.RefreshMetadata(catalog.ToolSpec{SourceKey: "api.search", Name: "search", Title: "Refreshed API", Description: "External search", InputSchema: schema, Tags: []string{"external"}}, providerContractClock{}.Now().Add(time.Second))
	if err != nil {
		t.Fatal(err)
	}
	if err := repo.ReplaceProviderTools(ctx, created.ID(), []catalog.Tool{refreshed}); err != nil {
		t.Fatal(err)
	}
	after, err := repo.ListProviderTools(ctx, created.ID())
	if err != nil || len(after) != 1 || after[0].ID() != current.ID() || after[0].Title() != "Refreshed API" {
		t.Fatalf("metadata refresh lost ToolID: %+v %v", after, err)
	}
	if err := repo.ReplaceProviderTools(ctx, created.ID(), nil); !errors.Is(err, ErrRetirementRequiresReview) {
		t.Fatalf("unreviewed ToolID retirement accepted: %v", err)
	}
	if err := repo.ReplaceProviderTools(ctx, created.ID(), nativeTools); !errors.Is(err, ErrSourceIdentityConflict) {
		t.Fatalf("native Tool adopted by Provider: %v", err)
	}
	if err := app.NewRemoveProvider(repo).Execute(ctx, created.ID()); err == nil {
		t.Fatal("Provider with live ToolIDs was silently deleted")
	}
	checkedNative, err := repo.ListSourceTools(ctx, nativeSource)
	if err != nil || len(checkedNative) != len(nativeTools) {
		t.Fatalf("Provider CRUD changed Native identity: %v %d", err, len(checkedNative))
	}
	assertDBIntegrity(t, db)
}

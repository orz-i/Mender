package sqlite

import (
	"bytes"
	"context"
	"errors"
	"path/filepath"
	"testing"
	"time"

	"github.com/orz-i/Mender/backend/internal/catalog/application"
	"github.com/orz-i/Mender/backend/internal/catalog/domain"
)

func TestProviderRoundTripAndNamespaceConflict(t *testing.T) {
	t.Parallel()

	store := openTestStore(t)
	now := testTime()
	namespace, _ := domain.NewNamespace("github")
	provider, err := domain.NewProvider(
		mustProviderID(t, "prov_00000000000000000000000000000001"),
		namespace,
		"GitHub",
		"Repository tools",
		now,
	)
	if err != nil {
		t.Fatal(err)
	}

	if err := store.CreateProvider(context.Background(), provider); err != nil {
		t.Fatalf("CreateProvider() error = %v", err)
	}

	loaded, err := store.GetProvider(context.Background(), provider.ID())
	if err != nil {
		t.Fatalf("GetProvider() error = %v", err)
	}
	if loaded.ID() != provider.ID() ||
		loaded.Namespace() != provider.Namespace() ||
		loaded.DisplayName() != provider.DisplayName() ||
		loaded.Description() != provider.Description() ||
		!loaded.CreatedAt().Equal(provider.CreatedAt()) {
		t.Fatalf("loaded Provider = %#v, want %#v", loaded, provider)
	}

	conflict, _ := domain.NewProvider(
		mustProviderID(t, "prov_00000000000000000000000000000002"),
		namespace,
		"Other GitHub",
		"",
		now,
	)
	if err := store.CreateProvider(context.Background(), conflict); !errors.Is(err, application.ErrNamespaceConflict) {
		t.Fatalf("CreateProvider(conflict) error = %v", err)
	}
}

func TestProviderAdministrationQueriesAndRemoval(t *testing.T) {
	t.Parallel()

	store := openTestStore(t)
	first := createProvider(t, store, "alpha", "prov_00000000000000000000000000000001")
	second := createProvider(t, store, "beta", "prov_00000000000000000000000000000002")

	providers, err := store.ListProviders(context.Background())
	if err != nil {
		t.Fatalf("ListProviders() error = %v", err)
	}
	if len(providers) != 2 || providers[0].ID() != first.ID() || providers[1].ID() != second.ID() {
		t.Fatalf("ListProviders() = %+v", providers)
	}

	found, err := store.FindProviderByNamespace(context.Background(), second.Namespace())
	if err != nil {
		t.Fatalf("FindProviderByNamespace() error = %v", err)
	}
	if found.ID() != second.ID() {
		t.Fatalf("found ID = %q, want %q", found.ID(), second.ID())
	}

	input, _ := domain.NewSchema([]byte(`{"type":"object"}`))
	tool, _ := domain.NewTool(
		mustToolID(t, "tool_00000000000000000000000000000001"),
		first.ID(),
		domain.ToolSpec{
			SourceKey:   "owned",
			Name:        "owned",
			InputSchema: input,
		},
		testTime(),
	)
	if err := store.ReplaceProviderTools(context.Background(), first.ID(), []domain.Tool{tool}); err != nil {
		t.Fatal(err)
	}

	if err := store.RemoveProvider(context.Background(), first.ID()); err != nil {
		t.Fatalf("RemoveProvider() error = %v", err)
	}
	if _, err := store.GetProvider(context.Background(), first.ID()); !errors.Is(err, application.ErrProviderNotFound) {
		t.Fatalf("GetProvider(removed) error = %v", err)
	}
	if _, err := store.GetTool(context.Background(), tool.ID()); !errors.Is(err, application.ErrToolNotFound) {
		t.Fatalf("GetTool(cascade removed) error = %v", err)
	}
}

func TestToolSchemaRoundTripDescribeAndSearch(t *testing.T) {
	t.Parallel()

	store := openTestStore(t)
	provider := createProvider(t, store, "github", "prov_00000000000000000000000000000001")

	inputRaw := []byte(`{"$schema":"https://json-schema.org/draft/2020-12/schema","type":"object","properties":{"title":{"type":"string"}}}`)
	outputRaw := []byte("false")
	input, _ := domain.NewSchema(inputRaw)
	output, _ := domain.NewSchema(outputRaw)

	issueTool, err := domain.NewTool(
		mustToolID(t, "tool_00000000000000000000000000000001"),
		provider.ID(),
		domain.ToolSpec{
			SourceKey:    "issues.create",
			Name:         "create_issue",
			Title:        "Create issue",
			Description:  "Create a repository issue",
			InputSchema:  input,
			OutputSchema: &output,
			Tags:         []string{"github", "write", "issue"},
		},
		testTime(),
	)
	if err != nil {
		t.Fatal(err)
	}

	branchTool, err := domain.NewTool(
		mustToolID(t, "tool_00000000000000000000000000000002"),
		provider.ID(),
		domain.ToolSpec{
			SourceKey:   "branches.list",
			Name:        "list_branches",
			Title:       "List branches",
			Description: "List repository branches",
			InputSchema: input,
			Tags:        []string{"github", "read"},
		},
		testTime(),
	)
	if err != nil {
		t.Fatal(err)
	}

	if err := store.ReplaceProviderTools(
		context.Background(),
		provider.ID(),
		[]domain.Tool{issueTool, branchTool},
	); err != nil {
		t.Fatalf("ReplaceProviderTools() error = %v", err)
	}

	describe := application.NewDescribeTool(store)
	loaded, err := describe.Execute(context.Background(), issueTool.ID())
	if err != nil {
		t.Fatalf("DescribeTool.Execute() error = %v", err)
	}
	if !bytes.Equal(loaded.InputSchema().Bytes(), inputRaw) {
		t.Fatalf("input schema = %s, want %s", loaded.InputSchema().Bytes(), inputRaw)
	}
	if loaded.OutputSchema() == nil || !bytes.Equal(loaded.OutputSchema().Bytes(), outputRaw) {
		t.Fatalf("output schema did not round-trip: %v", loaded.OutputSchema())
	}

	search := application.NewSearchTools(store)
	results, err := search.Execute(context.Background(), application.SearchToolsRequest{
		Query: "issue",
		Limit: 10,
	})
	if err != nil {
		t.Fatalf("SearchTools.Execute() error = %v", err)
	}
	if len(results) != 1 || results[0].ToolID != issueTool.ID() {
		t.Fatalf("issue search results = %+v", results)
	}

	prefixResults, err := search.Execute(context.Background(), application.SearchToolsRequest{
		Query: "bran",
		Limit: 10,
	})
	if err != nil {
		t.Fatalf("SearchTools prefix error = %v", err)
	}
	if len(prefixResults) != 1 || prefixResults[0].ToolID != branchTool.ID() {
		t.Fatalf("prefix search results = %+v", prefixResults)
	}

	restricted, err := search.Execute(context.Background(), application.SearchToolsRequest{
		Query:          "repository",
		Limit:          10,
		AllowedToolIDs: []domain.ToolID{branchTool.ID()},
	})
	if err != nil {
		t.Fatalf("restricted search error = %v", err)
	}
	if len(restricted) != 1 || restricted[0].ToolID != branchTool.ID() {
		t.Fatalf("restricted search results = %+v", restricted)
	}

	empty, err := search.Execute(context.Background(), application.SearchToolsRequest{
		Query:          "repository",
		Limit:          10,
		AllowedToolIDs: []domain.ToolID{},
	})
	if err != nil {
		t.Fatalf("empty scoped search error = %v", err)
	}
	if len(empty) != 0 {
		t.Fatalf("empty scoped search results = %+v", empty)
	}
}

func TestBrowseToolsAndProviderCountsUseCompactCatalogRows(t *testing.T) {
	t.Parallel()

	store := openTestStore(t)
	first := createProvider(t, store, "alpha", "prov_00000000000000000000000000000001")
	second := createProvider(t, store, "beta", "prov_00000000000000000000000000000002")
	input, _ := domain.NewSchema([]byte(`{"type":"object","properties":{"large":{"type":"string"}}}`))

	firstTool, _ := domain.NewTool(
		mustToolID(t, "tool_00000000000000000000000000000001"),
		first.ID(),
		domain.ToolSpec{
			SourceKey:   "alpha.search",
			Name:        "search",
			Title:       "Search",
			Description: "Search alpha",
			InputSchema: input,
		},
		testTime(),
	)
	secondTool, _ := domain.NewTool(
		mustToolID(t, "tool_00000000000000000000000000000002"),
		second.ID(),
		domain.ToolSpec{
			SourceKey:   "beta.extract",
			Name:        "extract",
			Title:       "Extract",
			Description: "Extract beta",
			InputSchema: input,
		},
		testTime(),
	)
	if err := store.ReplaceProviderTools(context.Background(), first.ID(), []domain.Tool{firstTool}); err != nil {
		t.Fatal(err)
	}
	if err := store.ReplaceProviderTools(context.Background(), second.ID(), []domain.Tool{secondTool}); err != nil {
		t.Fatal(err)
	}

	browse := application.NewBrowseTools(store)
	items, err := browse.Execute(context.Background(), application.BrowseToolsRequest{Limit: 10})
	if err != nil {
		t.Fatalf("BrowseTools.Execute() error = %v", err)
	}
	if len(items) != 2 {
		t.Fatalf("browse items = %+v", items)
	}

	scoped, err := browse.Execute(context.Background(), application.BrowseToolsRequest{
		Limit:      10,
		ProviderID: ptrProviderID(first.ID()),
	})
	if err != nil {
		t.Fatalf("scoped BrowseTools.Execute() error = %v", err)
	}
	if len(scoped) != 1 || scoped[0].ToolID != firstTool.ID() {
		t.Fatalf("scoped browse items = %+v", scoped)
	}

	counts := application.NewListProviderToolCounts(store)
	byProvider, err := counts.Execute(context.Background())
	if err != nil {
		t.Fatalf("ListProviderToolCounts.Execute() error = %v", err)
	}
	if byProvider[first.ID()] != 1 || byProvider[second.ID()] != 1 {
		t.Fatalf("provider counts = %+v", byProvider)
	}
}

func TestReplaceProviderToolsRemovesOldSnapshotAndSearchRows(t *testing.T) {
	t.Parallel()

	store := openTestStore(t)
	provider := createProvider(t, store, "github", "prov_00000000000000000000000000000001")
	input, _ := domain.NewSchema([]byte(`{"type":"object"}`))

	oldTool, _ := domain.NewTool(
		mustToolID(t, "tool_00000000000000000000000000000001"),
		provider.ID(),
		domain.ToolSpec{
			SourceKey:   "old",
			Name:        "old_tool",
			Description: "obsolete marker",
			InputSchema: input,
		},
		testTime(),
	)
	newTool, _ := domain.NewTool(
		mustToolID(t, "tool_00000000000000000000000000000002"),
		provider.ID(),
		domain.ToolSpec{
			SourceKey:   "new",
			Name:        "new_tool",
			Description: "current marker",
			InputSchema: input,
		},
		testTime(),
	)

	if err := store.ReplaceProviderTools(context.Background(), provider.ID(), []domain.Tool{oldTool}); err != nil {
		t.Fatal(err)
	}
	if err := store.ReplaceProviderTools(context.Background(), provider.ID(), []domain.Tool{newTool}); err != nil {
		t.Fatal(err)
	}

	if _, err := store.GetTool(context.Background(), oldTool.ID()); !errors.Is(err, application.ErrToolNotFound) {
		t.Fatalf("GetTool(old) error = %v, want ErrToolNotFound", err)
	}

	search := application.NewSearchTools(store)
	results, err := search.Execute(context.Background(), application.SearchToolsRequest{
		Query: "obsolete",
		Limit: 10,
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(results) != 0 {
		t.Fatalf("stale search results = %+v", results)
	}
}

func openTestStore(t *testing.T) *Store {
	t.Helper()

	path := filepath.Join(t.TempDir(), "catalog.db")
	store, err := Open(context.Background(), path)
	if err != nil {
		t.Fatalf("Open() error = %v", err)
	}
	t.Cleanup(func() {
		if err := store.Close(); err != nil {
			t.Errorf("Close() error = %v", err)
		}
	})
	return store
}

func createProvider(t *testing.T, store *Store, namespaceValue, rawID string) domain.Provider {
	t.Helper()

	namespace, err := domain.NewNamespace(namespaceValue)
	if err != nil {
		t.Fatal(err)
	}
	provider, err := domain.NewProvider(
		mustProviderID(t, rawID),
		namespace,
		namespaceValue,
		"",
		testTime(),
	)
	if err != nil {
		t.Fatal(err)
	}
	if err := store.CreateProvider(context.Background(), provider); err != nil {
		t.Fatal(err)
	}
	return provider
}

func ptrProviderID(value domain.ProviderID) *domain.ProviderID {
	return &value
}

func mustProviderID(t *testing.T, raw string) domain.ProviderID {
	t.Helper()
	id, err := domain.ParseProviderID(raw)
	if err != nil {
		t.Fatal(err)
	}
	return id
}

func mustToolID(t *testing.T, raw string) domain.ToolID {
	t.Helper()
	id, err := domain.ParseToolID(raw)
	if err != nil {
		t.Fatal(err)
	}
	return id
}

func testTime() time.Time {
	return time.Date(2026, 9, 25, 12, 0, 0, 123456789, time.UTC)
}

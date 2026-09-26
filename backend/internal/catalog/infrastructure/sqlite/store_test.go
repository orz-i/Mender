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

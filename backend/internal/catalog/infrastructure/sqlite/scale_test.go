package sqlite

import (
	"context"
	"fmt"
	"path/filepath"
	"testing"

	"github.com/orz-i/Mender/backend/internal/catalog/application"
	"github.com/orz-i/Mender/backend/internal/catalog/domain"
)

const scaleToolCount = 10_000

func TestCatalogSearchAtTenThousandTools(t *testing.T) {
	store, ids := seedScaleCatalog(t, scaleToolCount)
	search := application.NewSearchTools(store)

	results, err := search.Execute(context.Background(), application.SearchToolsRequest{
		Query: "catalog common",
		Limit: 25,
	})
	if err != nil {
		t.Fatalf("SearchTools.Execute() error = %v", err)
	}
	if len(results) != 25 {
		t.Fatalf("result count = %d, want 25", len(results))
	}

	needle, err := search.Execute(context.Background(), application.SearchToolsRequest{
		Query: "catalogneedle",
		Limit: 10,
	})
	if err != nil {
		t.Fatalf("needle search error = %v", err)
	}
	if len(needle) != 1 || needle[0].ToolID != ids[7_777] {
		t.Fatalf("needle search = %+v, want tool %s", needle, ids[7_777])
	}

	allowed := []domain.ToolID{ids[5], ids[7_777], ids[9_999]}
	scoped, err := search.Execute(context.Background(), application.SearchToolsRequest{
		Query:          "catalog common",
		Limit:          10,
		AllowedToolIDs: allowed,
	})
	if err != nil {
		t.Fatalf("scoped search error = %v", err)
	}
	if len(scoped) != len(allowed) {
		t.Fatalf("scoped result count = %d, want %d", len(scoped), len(allowed))
	}
	allowedSet := make(map[domain.ToolID]struct{}, len(allowed))
	for _, id := range allowed {
		allowedSet[id] = struct{}{}
	}
	for _, result := range scoped {
		if _, ok := allowedSet[result.ToolID]; !ok {
			t.Fatalf("scoped search returned disallowed tool %s", result.ToolID)
		}
	}
}

func BenchmarkCatalogSearch10K(b *testing.B) {
	store, _ := seedScaleCatalog(b, scaleToolCount)
	search := application.NewSearchTools(store)
	request := application.SearchToolsRequest{
		Query: "catalog common",
		Limit: 20,
	}

	b.ReportAllocs()
	b.ResetTimer()
	for range b.N {
		results, err := search.Execute(context.Background(), request)
		if err != nil {
			b.Fatal(err)
		}
		if len(results) != request.Limit {
			b.Fatalf("result count = %d, want %d", len(results), request.Limit)
		}
	}
}

func BenchmarkCatalogScopedSearch10K(b *testing.B) {
	store, ids := seedScaleCatalog(b, scaleToolCount)
	search := application.NewSearchTools(store)
	allowed := make([]domain.ToolID, 0, 100)
	for i := 0; i < scaleToolCount; i += 100 {
		allowed = append(allowed, ids[i])
	}
	request := application.SearchToolsRequest{
		Query:          "catalog common",
		Limit:          20,
		AllowedToolIDs: allowed,
	}

	b.ReportAllocs()
	b.ResetTimer()
	for range b.N {
		results, err := search.Execute(context.Background(), request)
		if err != nil {
			b.Fatal(err)
		}
		if len(results) != request.Limit {
			b.Fatalf("result count = %d, want %d", len(results), request.Limit)
		}
	}
}

func seedScaleCatalog(tb testing.TB, count int) (*Store, []domain.ToolID) {
	tb.Helper()

	path := filepath.Join(tb.TempDir(), "catalog-scale.db")
	store, err := Open(context.Background(), path)
	if err != nil {
		tb.Fatalf("Open() error = %v", err)
	}
	tb.Cleanup(func() {
		if err := store.Close(); err != nil {
			tb.Errorf("Close() error = %v", err)
		}
	})

	provider := createProviderTB(
		tb,
		store,
		"scale",
		"prov_ffffffffffffffffffffffffffffffff",
	)
	input, err := domain.NewSchema([]byte(`{"type":"object","properties":{"value":{"type":"string"}}}`))
	if err != nil {
		tb.Fatal(err)
	}

	tools := make([]domain.Tool, 0, count)
	ids := make([]domain.ToolID, 0, count)
	for i := range count {
		id, err := domain.ParseToolID(fmt.Sprintf("tool_%032x", i+1))
		if err != nil {
			tb.Fatalf("ParseToolID(%d) error = %v", i, err)
		}
		description := "Catalog common scale search tool"
		if i == 7_777 {
			description += " catalogneedle"
		}
		tool, err := domain.NewTool(
			id,
			provider.ID(),
			domain.ToolSpec{
				SourceKey:   fmt.Sprintf("scale.%05d", i),
				Name:        fmt.Sprintf("catalog_tool_%05d", i),
				Title:       fmt.Sprintf("Catalog tool %05d", i),
				Description: description,
				InputSchema: input,
				Tags:        []string{"catalog", "scale", "common"},
			},
			testTime(),
		)
		if err != nil {
			tb.Fatalf("NewTool(%d) error = %v", i, err)
		}
		tools = append(tools, tool)
		ids = append(ids, id)
	}

	if err := store.ReplaceProviderTools(context.Background(), provider.ID(), tools); err != nil {
		tb.Fatalf("ReplaceProviderTools(%d) error = %v", count, err)
	}
	return store, ids
}

func createProviderTB(
	tb testing.TB,
	store *Store,
	namespaceValue string,
	rawID string,
) domain.Provider {
	tb.Helper()

	namespace, err := domain.NewNamespace(namespaceValue)
	if err != nil {
		tb.Fatal(err)
	}
	provider, err := domain.NewProvider(
		mustProviderIDTB(tb, rawID),
		namespace,
		namespaceValue,
		"",
		testTime(),
	)
	if err != nil {
		tb.Fatal(err)
	}
	if err := store.CreateProvider(context.Background(), provider); err != nil {
		tb.Fatal(err)
	}
	return provider
}

func mustProviderIDTB(tb testing.TB, raw string) domain.ProviderID {
	tb.Helper()
	id, err := domain.ParseProviderID(raw)
	if err != nil {
		tb.Fatal(err)
	}
	return id
}

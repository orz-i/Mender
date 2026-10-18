package sqlite

import (
	"context"
	"fmt"
	"strings"
	"testing"

	"github.com/orz-i/Mender/backend/internal/catalog/application"
	"github.com/orz-i/Mender/backend/internal/catalog/domain"
)

// Long external descriptions must not rank above canonical Tool names/tags.
func TestCatalogSearchPrioritizesNamesAndTagsOverGenericDescriptions(t *testing.T) {
	ctx := context.Background()
	store, err := Open(ctx, fmt.Sprintf("%s/relevance.db", t.TempDir()))
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = store.Close() }()
	provider := createProviderTB(t, store, "relevance", "prov_aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa")
	input, err := domain.NewSchema([]byte(`{"type":"object"}`))
	if err != nil {
		t.Fatal(err)
	}
	specs := []domain.ToolSpec{
		{SourceKey: "search", Name: "anysearch_search", Title: "Search all",
			Description: strings.Repeat("agent ", 400), InputSchema: input},
		{SourceKey: "delegate", Name: "agent_delegate", Title: "Delegate to Agent",
			Description: "Submit task", InputSchema: input},
		{SourceKey: "tagged", Name: "other_service", Title: "Other service",
			Description: "Simple task", InputSchema: input, Tags: []string{"agent"}},
	}
	tools := make([]domain.Tool, 0, len(specs))
	for i, spec := range specs {
		id, err := domain.ParseToolID(fmt.Sprintf("tool_%032x", i+1))
		if err != nil {
			t.Fatal(err)
		}
		tool, err := domain.NewTool(id, provider.ID(), spec, testTime())
		if err != nil {
			t.Fatal(err)
		}
		tools = append(tools, tool)
	}
	if err := store.ReplaceProviderTools(ctx, provider.ID(), tools); err != nil {
		t.Fatal(err)
	}
	results, err := application.NewSearchTools(store).Execute(ctx, application.SearchToolsRequest{
		Query: "agent", Limit: 10,
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(results) != len(specs) || results[0].ToolID != tools[1].ID() {
		t.Fatalf("canonical name did not outrank generic description: %+v", results)
	}
	if results[1].ToolID != tools[2].ID() {
		t.Fatalf("canonical tag did not outrank generic description: %+v", results)
	}
	for _, hostile := range []string{`agent" OR "delegation`, `(agent AND description)`, `agent NEAR(1)`, `"agent`} {
		if _, err := application.NewSearchTools(store).Execute(ctx, application.SearchToolsRequest{
			Query: hostile, Limit: 10, AllowedToolIDs: []domain.ToolID{tools[1].ID()},
		}); err != nil {
			t.Fatalf("user query %q caused SQL/FTS failure: %v", hostile, err)
		}
	}
}

func TestCatalogScopedFTSUsesAllowedIDsBeforeOffsetAndLimit(t *testing.T) {
	ctx := context.Background()
	store, ids := seedScaleCatalog(t, 153)
	pager := application.NewPageTools(store)
	allow := []domain.ToolID{ids[151], ids[152]}
	provider := mustProviderIDTB(t, "prov_ffffffffffffffffffffffffffffffff")
	search := application.NewSearchTools(store)
	results, err := search.Execute(ctx, application.SearchToolsRequest{
		Query: "catalog", Limit: 1, ProviderID: &provider, AllowedToolIDs: allow,
	})
	if err != nil || len(results) != 1 {
		t.Fatalf("allowed SQL-scoped limit 1 = %+v err=%v", results, err)
	}
	if results[0].ToolID != allow[0] && results[0].ToolID != allow[1] {
		t.Fatalf("SQL limit selected hidden tool %s", results[0].ToolID)
	}
	req := application.PageToolsRequest{
		Mode: "search", Query: "catalog", Limit: 1,
		ProviderID: &provider, Profile: "restricted", AllowedToolIDs: allow,
	}
	seen := map[domain.ToolID]bool{}
	for i := 0; i < 4; i++ {
		page, err := pager.Execute(ctx, req)
		if err != nil {
			t.Fatal(err)
		}
		for _, tool := range page.Tools {
			seen[tool.ToolID] = true
		}
		if page.NextCursor == "" {
			break
		}
		req.Cursor = page.NextCursor
	}
	if len(seen) != 2 || !seen[allow[0]] || !seen[allow[1]] {
		t.Fatalf("paged SQL allowlist failed after 153 candidates: %+v", seen)
	}
	other := mustProviderIDTB(t, "prov_bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb")
	results, err = search.Execute(ctx, application.SearchToolsRequest{
		Query: "catalog", Limit: 10, ProviderID: &other, AllowedToolIDs: allow,
	})
	if err != nil || len(results) != 0 {
		t.Fatalf("provider filter bypassed: %+v err=%v", results, err)
	}
	none, err := search.Execute(ctx, application.SearchToolsRequest{
		Query: "catalog", Limit: 10, AllowedToolIDs: []domain.ToolID{},
	})
	if err != nil || len(none) != 0 {
		t.Fatalf("empty bound Profile leaked: %+v err=%v", none, err)
	}
}

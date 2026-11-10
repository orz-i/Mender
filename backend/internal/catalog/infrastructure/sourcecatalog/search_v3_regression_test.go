package sourcecatalog

import (
	"context"
	"fmt"
	"strings"
	"testing"

	app "github.com/orz-i/Mender/backend/internal/catalog/application"
	catalog "github.com/orz-i/Mender/backend/internal/catalog/domain"
	v3schema "github.com/orz-i/Mender/backend/internal/schema/v3"
	v3fixture "github.com/orz-i/Mender/backend/internal/testfixtures/v3"
)

const typedCatalogScaleCount = 10_000

// This fixture intentionally seeds the *canonical* typed Tool/FTS schema. It
// does not create the obsolete provider_id Tool owner or legacy Catalog Store.
func seedTypedCatalog(t testing.TB, count int) (*Repository, []catalog.ToolID, catalog.SourceRef) {
	t.Helper()
	path := fmt.Sprintf("%s/catalog-v3.sqlite", t.TempDir())
	v3fixture.Prepare(t, path)
	db, err := v3schema.OpenContextStore(context.Background(), path)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	provider, _ := catalog.ProviderSource("prov_ffffffffffffffffffffffffffffffff")
	_, err = db.ExecContext(context.Background(), `INSERT INTO providers(id,namespace,display_name,description,created_at,updated_at) VALUES(?,'scale','Scale','','2026-11-09T00:00:00Z','2026-11-09T00:00:00Z')`, provider.ID())
	if err != nil {
		t.Fatal(err)
	}
	repo, err := NewRepository(context.Background(), db, approvedOwners{map[catalog.SourceRef]bool{provider: true}})
	if err != nil {
		t.Fatal(err)
	}
	tx, err := db.BeginTx(context.Background(), nil)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = tx.Rollback() }()
	addTool, err := tx.Prepare(`INSERT INTO tools(id,source_kind,source_id,source_key,name,title,description,input_schema,output_schema,tags_json,created_at,updated_at) VALUES(?,'provider',?,?,?,?,?,?,NULL,?,?,?)`)
	if err != nil {
		t.Fatal(err)
	}
	addFTS, err := tx.Prepare(`INSERT INTO tools_fts(tool_id,source_kind,source_id,name,title,description,tags) VALUES(?,'provider',?,?,?,?,?)`)
	if err != nil {
		t.Fatal(err)
	}
	ids := make([]catalog.ToolID, 0, count)
	for i := range count {
		id, err := catalog.ParseToolID(fmt.Sprintf("tool_%032x", i+1))
		if err != nil {
			t.Fatal(err)
		}
		key, name := fmt.Sprintf("catalog.scale.%05d", i), fmt.Sprintf("catalog_tool_%05d", i)
		description := "Catalog common searchable service"
		if i == 7_777 {
			description += " catalogneedle"
		}
		if _, err = addTool.Exec(id.String(), provider.ID(), key, name, "Catalog Tool", description, `{"type":"object"}`, `["catalog","common"]`, "2026-11-09T00:00:00Z", "2026-11-09T00:00:00Z"); err != nil {
			t.Fatalf("insert Tool %d: %v", i, err)
		}
		if _, err = addFTS.Exec(id.String(), provider.ID(), name, "Catalog Tool", description, "catalog common"); err != nil {
			t.Fatalf("insert FTS %d: %v", i, err)
		}
		ids = append(ids, id)
	}
	if err = addTool.Close(); err != nil {
		t.Fatal(err)
	}
	if err = addFTS.Close(); err != nil {
		t.Fatal(err)
	}
	if err = tx.Commit(); err != nil {
		t.Fatal(err)
	}
	return repo, ids, provider
}

func TestTypedCatalogFTSScopedSearchAtTenThousandTools(t *testing.T) {
	repo, ids, source := seedTypedCatalog(t, typedCatalogScaleCount)
	search := app.NewSearchTools(repo)
	common, err := search.Execute(context.Background(), app.SearchToolsRequest{Query: "catalog common", Limit: 25, SourceRef: &source})
	if err != nil || len(common) != 25 {
		t.Fatalf("typed Catalog 10K search: %v %d", err, len(common))
	}
	needle, err := search.Execute(context.Background(), app.SearchToolsRequest{Query: "catalogneedle", Limit: 10})
	if err != nil || len(needle) != 1 || needle[0].ToolID != ids[7777] {
		t.Fatalf("typed needle: %+v %v", needle, err)
	}
	allowed := []catalog.ToolID{ids[5], ids[7777], ids[9999]}
	scoped, err := search.Execute(context.Background(), app.SearchToolsRequest{Query: "catalog common", Limit: 10, AllowedToolIDs: allowed})
	if err != nil || len(scoped) != len(allowed) {
		t.Fatalf("allowed Tool scope not enforced before LIMIT: %+v %v", scoped, err)
	}
	for _, x := range scoped {
		if x.ToolID != allowed[0] && x.ToolID != allowed[1] && x.ToolID != allowed[2] {
			t.Fatalf("ungranted Tool surfaced: %s", x.ToolID)
		}
	}
	other, _ := catalog.ProviderSource("prov_bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb")
	blocked, err := search.Execute(context.Background(), app.SearchToolsRequest{Query: "catalog common", Limit: 10, SourceRef: &other, AllowedToolIDs: allowed})
	if err != nil || len(blocked) != 0 {
		t.Fatalf("Provider scope escaped: %+v %v", blocked, err)
	}
	for _, hostile := range []string{`catalog" OR "other`, `(catalog AND common)`, `catalog NEAR(1)`, `"catalog`, strings.Repeat("x", 350)} {
		result, err := search.Execute(context.Background(), app.SearchToolsRequest{Query: hostile, Limit: 10, AllowedToolIDs: allowed})
		if err != nil {
			t.Fatalf("unsafe FTS syntax for %q: %v", hostile, err)
		}
		for _, row := range result {
			if row.ToolID != allowed[0] && row.ToolID != allowed[1] && row.ToolID != allowed[2] {
				t.Fatal("hostile query escaped Profile scope")
			}
		}
	}
}
func BenchmarkTypedCatalogSearch10K(b *testing.B) {
	repo, _, source := seedTypedCatalog(b, typedCatalogScaleCount)
	search := app.NewSearchTools(repo)
	q := app.SearchToolsRequest{Query: "catalog common", Limit: 20, SourceRef: &source}
	b.ReportAllocs()
	b.ResetTimer()
	for b.Loop() {
		rows, err := search.Execute(context.Background(), q)
		if err != nil || len(rows) != 20 {
			b.Fatalf("search: %d %v", len(rows), err)
		}
	}
}

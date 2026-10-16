package httpapi

import (
	"context"
	"database/sql"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"path/filepath"
	"testing"
	"time"

	"github.com/orz-i/Mender/backend/internal/bootstrap"
	catalogapplication "github.com/orz-i/Mender/backend/internal/catalog/application"
	_ "modernc.org/sqlite"
)

func TestHTTPBrowseAndSearchCursorCoversLargeServiceCatalog(t *testing.T) {
	ctx := context.Background()
	path := filepath.Join(t.TempDir(), "page-http.db")
	runtime, err := bootstrap.NewRuntime(ctx, bootstrap.RuntimeOptions{DatabasePath: path})
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = runtime.Close() }()
	provider, err := runtime.Catalog.RegisterProvider.Execute(ctx, catalogapplication.RegisterProviderRequest{
		Namespace: "anysearch", DisplayName: "AnySearch",
	})
	if err != nil {
		t.Fatal(err)
	}
	db, err := sql.Open("sqlite", path)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = db.Close() }()
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 205; i++ {
		id := fmt.Sprintf("tool_%032x", i+1)
		name := fmt.Sprintf("search_%03d", i)
		source := fmt.Sprintf("anysearch.%03d", i)
		now := time.Now().UTC().Format(time.RFC3339Nano)
		if _, err = tx.ExecContext(ctx, `INSERT INTO tools(id,provider_id,source_key,name,title,description,input_schema,output_schema,tags_json,created_at,updated_at)
    VALUES(?,?,?,?,?,?,?,NULL,'[]',?,?)`, id, provider.ID().String(), source, name, name, "search the web", []byte(`{"type":"object"}`), now, now); err != nil {
			t.Fatal(err)
		}
		if _, err = tx.ExecContext(ctx, `INSERT INTO tools_fts(tool_id,provider_id,name,title,description,tags) VALUES(?,?,?,?,?,?)`,
			id, provider.ID().String(), name, name, "search the web", "search"); err != nil {
			t.Fatal(err)
		}
	}
	if err = tx.Commit(); err != nil {
		t.Fatal(err)
	}
	server := httptest.NewServer(New(runtime))
	defer server.Close()
	for _, pathName := range []string{"/api/v1/tools/browse?limit=37", "/api/v1/tools?query=search&limit=37"} {
		seen := map[string]bool{}
		firstCursor := ""
		cursor := ""
		for iteration := 0; iteration < 7; iteration++ {
			u := server.URL + pathName + "&providerId=" + url.QueryEscape(provider.ID().String())
			if cursor != "" {
				u += "&cursor=" + url.QueryEscape(cursor)
			}
			result := requestJSON(t, server.Client(), http.MethodGet, u, nil, http.StatusOK)
			records, ok := result["tools"].([]any)
			if !ok {
				t.Fatalf("malformed page %#v", result)
			}
			for _, record := range records {
				tool := record.(map[string]any)
				id := tool["toolId"].(string)
				if seen[id] || tool["providerId"] != provider.ID().String() {
					t.Fatalf("duplicate or foreign tool: %#v", tool)
				}
				seen[id] = true
			}
			cursor, _ = result["nextCursor"].(string)
			if iteration == 0 {
				firstCursor = cursor
			}
			if cursor == "" {
				break
			}
		}
		if len(seen) != 205 || firstCursor == "" {
			t.Fatalf("%s paginated %d/205 tools", pathName, len(seen))
		}
		wrong := server.URL + pathName + "&providerId=prov_ffffffffffffffffffffffffffffffff&cursor=" + url.QueryEscape(firstCursor)
		requestJSON(t, server.Client(), http.MethodGet, wrong, nil, http.StatusBadRequest)
		if _, err = db.ExecContext(ctx, "UPDATE tools SET title='changed' WHERE id=?", fmt.Sprintf("tool_%032x", 1)); err != nil {
			t.Fatal(err)
		}
		stale := server.URL + pathName + "&providerId=" + provider.ID().String() + "&cursor=" + url.QueryEscape(firstCursor)
		requestJSON(t, server.Client(), http.MethodGet, stale, nil, http.StatusConflict)
	}
}

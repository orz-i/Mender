package sqlite

import (
	"context"
	"errors"
	"testing"

	"github.com/orz-i/Mender/backend/internal/catalog/application"
	"github.com/orz-i/Mender/backend/internal/catalog/domain"
)

func TestCatalogPagesBeyondHundredWithScopedCursorAndRevision(t *testing.T) {
	ctx := context.Background()
	store, ids := seedScaleCatalog(t, 253)
	pager := application.NewPageTools(store)
	provider, err := domain.ParseProviderID("prov_ffffffffffffffffffffffffffffffff")
	if err != nil {
		t.Fatal(err)
	}
	for _, mode := range []string{"browse", "search"} {
		t.Run(mode, func(t *testing.T) {
			req := application.PageToolsRequest{Mode: mode, Limit: 43, ProviderID: &provider}
			if mode == "search" {
				req.Query = "catalog common"
			}
			seen := map[domain.ToolID]bool{}
			pages := 0
			for {
				page, e := pager.Execute(ctx, req)
				if e != nil {
					t.Fatal(e)
				}
				pages++
				if len(page.Tools) < 1 || len(page.Tools) > req.Limit {
					t.Fatalf("invalid page length %d", len(page.Tools))
				}
				for _, tool := range page.Tools {
					if tool.ProviderID != provider || seen[tool.ToolID] {
						t.Fatalf("unscoped or duplicated Tool: %+v", tool)
					}
					seen[tool.ToolID] = true
				}
				if page.NextCursor == "" {
					break
				}
				req.Cursor = page.NextCursor
				if pages > 10 {
					t.Fatal("cursor loop")
				}
			}
			if len(seen) != len(ids) || pages < 3 {
				t.Fatalf("paged %d of %d Tools in %d pages", len(seen), len(ids), pages)
			}
		})
	}
	initial, err := pager.Execute(ctx, application.PageToolsRequest{Mode: "browse", Limit: 21})
	if err != nil || initial.NextCursor == "" {
		t.Fatalf("first page %+v %v", initial, err)
	}
	withOtherScope := application.PageToolsRequest{Mode: "browse", Limit: 21, Cursor: initial.NextCursor, ProviderID: &provider}
	if _, err = pager.Execute(ctx, withOtherScope); !errors.Is(err, application.ErrInvalidToolCursor) {
		t.Fatalf("cross-Provider cursor accepted: %v", err)
	}
	if _, err = pager.Execute(ctx, application.PageToolsRequest{Mode: "search", Limit: 21, Query: "catalog common", Cursor: initial.NextCursor}); !errors.Is(err, application.ErrInvalidToolCursor) {
		t.Fatalf("cross-query cursor accepted: %v", err)
	}
	if _, err = pager.Execute(ctx, application.PageToolsRequest{Mode: "browse", Limit: 21, Cursor: "forged"}); !errors.Is(err, application.ErrInvalidToolCursor) {
		t.Fatalf("forged cursor accepted: %v", err)
	}
	revisionBefore, err := store.CatalogRevision(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = store.db.ExecContext(ctx, "UPDATE tools SET title=? WHERE id=?", "Updated", ids[0].String()); err != nil {
		t.Fatal(err)
	}
	revisionAfter, err := store.CatalogRevision(ctx)
	if err != nil || revisionAfter <= revisionBefore {
		t.Fatalf("revision not monotonic: %d => %d (%v)", revisionBefore, revisionAfter, err)
	}
	if _, err = pager.Execute(ctx, application.PageToolsRequest{Mode: "browse", Limit: 21, Cursor: initial.NextCursor}); !errors.Is(err, application.ErrStaleToolCursor) {
		t.Fatalf("mutated Catalog cursor accepted: %v", err)
	}
}

func TestCatalogPagedSearchHonorsProfileRestrictionAndProviderFilter(t *testing.T) {
	ctx := context.Background()
	store, ids := seedScaleCatalog(t, 233)
	pager := application.NewPageTools(store)
	provider, err := domain.ParseProviderID("prov_ffffffffffffffffffffffffffffffff")
	if err != nil {
		t.Fatal(err)
	}
	allowed := make([]domain.ToolID, 0, 30)
	for i := 0; i < len(ids); i += 8 {
		allowed = append(allowed, ids[i])
	}
	req := application.PageToolsRequest{
		Mode: "search", Query: "catalog common", Limit: 7, ProviderID: &provider,
		Profile: "profile.team", AllowedToolIDs: allowed,
	}
	seen := map[domain.ToolID]bool{}
	for pageNumber := 0; pageNumber < 12; pageNumber++ {
		page, e := pager.Execute(ctx, req)
		if e != nil {
			t.Fatal(e)
		}
		for _, tool := range page.Tools {
			seen[tool.ToolID] = true
		}
		if page.NextCursor == "" {
			break
		}
		req.Cursor = page.NextCursor
	}
	if len(seen) != len(allowed) {
		t.Fatalf("Profile-scoped page returned %d, want %d", len(seen), len(allowed))
	}
	for _, id := range allowed {
		if !seen[id] {
			t.Fatalf("missing allowed %s", id)
		}
	}
	req.Cursor = ""
	first, err := pager.Execute(ctx, req)
	if err != nil || first.NextCursor == "" {
		t.Fatalf("restricted first page: %+v %v", first, err)
	}
	req.Cursor = first.NextCursor
	req.Profile = "profile.different"
	if _, err = pager.Execute(ctx, req); !errors.Is(err, application.ErrInvalidToolCursor) {
		t.Fatalf("cross-Profile cursor %v", err)
	}
	req.Profile = "profile.team"
	req.AllowedToolIDs = []domain.ToolID{ids[0]}
	if _, err = pager.Execute(ctx, req); !errors.Is(err, application.ErrInvalidToolCursor) {
		t.Fatalf("modified Profile allowed-set cursor %v", err)
	}
}

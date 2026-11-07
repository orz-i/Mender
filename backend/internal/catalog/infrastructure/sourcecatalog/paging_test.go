package sourcecatalog

import (
	"context"
	"errors"
	"testing"

	app "github.com/orz-i/Mender/backend/internal/catalog/application"
	catalog "github.com/orz-i/Mender/backend/internal/catalog/domain"
)

func TestTypedCatalogUsesSourceRefForBrowseSearchAndRevisionFences(t *testing.T) {
	ctx := context.Background()
	db, repo, provider, agentA, agentB, native := futureFixture(t)
	providerTool := testSourceTool(t, provider, "tool_11111111111111111111111111111111", "api.search", "search")
	firstAgent := testSourceTool(t, agentA, "tool_22222222222222222222222222222222", "agent.delegate", "delegate")
	secondAgent := testSourceTool(t, agentB, "tool_33333333333333333333333333333333", "agent.delegate", "delegate")
	for _, suite := range []struct {
		source catalog.SourceRef
		tools  []catalog.Tool
	}{
		{provider, []catalog.Tool{providerTool}},
		{agentA, []catalog.Tool{firstAgent}},
		{agentB, []catalog.Tool{secondAgent}},
		{native, nativeWorkspaceTools(t, native)},
	} {
		if err := repo.UpsertSourceSnapshot(ctx, suite.source, suite.tools); err != nil {
			t.Fatal(err)
		}
	}
	// The same application BrowseTools and SearchTools ports used by CLI
	// work on v3 sources without Provider-shaped adapter identities.
	browse, err := app.NewBrowseTools(repo).Execute(ctx, app.BrowseToolsRequest{Limit: 10, SourceRef: &agentA})
	if err != nil || len(browse) != 1 || browse[0].ToolID != firstAgent.ID() || browse[0].SourceRef != agentA {
		t.Fatalf("typed BrowseTools lost Agent owner: %+v %v", browse, err)
	}
	searched, err := app.NewSearchTools(repo).Execute(ctx, app.SearchToolsRequest{Query: "delegate", Limit: 10, SourceRef: &agentB})
	if err != nil || len(searched) != 1 || searched[0].ToolID != secondAgent.ID() || searched[0].SourceRef != agentB {
		t.Fatalf("typed SearchTools lost Agent owner: %+v %v", searched, err)
	}
	pager := app.NewPageTools(repo)
	agents, err := pager.Execute(ctx, app.PageToolsRequest{Mode: "search", Query: "delegate", Limit: 10})
	if err != nil || len(agents.Tools) != 2 || agents.Tools[0].SourceRef == agents.Tools[1].SourceRef {
		t.Fatalf("typed Agents not separated by source: %+v %v", agents, err)
	}
	scoped, err := pager.Execute(ctx, app.PageToolsRequest{Mode: "search", Query: "delegate", Limit: 10, SourceRef: &agentA})
	if err != nil || len(scoped.Tools) != 1 || scoped.Tools[0].ToolID != firstAgent.ID() {
		t.Fatalf("typed Agent search crossed sources: %+v %v", scoped, err)
	}
	nativePage, err := pager.Execute(ctx, app.PageToolsRequest{Mode: "browse", Limit: 2, SourceRef: &native})
	if err != nil || len(nativePage.Tools) != 2 || nativePage.NextCursor == "" {
		t.Fatalf("typed Native browse page failed: %+v %v", nativePage, err)
	}
	for _, tool := range nativePage.Tools {
		if tool.SourceRef != native {
			t.Fatalf("Native browse included foreign source: %+v", tool)
		}
	}
	if _, err := pager.Execute(ctx, app.PageToolsRequest{Mode: "browse", Limit: 2, Cursor: nativePage.NextCursor, SourceRef: &agentA}); !errors.Is(err, app.ErrInvalidToolCursor) {
		t.Fatalf("cursor ignored typed source scope: %v", err)
	}
	remaining, err := pager.Execute(ctx, app.PageToolsRequest{Mode: "browse", Limit: 2, SourceRef: &native, Cursor: nativePage.NextCursor})
	if err != nil || len(remaining.Tools) != 2 || remaining.NextCursor != "" {
		t.Fatalf("Native continuation wrong: %+v %v", remaining, err)
	}
	limited, err := pager.Execute(ctx, app.PageToolsRequest{Mode: "search", Query: "delegate", Limit: 10,
		AllowedToolIDs: []catalog.ToolID{firstAgent.ID()}, Profile: "profile-1"})
	if err != nil || len(limited.Tools) != 1 || limited.Tools[0].ToolID != firstAgent.ID() {
		t.Fatalf("typed allowlist escaped Profile: %+v %v", limited, err)
	}
	first, err := pager.Execute(ctx, app.PageToolsRequest{Mode: "browse", Limit: 1})
	if err != nil || first.NextCursor == "" {
		t.Fatalf("no revision-bound cursor: %+v %v", first, err)
	}
	// Refresh a single real Provider's metadata; all catalog cursor snapshots
	// must be invalidated, without changing any ToolID or source ownership.
	if _, err := db.ExecContext(ctx, "UPDATE tools SET title=title||' updated' WHERE id=?", providerTool.ID().String()); err != nil {
		t.Fatal(err)
	}
	if _, err := pager.Execute(ctx, app.PageToolsRequest{Mode: "browse", Limit: 1, Cursor: first.NextCursor}); !errors.Is(err, app.ErrStaleToolCursor) {
		t.Fatalf("stale typed Catalog cursor accepted: %v", err)
	}
}

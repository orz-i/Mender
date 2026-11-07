package sqlite

import (
	"context"
	"encoding/json"
	"fmt"
	"slices"
	"strings"
	"testing"

	"github.com/orz-i/Mender/backend/internal/catalog/application"
	"github.com/orz-i/Mender/backend/internal/catalog/domain"
)

func seedTierLadderCatalog(t *testing.T) (*Store, []domain.Tool) {
	t.Helper()

	store := openTestStore(t)
	provider := createProvider(t, store, "ladder", "prov_000000000000000000000000000000a1")
	input, err := domain.NewSchema([]byte(`{"type":"object"}`))
	if err != nil {
		t.Fatal(err)
	}

	specs := []domain.ToolSpec{
		// Tier 0: canonical Tool name matches the query exactly.
		{SourceKey: "t0", Name: "orion", Title: "Scanner",
			Description: "runs quiet scans", InputSchema: input, Tags: []string{"scan"}},
		// Tier 1: Tool name contains the query prefix.
		{SourceKey: "t1", Name: "orion_probe", Title: "Detector",
			Description: "reads instrument buffers", InputSchema: input, Tags: []string{"detect"}},
		// Tier 2: canonical title matches the query exactly.
		{SourceKey: "t2", Name: "probe_two", Title: "orion",
			Description: "aligns the beacon", InputSchema: input, Tags: []string{"align"}},
		// Tier 3: canonical title contains the query.
		{SourceKey: "t3", Name: "probe_three", Title: "Orion operator",
			Description: "steers the array", InputSchema: input, Tags: []string{"steer"}},
		// Tier 4: only the tags match.
		{SourceKey: "t4", Name: "probe_four", Title: "Field set",
			Description: "collects field samples", InputSchema: input, Tags: []string{"orion"}},
		// Tier 5: only the long generic description matches.
		{SourceKey: "t5", Name: "probe_five", Title: "Field notes",
			Description: "long generic orchestration narrative orion filler verbose " +
				"padding orion repetition closing remarks orion",
			InputSchema: input, Tags: []string{"notes"}},
	}
	tools := make([]domain.Tool, 0, len(specs))
	for i, spec := range specs {
		tool, err := domain.NewTool(
			mustToolID(t, fmt.Sprintf("tool_aaaa%028x", i+1)),
			provider.ID(),
			spec,
			testTime(),
		)
		if err != nil {
			t.Fatal(err)
		}
		tools = append(tools, tool)
	}
	if err := store.ReplaceProviderTools(context.Background(), provider.ID(), tools); err != nil {
		t.Fatal(err)
	}
	return store, tools
}

func seedTieBreakCatalog(t *testing.T) (*Store, []domain.ToolID) {
	t.Helper()

	store := openTestStore(t)
	provider := createProvider(t, store, "ties", "prov_000000000000000000000000000000b1")
	input, err := domain.NewSchema([]byte(`{"type":"object"}`))
	if err != nil {
		t.Fatal(err)
	}

	const count = 8
	tools := make([]domain.Tool, 0, count)
	ids := make([]domain.ToolID, 0, count)
	for i := range count {
		id := mustToolID(t, fmt.Sprintf("tool_bfbf%028x", i))
		tool, err := domain.NewTool(id, provider.ID(), domain.ToolSpec{
			SourceKey:   fmt.Sprintf("ties.%02d", i),
			Name:        "tie_probe",
			Title:       "Tie probe",
			Description: "identical content for deterministic order checks tie",
			InputSchema: input,
			Tags:        []string{"tie"},
		}, testTime())
		if err != nil {
			t.Fatal(err)
		}
		tools = append(tools, tool)
		ids = append(ids, id)
	}
	if err := store.ReplaceProviderTools(context.Background(), provider.ID(), tools); err != nil {
		t.Fatal(err)
	}
	return store, ids
}

func seedScopedPaginationCatalog(t *testing.T) (*Store, []domain.ToolID, []domain.ToolID, []domain.ToolID) {
	t.Helper()

	store, seeds := seedScaleCatalog(t, 150)
	extra := createProviderTB(t, store, "extraprov", "prov_cccccccccccccccccccccccccccccccc")
	input, err := domain.NewSchema([]byte(`{"type":"object","properties":{"value":{"type":"string"}}}`))
	if err != nil {
		t.Fatal(err)
	}
	extraTools := make([]domain.Tool, 0, 2)
	for i := range 2 {
		tool, err := domain.NewTool(
			mustToolID(t, fmt.Sprintf("tool_cccc%028x", i)),
			extra.ID(),
			domain.ToolSpec{
				SourceKey:   fmt.Sprintf("extraprov.%02d", i),
				Name:        fmt.Sprintf("catalog_tool_extra_%02d", i),
				Title:       fmt.Sprintf("Catalog tool extra %02d", i),
				Description: "Catalog common scale search tool extra",
				InputSchema: input,
				Tags:        []string{"catalog", "scale", "common"},
			},
			testTime(),
		)
		if err != nil {
			t.Fatal(err)
		}
		extraTools = append(extraTools, tool)
	}
	if err := store.ReplaceProviderTools(context.Background(), extra.ID(), extraTools); err != nil {
		t.Fatal(err)
	}

	allowed := make([]domain.ToolID, 0, len(seeds)/3+1)
	for i := 0; i < len(seeds); i += 3 {
		allowed = append(allowed, seeds[i])
	}
	allowed = append(allowed, extraTools[1].ID())
	extraIDs := make([]domain.ToolID, 0, len(extraTools))
	for _, tool := range extraTools {
		extraIDs = append(extraIDs, tool.ID())
	}
	return store, seeds, allowed, extraIDs
}

// The six lexical tiers must order canonical fields strictly ahead of the
// long generic description, identically on every search path.
func TestSearchToolsTierLadderPrefersCanonicalFieldsOnAllPaths(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	store, tools := seedTierLadderCatalog(t)
	wantOrder := make([]domain.ToolID, 0, len(tools))
	for _, tool := range tools {
		wantOrder = append(wantOrder, tool.ID())
	}

	toolsResults, err := store.SearchTools(ctx, "orion", 10, application.ToolSearchScope{})
	if err != nil {
		t.Fatalf("SearchTools() error = %v", err)
	}
	gotOrder := make([]domain.ToolID, 0, len(toolsResults))
	for _, tool := range toolsResults {
		gotOrder = append(gotOrder, tool.ID())
	}
	if !slices.Equal(gotOrder, wantOrder) {
		t.Fatalf("tier ladder order = %v, want %v", gotOrder, wantOrder)
	}

	items, err := store.PagedSearchTools(ctx, "orion", 10, application.ToolSearchScope{}, 0)
	if err != nil {
		t.Fatalf("PagedSearchTools() error = %v", err)
	}
	listOrder := make([]domain.ToolID, 0, len(items))
	for _, item := range items {
		listOrder = append(listOrder, item.ToolID)
	}
	if !slices.Equal(listOrder, wantOrder) {
		t.Fatalf("compact path order = %v, want %v", listOrder, wantOrder)
	}

	// Same ladder under a Profile allowlist: restriction narrows the set,
	// never reranks the survivors.
	allowedIDs := []domain.ToolID{tools[1].ID(), tools[3].ID(), tools[4].ID(), tools[5].ID()}
	scopedResults, err := store.SearchTools(ctx, "orion", 10, application.ToolSearchScope{
		AllowedToolIDs: allowedIDs,
	})
	if err != nil {
		t.Fatalf("scoped SearchTools() error = %v", err)
	}
	scopedOrder := make([]domain.ToolID, 0, len(scopedResults))
	for _, tool := range scopedResults {
		scopedOrder = append(scopedOrder, tool.ID())
	}
	wantScoped := []domain.ToolID{tools[1].ID(), tools[3].ID(), tools[4].ID(), tools[5].ID()}
	if !slices.Equal(scopedOrder, wantScoped) {
		t.Fatalf("scoped ladder order = %v, want %v", scopedOrder, wantScoped)
	}
}

// Rows with byte-identical index content carry equal tier and BM25 values;
// only the trailing ToolID tie-break may order them.
func TestSearchToolsTieBreakIsStableToolIDOrder(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	store, ids := seedTieBreakCatalog(t)
	want := slices.Clone(ids)

	results, err := store.SearchTools(ctx, "tie", 10, application.ToolSearchScope{})
	if err != nil {
		t.Fatalf("SearchTools() error = %v", err)
	}
	got := make([]domain.ToolID, 0, len(ids))
	for _, tool := range results {
		got = append(got, tool.ID())
	}
	if !slices.Equal(got, want) {
		t.Fatalf("tie order = %v, want %v", got, want)
	}

	repeat, err := store.SearchTools(ctx, "tie", 10, application.ToolSearchScope{})
	if err != nil {
		t.Fatalf("repeated SearchTools() error = %v", err)
	}
	repeatOrder := make([]domain.ToolID, 0, len(ids))
	for _, tool := range repeat {
		repeatOrder = append(repeatOrder, tool.ID())
	}
	if !slices.Equal(repeatOrder, want) {
		t.Fatalf("repeated tie order = %v, want stable %v", repeatOrder, want)
	}

	scopedWant := want[3:6]
	scoped, err := store.SearchTools(ctx, "tie", 10, application.ToolSearchScope{
		AllowedToolIDs: scopedWant,
	})
	if err != nil {
		t.Fatalf("scoped tie SearchTools() error = %v", err)
	}
	scopedOrder := make([]domain.ToolID, 0, len(scopedWant))
	for _, tool := range scoped {
		scopedOrder = append(scopedOrder, tool.ID())
	}
	if !slices.Equal(scopedOrder, scopedWant) {
		t.Fatalf("scoped tie order = %v, want %v", scopedOrder, scopedWant)
	}

	items, err := store.PagedSearchTools(ctx, "tie", 10, application.ToolSearchScope{}, 0)
	if err != nil {
		t.Fatalf("PagedSearchTools() error = %v", err)
	}
	listOrder := make([]domain.ToolID, 0, len(ids))
	for _, item := range items {
		listOrder = append(listOrder, item.ToolID)
	}
	if !slices.Equal(listOrder, want) {
		t.Fatalf("compact tie order = %v, want %v", listOrder, want)
	}
}

// 150 matching Tools with a Profile allowlist and a Provider filter: the
// restriction must be applied in SQL before ranking, LIMIT and OFFSET so the
// paged windows join into exactly the restricted ranking.
func TestSearchToolsScopedPaginationRestrictsBeforeRankingWindow(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	store, seeds, allowed, extraIDs := seedScopedPaginationCatalog(t)
	query := "catalog common"

	unscoped, err := store.SearchTools(ctx, query, 200, application.ToolSearchScope{})
	if err != nil {
		t.Fatalf("unscoped SearchTools() error = %v", err)
	}
	if len(unscoped) != len(seeds)+2 {
		t.Fatalf("unscoped search = %d tools, want %d", len(unscoped), len(seeds)+2)
	}
	unscopedOrder := make([]domain.ToolID, 0, len(unscoped))
	for _, tool := range unscoped {
		unscopedOrder = append(unscopedOrder, tool.ID())
	}

	allowedSet := make(map[domain.ToolID]bool, len(allowed))
	for _, id := range allowed {
		allowedSet[id] = true
	}
	wantOrder := slices.DeleteFunc(slices.Clone(unscopedOrder), func(id domain.ToolID) bool {
		return !allowedSet[id]
	})
	if len(wantOrder) != len(allowed) {
		t.Fatalf("%d of %d allowances match the FTS query", len(wantOrder), len(allowed))
	}

	seen := make([]domain.ToolID, 0, len(allowed))
	for offset := 0; ; offset += 10 {
		page, pageErr := store.pagedSearchToolDomain(
			ctx, query, 10, application.ToolSearchScope{AllowedToolIDs: allowed}, offset,
		)
		if pageErr != nil {
			t.Fatalf("scoped pagedSearchToolDomain() error = %v", pageErr)
		}
		for _, tool := range page {
			seen = append(seen, tool.ID())
			if !allowedSet[tool.ID()] {
				t.Fatalf("hidden Tool %s leaked through the scoped page", tool.ID())
			}
		}
		if len(page) < 10 {
			break
		}
	}
	if !slices.Equal(seen, wantOrder) {
		t.Fatalf("scoped page join = %d tools, want the restricted ranking %d in order",
			len(seen), len(wantOrder))
	}

	items, err := store.PagedSearchTools(ctx, query, 10, application.ToolSearchScope{
		AllowedToolIDs: allowed,
	}, 0)
	if err != nil {
		t.Fatalf("scoped PagedSearchTools() error = %v", err)
	}
	firstWindow := make([]domain.ToolID, 0, 10)
	for _, item := range items {
		firstWindow = append(firstWindow, item.ToolID)
	}
	if !slices.Equal(firstWindow, seen[:10]) {
		t.Fatalf("compact window = %v, want %v", firstWindow, seen[:10])
	}

	// Provider + allowlist intersect; the extra-provider Tool stays, the
	// scale-provider allowances are filtered away.
	extraProvider, err := domain.ParseProviderID("prov_cccccccccccccccccccccccccccccccc")
	if err != nil {
		t.Fatal(err)
	}
	extraScoped, err := store.SearchTools(ctx, query, 10, application.ToolSearchScope{
		SourceRef:      testProviderSource(extraProvider),
		AllowedToolIDs: allowed,
	})
	if err != nil {
		t.Fatalf("provider scoped SearchTools() error = %v", err)
	}
	if len(extraScoped) != 1 || extraScoped[0].ProviderID() != extraProvider {
		t.Fatalf("provider scoped search = %+v, want only the extra-provider allowance", extraScoped)
	}

	// Provider filter alone keeps the ranking intact.
	providerOnly, err := store.SearchTools(ctx, query, 10, application.ToolSearchScope{
		SourceRef: testProviderSource(extraProvider),
	})
	if err != nil {
		t.Fatalf("provider SearchTools() error = %v", err)
	}
	extraSet := make(map[domain.ToolID]bool, len(extraIDs))
	for _, id := range extraIDs {
		extraSet[id] = true
	}
	providerWant := slices.DeleteFunc(slices.Clone(unscopedOrder), func(id domain.ToolID) bool {
		return !extraSet[id]
	})
	gotProvider := make([]domain.ToolID, 0, len(providerOnly))
	for _, tool := range providerOnly {
		gotProvider = append(gotProvider, tool.ID())
	}
	if !slices.Equal(gotProvider, providerWant) {
		t.Fatalf("provider-only order = %v, want %v", gotProvider, providerWant)
	}

	// A non-nil empty allowlist stays distinct from nil (unrestricted):
	// nothing is ever visible, no SQL rows escape.
	emptyScope := application.ToolSearchScope{AllowedToolIDs: []domain.ToolID{}}
	domainEmpty, err := store.SearchTools(ctx, query, 10, emptyScope)
	if err != nil || len(domainEmpty) != 0 {
		t.Fatalf("empty allowance domain search = %+v err=%v", domainEmpty, err)
	}
	listEmpty, err := store.PagedSearchTools(ctx, query, 10, emptyScope, 3)
	if err != nil || len(listEmpty) != 0 {
		t.Fatalf("empty allowance page = %+v err=%v", listEmpty, err)
	}

	beyond, err := store.pagedSearchToolDomain(ctx, query, 10,
		application.ToolSearchScope{AllowedToolIDs: allowed}, 10_000)
	if err != nil || len(beyond) != 0 {
		t.Fatalf("offset beyond restricted window = %+v err=%v", beyond, err)
	}
}

// Hostile user input must fail closed: FTS5 syntax is fenced inside quoted
// phrases, scope containment holds for every result, and whitespace-only
// queries short-circuit before any SQL runs.
func TestSearchToolsHostileQueriesFailClosedAndStayScoped(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	store, hidden := seedHostileCatalogFixture(t)
	allowed := []domain.ToolID{hidden[0].ID(), hidden[1].ID()}
	allowedSet := map[domain.ToolID]bool{hidden[0].ID(): true, hidden[1].ID(): true}

	queries := []string{
		"", "   ", "*", `"`, `" OR 1=1 --`, `NEAR(a b)`, `x: y " z`, `\`,
		`-agent +x ^y`, `a"b"c`, `agent) OR (1=1`, `"agent"* AND "missing"*`,
		strings.Repeat(`"`, 97), strings.Repeat("agent ", 400), "agent 🚀",
	}
	for _, query := range queries {
		results, err := store.SearchTools(ctx, query, 10, application.ToolSearchScope{
			AllowedToolIDs: allowed,
		})
		if err != nil {
			continue // fail closed with an error, never with a wider result
		}
		for _, tool := range results {
			if !allowedSet[tool.ID()] {
				t.Fatalf("hostile query %q leaked disallowed Tool %s", query, tool.ID())
			}
		}

		offsetResults, err := store.pagedSearchToolDomain(ctx, query, 1,
			application.ToolSearchScope{AllowedToolIDs: allowed}, 1)
		if err != nil {
			continue
		}
		for _, tool := range offsetResults {
			if !allowedSet[tool.ID()] {
				t.Fatalf("hostile offset query %q leaked Tool %s", query, tool.ID())
			}
		}

		itemResults, err := store.PagedSearchTools(ctx, query, 3, application.ToolSearchScope{
			AllowedToolIDs: allowed,
		}, 1)
		if err != nil {
			continue
		}
		for _, item := range itemResults {
			if !allowedSet[item.ToolID] {
				t.Fatalf("hostile compact query %q leaked Tool %s", query, item.ToolID)
			}
		}
	}

	for _, query := range []string{"", "   ", "*", `\`, strings.Repeat(`"`, 97)} {
		if _, err := store.SearchTools(ctx, query, 10, application.ToolSearchScope{}); err != nil {
			t.Fatalf("safe hostile query %q errored unscoped: %v", query, err)
		}
	}

	// A matching positive control proves the scoped fixture has live results;
	// the hostile inputs above are checked independently for containment.
	live, err := store.SearchTools(ctx, "agent", 10, application.ToolSearchScope{
		AllowedToolIDs: allowed,
	})
	if err != nil {
		t.Fatalf("tokenized hostile query errored: %v", err)
	}
	if len(live) == 0 {
		t.Fatalf("tokenized hostile query returned nothing for allowed tools")
	}
	for _, tool := range live {
		if !allowedSet[tool.ID()] {
			t.Fatalf("hostile live query leaked disallowed Tool %s", tool.ID())
		}
	}
}

func seedHostileCatalogFixture(t *testing.T) (*Store, []domain.Tool) {
	t.Helper()

	store := openTestStore(t)
	provider := createProvider(t, store, "hostile", "prov_000000000000000000000000000000c1")
	input, err := domain.NewSchema([]byte(`{"type":"object"}`))
	if err != nil {
		t.Fatal(err)
	}

	specs := []domain.ToolSpec{
		{SourceKey: "h1", Name: "agent_delegate", Title: "Delegate to Agent",
			Description: "Submit task", InputSchema: input},
		{SourceKey: "h2", Name: "other_service", Title: "Other service",
			Description: "Simple task", InputSchema: input, Tags: []string{"agent"}},
		{SourceKey: "hidden", Name: "plain_worker", Title: "Plain worker",
			Description: "does not mention the probe word", InputSchema: input},
	}
	tools := make([]domain.Tool, 0, len(specs))
	for i, spec := range specs {
		tool, err := domain.NewTool(
			mustToolID(t, fmt.Sprintf("tool_f00d%028x", i)),
			provider.ID(),
			spec,
			testTime(),
		)
		if err != nil {
			t.Fatal(err)
		}
		tools = append(tools, tool)
	}
	if err := store.ReplaceProviderTools(context.Background(), provider.ID(), tools); err != nil {
		t.Fatal(err)
	}
	return store, tools
}

// The statement builder is the single composition point: both select
// prefixes, with and without scope fragments, must carry the same weighted
// and tiered rank clause before LIMIT/OFFSET.
func TestSearchStatementComposesScopeAndRankForEveryPath(t *testing.T) {
	t.Parallel()

	provider, err := domain.ParseProviderID("prov_00000000000000000000000000000001")
	if err != nil {
		t.Fatal(err)
	}
	allowedID := mustToolID(t, "tool_00000000000000000000000000000002")
	variants := []struct {
		name        string
		prefix      string
		scope       application.ToolSearchScope
		wantArgs    int
		hasAllowed  bool
		hasProvider bool
	}{
		{name: "unscoped", prefix: searchToolsStatement, scope: application.ToolSearchScope{}, wantArgs: 8},
		{name: "provider", prefix: searchToolsStatement,
			scope: application.ToolSearchScope{SourceRef: testProviderSource(provider)}, wantArgs: 9, hasProvider: true},
		{name: "allowed", prefix: searchToolsStatement,
			scope:    application.ToolSearchScope{AllowedToolIDs: []domain.ToolID{allowedID}},
			wantArgs: 9, hasAllowed: true},
		{name: "paged-unscoped", prefix: pagedSearchToolsStatement,
			scope: application.ToolSearchScope{}, wantArgs: 8},
		{name: "paged-both", prefix: pagedSearchToolsStatement,
			scope: application.ToolSearchScope{SourceRef: testProviderSource(provider),
				AllowedToolIDs: []domain.ToolID{allowedID}},
			wantArgs: 10, hasAllowed: true, hasProvider: true},
	}

	for _, variant := range variants {
		t.Run(variant.name, func(t *testing.T) {
			statement, args, err := searchStatement(
				variant.prefix, `"agent"*`, "agent", 20, 40, variant.scope,
			)
			if err != nil {
				t.Fatalf("searchStatement() error = %v", err)
			}

			rankIndex := strings.Index(statement, "ORDER BY CASE\n WHEN lower(t.name) = ? THEN 0")
			if rankIndex < 0 || !strings.Contains(statement, "bm25(tools_fts, 0.0, 0.0, 12.0, 8.0, 1.0, 6.0), t.id") {
				t.Fatalf("rank clause missing from %q", statement)
			}
			limitIndex := strings.LastIndex(statement, "LIMIT ? OFFSET ?")
			if limitIndex < 0 || rankIndex > limitIndex {
				t.Fatalf("rank clause must precede LIMIT/OFFSET: %q", statement)
			}

			if hasProvider := strings.Contains(statement, "tools_fts.provider_id = ?"); hasProvider != variant.hasProvider {
				t.Fatalf("provider fragment %v, want %v", hasProvider, variant.hasProvider)
			}
			if hasAllowed := strings.Contains(statement, "json_each(?)"); hasAllowed != variant.hasAllowed {
				t.Fatalf("allowlist fragment %v, want %v", hasAllowed, variant.hasAllowed)
			}
			if gotArgs := len(args); gotArgs != variant.wantArgs {
				t.Fatalf("args = %d, want %d", gotArgs, variant.wantArgs)
			}
			if gotMatch := args[0]; gotMatch != `"agent"*` {
				t.Fatalf("match arg = %v", gotMatch)
			}
			if variant.hasProvider && args[1] != provider.String() {
				t.Fatalf("provider arg = %v", args[1])
			}
			if variant.hasAllowed {
				allowedIndex := 1
				if variant.hasProvider {
					allowedIndex = 2
				}
				encoded, ok := args[allowedIndex].(string)
				if !ok {
					t.Fatalf("allowlist arg type = %T", args[allowedIndex])
				}
				var decoded []string
				if err := json.Unmarshal([]byte(encoded), &decoded); err != nil {
					t.Fatalf("allowlist JSON = %q: %v", encoded, err)
				}
				if len(decoded) != 1 || decoded[0] != allowedID.String() {
					t.Fatalf("allowlist JSON = %q", encoded)
				}
			}
			// The trailing term, limit and offset parameters follow the
			// scope fragments in placeholder order.
			if !strings.Contains(statement, "ELSE 5 END") {
				t.Fatalf("tier fallback missing: %q", statement)
			}
		})
	}
}

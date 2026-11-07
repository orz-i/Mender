package sourcecatalog

import (
	"context"
	"database/sql"
	"errors"
	"path/filepath"
	"reflect"
	"slices"
	"strings"
	"testing"
	"time"

	catalog "github.com/orz-i/Mender/backend/internal/catalog/domain"
	legacysqlite "github.com/orz-i/Mender/backend/internal/catalog/infrastructure/sqlite"
	"github.com/orz-i/Mender/backend/internal/toolsuite/infrastructure/curated"
	_ "modernc.org/sqlite"
)

type approvedOwners struct{ allowed map[catalog.SourceRef]bool }

func (a approvedOwners) VerifySourceOwner(_ context.Context, s catalog.SourceRef) error {
	if !a.allowed[s] {
		return errors.New("unknown source owner")
	}
	return nil
}

func testDatabase(t *testing.T) *sql.DB {
	t.Helper()
	db, err := sql.Open("sqlite", filepath.Join(t.TempDir(), "future-catalog.sqlite"))
	if err != nil {
		t.Fatal(err)
	}
	db.SetMaxOpenConns(1)
	t.Cleanup(func() { _ = db.Close() })
	for _, statement := range FreshSchema() {
		if _, err := db.ExecContext(context.Background(), statement); err != nil {
			t.Fatalf("future schema statement failed: %v: %s", err, statement)
		}
	}
	return db
}

func testSourceTool(t *testing.T, source catalog.SourceRef, id, key, name string) catalog.Tool {
	t.Helper()
	input, err := catalog.NewSchema([]byte(`{"type":"object"}`))
	if err != nil {
		t.Fatal(err)
	}
	toolID, err := catalog.ParseToolID(id)
	if err != nil {
		t.Fatal(err)
	}
	tool, err := catalog.NewToolFromSource(toolID, source, catalog.ToolSpec{
		SourceKey: key, Name: name, Title: name, Description: key,
		InputSchema: input, Tags: []string{"source", key},
	}, time.Date(2026, 11, 6, 10, 0, 0, 0, time.UTC))
	if err != nil {
		t.Fatal(err)
	}
	return tool
}

func futureFixture(t *testing.T) (*sql.DB, *Repository, catalog.SourceRef, catalog.SourceRef, catalog.SourceRef, catalog.SourceRef) {
	t.Helper()
	db := testDatabase(t)
	provider, _ := catalog.ProviderSource("prov_11111111111111111111111111111111")
	agentA, _ := catalog.AgentSource("agent_aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa")
	agentB, _ := catalog.AgentSource("agent_bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb")
	native, _ := catalog.NativeSource("workspace")
	_, err := db.Exec(`INSERT INTO providers(id,namespace,display_name,description,created_at,updated_at)
		VALUES(?,'example','Example','','2026-11-06T10:00:00Z','2026-11-06T10:00:00Z')`, provider.ID())
	if err != nil {
		t.Fatal(err)
	}
	owners := approvedOwners{map[catalog.SourceRef]bool{
		provider: true, agentA: true, agentB: true, native: true,
	}}
	repo, err := NewRepository(context.Background(), db, owners)
	if err != nil {
		t.Fatal(err)
	}
	return db, repo, provider, agentA, agentB, native
}

func nativeWorkspaceTools(t *testing.T, source catalog.SourceRef) []catalog.Tool {
	t.Helper()
	defs, err := curated.PlannedNativeCatalogTools(time.Date(2026, 11, 6, 10, 0, 0, 0, time.UTC))
	if err != nil {
		t.Fatal(err)
	}
	result := []catalog.Tool{}
	for _, def := range defs {
		if def.SourceRef() == source {
			result = append(result, def)
		}
	}
	return result
}

func TestSourceRepositoryFailsClosedWithLegacyOrMissingSchema(t *testing.T) {
	if _, err := NewRepository(context.Background(), nil, approvedOwners{}); err == nil {
		t.Fatal("nil db accepted")
	}
	db := testDatabase(t)
	if _, err := NewRepository(context.Background(), db, nil); err == nil {
		t.Fatal("nil owner verifier accepted")
	}
	legacyPath := filepath.Join(t.TempDir(), "old.sqlite")
	legacy, err := legacysqlite.Open(context.Background(), legacyPath)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = legacy.Close() }()
	oldDB, err := sql.Open("sqlite", legacyPath)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = oldDB.Close() }()
	if _, err := NewRepository(context.Background(), oldDB, approvedOwners{}); err == nil {
		t.Fatal("empty/old schema must not be silently migrated")
	}
}

func TestSourceRepositoryProviderAgentNativeSnapshotsRemainIndependent(t *testing.T) {
	ctx := context.Background()
	db, repo, provider, agentA, agentB, native := futureFixture(t)
	providerTool := testSourceTool(t, provider, "tool_11111111111111111111111111111111", "issues.read", "read_issue")
	agentToolA := testSourceTool(t, agentA, "tool_22222222222222222222222222222222", "agent.delegate", "delegate")
	agentToolB := testSourceTool(t, agentB, "tool_33333333333333333333333333333333", "agent.delegate", "delegate")
	for _, tc := range []struct {
		source catalog.SourceRef
		tools  []catalog.Tool
	}{
		{provider, []catalog.Tool{providerTool}},
		{agentA, []catalog.Tool{agentToolA}},
		{agentB, []catalog.Tool{agentToolB}},
		{native, nativeWorkspaceTools(t, native)},
	} {
		if err := repo.UpsertSourceSnapshot(ctx, tc.source, tc.tools); err != nil {
			t.Fatalf("upsert %s: %v", tc.source, err)
		}
		recovered, err := repo.ListSourceTools(ctx, tc.source)
		if err != nil || !reflect.DeepEqual(snapshotIDs(recovered), snapshotIDs(tc.tools)) {
			t.Fatalf("source %s lost exact ToolIDs, %v %v", tc.source, snapshotIDs(recovered), err)
		}
	}
	for _, expected := range []catalog.Tool{providerTool, agentToolA, agentToolB} {
		got, err := repo.GetTool(ctx, expected.ID())
		if err != nil || got.SourceRef() != expected.SourceRef() || got.ID() != expected.ID() {
			t.Fatalf("get Tool %s: %+v %v", expected.ID(), got, err)
		}
	}
	counts, err := repo.ListSourceCounts(ctx)
	if err != nil || len(counts) != 4 {
		t.Fatalf("source count mismatch: %+v, %v", counts, err)
	}
	if counts[0].Source == counts[1].Source {
		t.Fatal("same Backend Agents collapsed")
	}
	agentMatches, err := repo.SearchSource(ctx, agentA, "delegate", 10)
	if err != nil || len(agentMatches) != 1 || agentMatches[0].ID() != agentToolA.ID() {
		t.Fatalf("Agent scoped FTS leaked another Tool: %+v %v", agentMatches, err)
	}
	nativeMatches, err := repo.SearchSource(ctx, native, "read", 10)
	if err != nil || len(nativeMatches) != 1 || nativeMatches[0].SourceRef() != native {
		t.Fatalf("native scoped FTS: %+v %v", nativeMatches, err)
	}
	if matches, err := repo.SearchSource(ctx, provider, "delegate", 10); err != nil || len(matches) != 0 {
		t.Fatalf("Provider received Agent result: %+v %v", matches, err)
	}
	for _, query := range []string{"", strings.Repeat("long", 100), "\" OR *", "read) OR native:jobs"} {
		_, err := repo.SearchSource(ctx, provider, query, 10)
		if query == "" || len(query) > 256 {
			if err == nil {
				t.Errorf("accepted invalid FTS query %q", query)
			}
		} else if err != nil {
			t.Errorf("FTS query %q should be safely quoted: %v", query, err)
		}
	}
	var ftsRows int
	if err := db.QueryRow("SELECT COUNT(*) FROM tools_fts").Scan(&ftsRows); err != nil || ftsRows != 7 {
		t.Fatalf("fts rows=%d err=%v", ftsRows, err)
	}
	assertDBIntegrity(t, db)
}

func TestSourceRepositoryPreservesIdentityOnMetadataRefreshAndFailsClosed(t *testing.T) {
	ctx := context.Background()
	db, repo, provider, agentA, _, native := futureFixture(t)
	item := testSourceTool(t, agentA, "tool_11111111111111111111111111111111", "agent.delegate", "delegate")
	if err := repo.UpsertSourceSnapshot(ctx, agentA, []catalog.Tool{item}); err != nil {
		t.Fatal(err)
	}
	updated, err := item.RefreshMetadata(catalog.ToolSpec{
		SourceKey: item.SourceKey(), Name: "new_delegate", Title: "Delegated task",
		Description: "delegate", InputSchema: item.InputSchema(),
	}, item.CreatedAt().Add(time.Second))
	if err != nil {
		t.Fatal(err)
	}
	if err := repo.UpsertSourceSnapshot(ctx, agentA, []catalog.Tool{updated}); err != nil {
		t.Fatal(err)
	}
	loaded, err := repo.GetTool(ctx, item.ID())
	if err != nil || loaded.Title() != "Delegated task" || !loaded.CreatedAt().Equal(item.CreatedAt()) {
		t.Fatalf("metadata refresh/ToolID changed: %+v %v", loaded, err)
	}
	if hits, err := repo.SearchSource(ctx, agentA, "new_delegate", 10); err != nil || len(hits) != 1 {
		t.Fatalf("FTS not refreshed: %+v %v", hits, err)
	}
	// A different source cannot adopt an existing ToolID by a new SourceKey.
	adopt := testSourceTool(t, provider, item.ID().String(), "issues.read", "issue")
	if err := repo.UpsertSourceSnapshot(ctx, provider, []catalog.Tool{adopt}); err == nil {
		t.Fatal("cross-source ToolID adoption succeeded")
	}
	reset := testSourceTool(t, agentA, item.ID().String(), item.SourceKey(), item.Name())
	if err := repo.UpsertSourceSnapshot(ctx, agentA, []catalog.Tool{reset}); !errors.Is(err, ErrSourceIdentityConflict) {
		t.Fatalf("Tool timestamp regression accepted: %v", err)
	}
	changedID := testSourceTool(t, agentA, "tool_22222222222222222222222222222222", "agent.delegate", "delegate")
	if err := repo.UpsertSourceSnapshot(ctx, agentA, []catalog.Tool{changedID}); !errors.Is(err, ErrSourceIdentityConflict) {
		t.Fatalf("ToolID rebind allowed: %v", err)
	}
	if err := repo.UpsertSourceSnapshot(ctx, agentA, nil); !errors.Is(err, ErrRetirementRequiresReview) {
		t.Fatalf("silently retired selected ToolID: %v", err)
	}
	invalidNative := testSourceTool(t, native, "tool_aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa", "workspace.read", "read")
	if err := repo.UpsertSourceSnapshot(ctx, native, []catalog.Tool{invalidNative}); err == nil {
		t.Fatal("arbitrary ToolID claimed privileged native source")
	}
	validNative := nativeWorkspaceTools(t, native)
	forged, err := validNative[0].RefreshMetadata(catalog.ToolSpec{
		SourceKey: validNative[0].SourceKey(), Name: validNative[0].Name(), Title: validNative[0].Title(),
		Description: validNative[0].Description(), InputSchema: validNative[0].InputSchema(),
		OutputSchema: validNative[0].OutputSchema(), Tags: []string{"unreviewed"},
	}, validNative[0].UpdatedAt().Add(time.Second))
	if err != nil {
		t.Fatal(err)
	}
	invalidSnapshot := append([]catalog.Tool{forged}, validNative[1:]...)
	if err := repo.UpsertSourceSnapshot(ctx, native, invalidSnapshot); err == nil {
		t.Fatal("native metadata was replaced with unreviewed tags")
	}
	if err := repo.UpsertSourceSnapshot(ctx, native, validNative[:1]); err == nil {
		t.Fatal("incomplete native source accepted")
	}
	badSource, _ := catalog.AgentSource("agent_00000000000000000000000000000000")
	if err := repo.UpsertSourceSnapshot(ctx, badSource, []catalog.Tool{}); !errors.Is(err, ErrUnverifiedSource) {
		t.Fatalf("unowned Agent source accepted: %v", err)
	}
	// A spoofed "approved" Provider is still checked by the SQLite trigger.
	ghost, _ := catalog.ProviderSource("prov_99999999999999999999999999999999")
	overtrusted, _ := NewRepository(ctx, db, approvedOwners{map[catalog.SourceRef]bool{ghost: true}})
	missingProvider := testSourceTool(t, ghost, "tool_44444444444444444444444444444444", "ghost.read", "ghost")
	if err := overtrusted.UpsertSourceSnapshot(ctx, ghost, []catalog.Tool{missingProvider}); err == nil {
		t.Fatal("owner check bypassed SQL Provider foreign owner")
	}
	counts, err := repo.ListSourceCounts(ctx)
	if err != nil || len(counts) != 1 || counts[0].Count != 1 {
		t.Fatalf("failed transaction partially committed: %+v %v", counts, err)
	}
	assertDBIntegrity(t, db)
}

func assertDBIntegrity(t *testing.T, db *sql.DB) {
	t.Helper()
	var integrity string
	if err := db.QueryRow("PRAGMA integrity_check").Scan(&integrity); err != nil || integrity != "ok" {
		t.Fatalf("integrity=%q err=%v", integrity, err)
	}
	rows, err := db.Query("PRAGMA foreign_key_check")
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = rows.Close() }()
	if rows.Next() {
		t.Fatal("source catalog has broken foreign references")
	}
}

func TestSourceRepositoryConcurrentUpdatesRetainExactSource(t *testing.T) {
	ctx := context.Background()
	_, repo, _, agentA, agentB, _ := futureFixture(t)
	a := testSourceTool(t, agentA, "tool_55555555555555555555555555555555", "agent.delegate", "delegate")
	b := testSourceTool(t, agentB, "tool_66666666666666666666666666666666", "agent.delegate", "delegate")
	// The single owned DB writer must serialize source-specific transactions.
	// This does not replace the cross-context owner-delete SQL guards in E.
	if err := repo.UpsertSourceSnapshot(ctx, agentA, []catalog.Tool{a}); err != nil {
		t.Fatal(err)
	}
	if err := repo.UpsertSourceSnapshot(ctx, agentB, []catalog.Tool{b}); err != nil {
		t.Fatal(err)
	}
	listA, _ := repo.ListSourceTools(ctx, agentA)
	listB, _ := repo.ListSourceTools(ctx, agentB)
	if len(listA) != 1 || len(listB) != 1 || listA[0].SourceRef() == listB[0].SourceRef() {
		t.Fatal("same-name Agent Tools merged across independent owners")
	}
}

func snapshotIDs(tools []catalog.Tool) []catalog.ToolID {
	out := make([]catalog.ToolID, 0, len(tools))
	for _, tool := range tools {
		out = append(out, tool.ID())
	}
	slices.Sort(out)
	return out
}

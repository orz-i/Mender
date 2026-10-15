package sqlite

import (
	"context"
	"errors"
	"path/filepath"
	"sync"
	"testing"

	catalogdomain "github.com/orz-i/Mender/backend/internal/catalog/domain"
	profileapplication "github.com/orz-i/Mender/backend/internal/profile/application"
	profiledomain "github.com/orz-i/Mender/backend/internal/profile/domain"
	tunneldomain "github.com/orz-i/Mender/backend/internal/tunnel/domain"
)

func auditProfile(t *testing.T, store *Store) profiledomain.Profile {
	t.Helper()
	id, _ := profiledomain.ParseProfileID("prof_00000000000000000000000000000001")
	slug, _ := profiledomain.NewSlug("audit-profile")
	profile, err := profiledomain.NewProfile(id, slug, "Audit Profile", profiledomain.ExposureDynamic, nil)
	if err != nil {
		t.Fatal(err)
	}
	if err := store.CreateProfile(context.Background(), profile); err != nil {
		t.Fatal(err)
	}
	return profile
}

func auditTool(t *testing.T, id string) catalogdomain.ToolID {
	t.Helper()
	result, err := catalogdomain.ParseToolID(id)
	if err != nil {
		t.Fatal(err)
	}
	return result
}

func TestProfileRevisionPreventsStaleRevocationRestore(t *testing.T) {
	ctx := context.Background()
	store := openTestStore(t)
	initial := auditProfile(t, store)
	toolA := auditTool(t, "tool_00000000000000000000000000000001")
	toolB := auditTool(t, "tool_00000000000000000000000000000002")
	initial, err := initial.AddTool(toolA, true)
	if err != nil {
		t.Fatal(err)
	}
	if err := store.SaveProfile(ctx, initial); err != nil {
		t.Fatal(err)
	}

	revoker, err := store.GetProfileByID(ctx, initial.ID())
	if err != nil {
		t.Fatal(err)
	}
	staleAdder, err := store.GetProfileByID(ctx, initial.ID())
	if err != nil {
		t.Fatal(err)
	}
	if revoker.Revision() != 1 || staleAdder.Revision() != 1 {
		t.Fatalf("initial revision = %d, %d", revoker.Revision(), staleAdder.Revision())
	}

	revoker, err = revoker.RemoveTool(toolA)
	if err != nil {
		t.Fatal(err)
	}
	if err := store.SaveProfile(ctx, revoker); err != nil {
		t.Fatal(err)
	}

	staleAdder, err = staleAdder.AddTool(toolB, true)
	if err != nil {
		t.Fatal(err)
	}
	if err := store.SaveProfile(ctx, staleAdder); !errors.Is(err, profileapplication.ErrRevisionConflict) {
		t.Fatalf("stale AddTool error = %v, want revision conflict", err)
	}

	got, err := store.GetProfileByID(ctx, initial.ID())
	if err != nil {
		t.Fatal(err)
	}
	if got.Contains(toolA) || got.Contains(toolB) || got.Revision() != 2 {
		t.Fatalf("revoke must hold: A=%v B=%v rev=%d", got.Contains(toolA), got.Contains(toolB), got.Revision())
	}
	fresh, err := got.AddTool(toolB, true)
	if err != nil {
		t.Fatal(err)
	}
	if err := store.SaveProfile(ctx, fresh); err != nil {
		t.Fatal(err)
	}
	got, err = store.GetProfileByID(ctx, initial.ID())
	if err != nil {
		t.Fatal(err)
	}
	if got.Contains(toolA) || !got.Contains(toolB) || got.Revision() != 3 {
		t.Fatalf("fresh write: A=%v B=%v rev=%d", got.Contains(toolA), got.Contains(toolB), got.Revision())
	}
	all, err := store.ListProfiles(ctx)
	if err != nil || len(all) != 1 || all[0].Revision() != 3 {
		t.Fatalf("ListProfiles revision = %v, %v", all, err)
	}
}

func TestProfileRevisionIncludesTunnelAndReplacement(t *testing.T) {
	ctx := context.Background()
	store := openTestStore(t)
	initial := auditProfile(t, store)
	editor, err := store.GetProfileByID(ctx, initial.ID())
	if err != nil {
		t.Fatal(err)
	}
	stale, err := store.GetProfileByID(ctx, initial.ID())
	if err != nil {
		t.Fatal(err)
	}
	tunnelID, _ := tunneldomain.ParseTunnelID("tun_00000000000000000000000000000001")
	editor, err = editor.SetTunnel(tunnelID)
	if err != nil {
		t.Fatal(err)
	}
	if err := store.SaveProfile(ctx, editor); err != nil {
		t.Fatal(err)
	}
	stale, err = stale.ReplaceBindings(nil)
	if err != nil {
		t.Fatal(err)
	}
	if err := store.SaveProfile(ctx, stale); !errors.Is(err, profileapplication.ErrRevisionConflict) {
		t.Fatalf("stale replace error = %v", err)
	}
	got, err := store.GetProfileByID(ctx, initial.ID())
	if err != nil {
		t.Fatal(err)
	}
	value, ok := got.TunnelID()
	if !ok || value != tunnelID || got.Revision() != 1 {
		t.Fatalf("tunnel after stale replace: %q, %v, rev=%d", value, ok, got.Revision())
	}
}

func TestProfileRevisionCASAcrossSQLiteConnections(t *testing.T) {
	ctx := context.Background()
	path := filepath.Join(t.TempDir(), "profiles.db")
	a, err := Open(ctx, path)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = a.Close() }()
	b, err := Open(ctx, path)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = b.Close() }()
	initial := auditProfile(t, a)
	snapshotA, err := a.GetProfileByID(ctx, initial.ID())
	if err != nil {
		t.Fatal(err)
	}
	snapshotB, err := b.GetProfileByID(ctx, initial.ID())
	if err != nil {
		t.Fatal(err)
	}
	snapshotA, _ = snapshotA.AddTool(auditTool(t, "tool_00000000000000000000000000000001"), true)
	snapshotB, _ = snapshotB.AddTool(auditTool(t, "tool_00000000000000000000000000000002"), true)

	start := make(chan struct{})
	results := make(chan error, 2)
	var wg sync.WaitGroup
	for _, entry := range []struct {
		store   *Store
		profile profiledomain.Profile
	}{
		{a, snapshotA}, {b, snapshotB},
	} {
		wg.Add(1)
		go func(s *Store, p profiledomain.Profile) {
			defer wg.Done()
			<-start
			results <- s.SaveProfile(ctx, p)
		}(entry.store, entry.profile)
	}
	close(start)
	wg.Wait()
	close(results)
	successes, conflicts := 0, 0
	for err := range results {
		switch {
		case err == nil:
			successes++
		case errors.Is(err, profileapplication.ErrRevisionConflict):
			conflicts++
		default:
			t.Fatalf("unexpected concurrent save error: %v", err)
		}
	}
	if successes != 1 || conflicts != 1 {
		t.Fatalf("concurrent saves: successes=%d conflicts=%d", successes, conflicts)
	}
	got, err := a.GetProfileByID(ctx, initial.ID())
	if err != nil {
		t.Fatal(err)
	}
	if len(got.Bindings()) != 1 || got.Revision() != 1 {
		t.Fatalf("concurrent commits persisted %d bindings at revision %d", len(got.Bindings()), got.Revision())
	}
}

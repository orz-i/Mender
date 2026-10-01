package sqlite

import (
	"context"
	"errors"
	"path/filepath"
	"testing"

	catalogdomain "github.com/orz-i/Mender/backend/internal/catalog/domain"
	profileapplication "github.com/orz-i/Mender/backend/internal/profile/application"
	profiledomain "github.com/orz-i/Mender/backend/internal/profile/domain"
	tunneldomain "github.com/orz-i/Mender/backend/internal/tunnel/domain"
)

func TestProfileRoundTripAndMembershipPersistence(t *testing.T) {
	t.Parallel()

	store := openTestStore(t)
	id, _ := profiledomain.ParseProfileID("prof_00000000000000000000000000000001")
	slug, _ := profiledomain.NewSlug("default")
	first, _ := catalogdomain.ParseToolID("tool_00000000000000000000000000000001")
	second, _ := catalogdomain.ParseToolID("tool_00000000000000000000000000000002")

	firstBinding, err := profiledomain.DefaultToolBinding(first, true)
	if err != nil {
		t.Fatal(err)
	}
	secondBinding, err := profiledomain.DefaultToolBinding(second, false)
	if err != nil {
		t.Fatal(err)
	}
	tunnelID, _ := tunneldomain.ParseTunnelID("tun_00000000000000000000000000000001")
	profile, err := profiledomain.NewProfileWithBindingsAndTunnel(
		id,
		slug,
		"Default",
		profiledomain.ExposureHybrid,
		tunnelID,
		[]profiledomain.ToolBinding{firstBinding, secondBinding},
	)
	if err != nil {
		t.Fatal(err)
	}
	if err := store.CreateProfile(context.Background(), profile); err != nil {
		t.Fatalf("CreateProfile() error = %v", err)
	}

	loadedByID, err := store.GetProfileByID(context.Background(), id)
	if err != nil {
		t.Fatalf("GetProfileByID() error = %v", err)
	}
	loadedTunnelID, hasTunnel := loadedByID.TunnelID()
	if loadedByID.Slug() != slug || len(loadedByID.Bindings()) != 2 ||
		!hasTunnel || loadedTunnelID != tunnelID {
		t.Fatalf("GetProfileByID() = %+v, tunnel = %q", loadedByID.Bindings(), loadedTunnelID)
	}

	loaded, err := store.GetProfileBySlug(context.Background(), slug)
	if err != nil {
		t.Fatalf("GetProfileBySlug() error = %v", err)
	}
	if loaded.ID() != profile.ID() || len(loaded.Bindings()) != 2 || len(loaded.DirectToolIDs()) != 1 {
		t.Fatalf("loaded Profile = %+v", loaded.Bindings())
	}

	updated, err := loaded.RemoveTool(first)
	if err != nil {
		t.Fatal(err)
	}
	if err := store.SaveProfile(context.Background(), updated); err != nil {
		t.Fatalf("SaveProfile() error = %v", err)
	}
	reloaded, err := store.GetProfileBySlug(context.Background(), slug)
	if err != nil {
		t.Fatal(err)
	}
	if reloaded.Contains(first) || !reloaded.Contains(second) {
		t.Fatalf("reloaded bindings = %+v", reloaded.Bindings())
	}

	profiles, err := store.ListProfiles(context.Background())
	if err != nil {
		t.Fatalf("ListProfiles() error = %v", err)
	}
	if len(profiles) != 1 || profiles[0].ID() != id {
		t.Fatalf("ListProfiles() = %+v", profiles)
	}
}

func TestProfileSlugConflict(t *testing.T) {
	t.Parallel()

	store := openTestStore(t)
	slug, _ := profiledomain.NewSlug("default")
	firstID, _ := profiledomain.ParseProfileID("prof_00000000000000000000000000000001")
	secondID, _ := profiledomain.ParseProfileID("prof_00000000000000000000000000000002")
	first, _ := profiledomain.NewProfile(firstID, slug, "First", profiledomain.ExposureDynamic, nil)
	second, _ := profiledomain.NewProfile(secondID, slug, "Second", profiledomain.ExposureDynamic, nil)

	if err := store.CreateProfile(context.Background(), first); err != nil {
		t.Fatal(err)
	}
	if err := store.CreateProfile(context.Background(), second); !errors.Is(err, profileapplication.ErrSlugConflict) {
		t.Fatalf("CreateProfile(conflict) error = %v", err)
	}
}

func openTestStore(t *testing.T) *Store {
	t.Helper()

	store, err := Open(context.Background(), filepath.Join(t.TempDir(), "mender.db"))
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

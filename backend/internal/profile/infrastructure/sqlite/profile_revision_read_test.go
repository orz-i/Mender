package sqlite

import (
	"context"
	"errors"
	"path/filepath"
	"testing"

	profileapplication "github.com/orz-i/Mender/backend/internal/profile/application"
	profiledomain "github.com/orz-i/Mender/backend/internal/profile/domain"
)

func TestProfileRevisionReadObservesOtherConnectionsWithoutMutation(t *testing.T) {
	ctx := context.Background()
	path := filepath.Join(t.TempDir(), "profiles.db")
	first, err := Open(ctx, path)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = first.Close() }()
	second, err := Open(ctx, path)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = second.Close() }()

	id, _ := profiledomain.ParseProfileID("prof_00000000000000000000000000000001")
	slug, _ := profiledomain.NewSlug("live-profile")
	p, err := profiledomain.NewProfileWithBindings(id, slug, "Revision test", profiledomain.ExposureDynamic, nil)
	if err != nil {
		t.Fatal(err)
	}
	read := profileapplication.NewGetProfileRevision(first)
	if _, err := read.Execute(ctx, id); !errors.Is(err, profileapplication.ErrProfileNotFound) {
		t.Fatalf("missing=%v", err)
	}
	if err := second.CreateProfile(ctx, p); err != nil {
		t.Fatal(err)
	}
	if rev, err := read.Execute(ctx, id); err != nil || rev != 0 {
		t.Fatalf("initial revision=%d err=%v", rev, err)
	}

	changed, err := second.GetProfileByID(ctx, id)
	if err != nil {
		t.Fatal(err)
	}
	if err := second.SaveProfile(ctx, changed); err != nil {
		t.Fatal(err)
	}
	if rev, err := read.Execute(ctx, id); err != nil || rev != 1 {
		t.Fatalf("committed external revision=%d err=%v", rev, err)
	}
	if rev, err := read.Execute(ctx, id); err != nil || rev != 1 {
		t.Fatalf("read mutated revision=%d err=%v", rev, err)
	}
}

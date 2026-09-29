package sqlite

import (
	"context"
	"errors"
	"path/filepath"
	"testing"
	"time"

	skillapplication "github.com/orz-i/Mender/backend/internal/skill/application"
	skilldomain "github.com/orz-i/Mender/backend/internal/skill/domain"
)

func TestStoreRoundTripStateAndDelete(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	store, err := Open(ctx, filepath.Join(t.TempDir(), "mender.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = store.Close() })

	first := testInstallation(
		t,
		"sinst_00112233445566778899aabbccddeeff",
		"skill_00112233445566778899aabbccddeeff",
		"example-skill",
		"sha256:0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef",
		time.Date(2026, 9, 28, 12, 0, 0, 0, time.UTC),
	)
	second := testInstallation(
		t,
		"sinst_11112222333344445555666677778888",
		"skill_11112222333344445555666677778888",
		"another-skill",
		"sha256:abcdef0123456789abcdef0123456789abcdef0123456789abcdef0123456789",
		time.Date(2026, 9, 28, 13, 0, 0, 0, time.UTC),
	)
	if err := store.Create(ctx, first); err != nil {
		t.Fatalf("Create(first) error = %v", err)
	}
	if err := store.Create(ctx, second); err != nil {
		t.Fatalf("Create(second) error = %v", err)
	}

	got, err := store.Get(ctx, first.ID())
	if err != nil {
		t.Fatalf("Get() error = %v", err)
	}
	if got.ID() != first.ID() || got.SkillID() != first.SkillID() ||
		got.Digest() != first.Digest() || got.Trusted() || got.Enabled() {
		t.Fatalf("Get() = %+v", got)
	}

	byDigest, err := store.GetBySkillDigest(ctx, first.SkillID(), first.Digest())
	if err != nil {
		t.Fatalf("GetBySkillDigest() error = %v", err)
	}
	if byDigest.ID() != first.ID() {
		t.Fatalf("GetBySkillDigest().ID = %s, want %s", byDigest.ID(), first.ID())
	}

	items, err := store.List(ctx)
	if err != nil {
		t.Fatalf("List() error = %v", err)
	}
	if len(items) != 2 || items[0].Name() != "another-skill" || items[1].Name() != "example-skill" {
		t.Fatalf("List() = %+v", items)
	}

	if err := store.SetState(ctx, first.ID(), true, true); err != nil {
		t.Fatalf("SetState() error = %v", err)
	}
	got, err = store.Get(ctx, first.ID())
	if err != nil {
		t.Fatal(err)
	}
	if !got.Trusted() || !got.Enabled() {
		t.Fatalf("updated state trusted=%v enabled=%v", got.Trusted(), got.Enabled())
	}
	if err := store.SetState(ctx, first.ID(), false, true); err == nil {
		t.Fatal("SetState(untrusted enabled) error = nil")
	}

	if err := store.Delete(ctx, first.ID()); err != nil {
		t.Fatalf("Delete() error = %v", err)
	}
	if _, err := store.Get(ctx, first.ID()); !errors.Is(err, skillapplication.ErrInstallationNotFound) {
		t.Fatalf("Get(deleted) error = %v", err)
	}
}

func TestStoreRejectsDuplicateSkillDigest(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	store, err := Open(ctx, filepath.Join(t.TempDir(), "mender.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = store.Close() })

	first := testInstallation(
		t,
		"sinst_00112233445566778899aabbccddeeff",
		"skill_00112233445566778899aabbccddeeff",
		"example-skill",
		"sha256:0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef",
		time.Now().UTC(),
	)
	second := testInstallation(
		t,
		"sinst_11112222333344445555666677778888",
		first.SkillID().String(),
		"example-skill",
		first.Digest(),
		time.Now().UTC().Add(time.Minute),
	)
	if err := store.Create(ctx, first); err != nil {
		t.Fatal(err)
	}
	if err := store.Create(ctx, second); !errors.Is(err, skillapplication.ErrInstallationConflict) {
		t.Fatalf("Create(duplicate) error = %v", err)
	}
}

func testInstallation(
	t *testing.T,
	rawID string,
	rawSkillID string,
	name string,
	digest string,
	installedAt time.Time,
) skilldomain.Installation {
	t.Helper()
	id, err := skilldomain.ParseInstallationID(rawID)
	if err != nil {
		t.Fatal(err)
	}
	skillID, err := skilldomain.ParseSkillID(rawSkillID)
	if err != nil {
		t.Fatal(err)
	}
	item, err := skilldomain.NewInstallation(
		id,
		skillID,
		name,
		"SQLite Skill test.",
		skilldomain.SourceLocalDirectory,
		"/tmp/"+name,
		digest,
		"1.0.0",
		false,
		false,
		installedAt,
	)
	if err != nil {
		t.Fatal(err)
	}
	return item
}

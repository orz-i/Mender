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

func TestDeploymentStoreRoundTrip(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	store, err := Open(ctx, filepath.Join(t.TempDir(), "mender.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = store.Close() })

	root := t.TempDir()
	first := testDeployment(
		t,
		"sdep_00112233445566778899aabbccddeeff",
		"sinst_00112233445566778899aabbccddeeff",
		skilldomain.DeploymentAgentsProject,
		root,
		filepath.Join(root, ".agents", "skills", "example-skill"),
		"sha256:0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef",
		time.Date(2026, 9, 28, 12, 0, 0, 0, time.UTC),
	)
	second := testDeployment(
		t,
		"sdep_11112222333344445555666677778888",
		first.InstallationID().String(),
		skilldomain.DeploymentGitHubProject,
		root,
		filepath.Join(root, ".github", "skills", "example-skill"),
		first.Digest(),
		time.Date(2026, 9, 28, 13, 0, 0, 0, time.UTC),
	)

	if err := store.CreateDeployment(ctx, first); err != nil {
		t.Fatalf("CreateDeployment(first) error = %v", err)
	}
	if err := store.CreateDeployment(ctx, second); err != nil {
		t.Fatalf("CreateDeployment(second) error = %v", err)
	}

	got, err := store.GetDeployment(ctx, first.ID())
	if err != nil {
		t.Fatalf("GetDeployment() error = %v", err)
	}
	if got.ID() != first.ID() ||
		got.InstallationID() != first.InstallationID() ||
		got.Target() != first.Target() ||
		got.Destination() != first.Destination() ||
		got.Digest() != first.Digest() {
		t.Fatalf("GetDeployment() = %+v", got)
	}

	items, err := store.ListDeployments(ctx)
	if err != nil {
		t.Fatalf("ListDeployments() error = %v", err)
	}
	if len(items) != 2 {
		t.Fatalf("ListDeployments() count = %d, want 2", len(items))
	}

	byInstallation, err := store.ListDeploymentsByInstallation(ctx, first.InstallationID())
	if err != nil {
		t.Fatalf("ListDeploymentsByInstallation() error = %v", err)
	}
	if len(byInstallation) != 2 {
		t.Fatalf("ListDeploymentsByInstallation() count = %d, want 2", len(byInstallation))
	}

	if err := store.DeleteDeployment(ctx, first.ID()); err != nil {
		t.Fatalf("DeleteDeployment() error = %v", err)
	}
	if _, err := store.GetDeployment(ctx, first.ID()); !errors.Is(err, skillapplication.ErrDeploymentNotFound) {
		t.Fatalf("GetDeployment(deleted) error = %v", err)
	}
}

func TestDeploymentStoreRejectsDuplicateDestination(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	store, err := Open(ctx, filepath.Join(t.TempDir(), "mender.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = store.Close() })

	root := t.TempDir()
	first := testDeployment(
		t,
		"sdep_00112233445566778899aabbccddeeff",
		"sinst_00112233445566778899aabbccddeeff",
		skilldomain.DeploymentAgentsProject,
		root,
		filepath.Join(root, ".agents", "skills", "example-skill"),
		"sha256:0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef",
		time.Now().UTC(),
	)
	second := testDeployment(
		t,
		"sdep_11112222333344445555666677778888",
		"sinst_11112222333344445555666677778888",
		skilldomain.DeploymentAgentsProject,
		root,
		first.Destination(),
		"sha256:abcdef0123456789abcdef0123456789abcdef0123456789abcdef0123456789",
		time.Now().UTC().Add(time.Minute),
	)
	if err := store.CreateDeployment(ctx, first); err != nil {
		t.Fatal(err)
	}
	if err := store.CreateDeployment(ctx, second); !errors.Is(err, skillapplication.ErrDeploymentConflict) {
		t.Fatalf("CreateDeployment(duplicate destination) error = %v", err)
	}
}

func testDeployment(
	t *testing.T,
	rawID string,
	rawInstallationID string,
	target skilldomain.DeploymentTarget,
	root string,
	destination string,
	digest string,
	deployedAt time.Time,
) skilldomain.Deployment {
	t.Helper()
	id, err := skilldomain.ParseDeploymentID(rawID)
	if err != nil {
		t.Fatal(err)
	}
	installationID, err := skilldomain.ParseInstallationID(rawInstallationID)
	if err != nil {
		t.Fatal(err)
	}
	item, err := skilldomain.NewDeployment(
		id,
		installationID,
		target,
		root,
		destination,
		digest,
		deployedAt,
	)
	if err != nil {
		t.Fatal(err)
	}
	return item
}

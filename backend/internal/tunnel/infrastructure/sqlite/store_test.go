package sqlite

import (
	"context"
	"errors"
	"path/filepath"
	"testing"

	credentialdomain "github.com/orz-i/Mender/backend/internal/credential/domain"
	tunnelapplication "github.com/orz-i/Mender/backend/internal/tunnel/application"
	tunneldomain "github.com/orz-i/Mender/backend/internal/tunnel/domain"
)

func TestTunnelRoundTripConflictAndDelete(t *testing.T) {
	t.Parallel()

	store, err := Open(context.Background(), filepath.Join(t.TempDir(), "mender.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = store.Close() })

	firstID, _ := tunneldomain.ParseTunnelID("tun_00000000000000000000000000000001")
	secondID, _ := tunneldomain.ParseTunnelID("tun_00000000000000000000000000000002")
	ref, _ := credentialdomain.NewReference("openai", "tunnel-runtime")
	first, err := tunneldomain.NewTunnel(
		firstID,
		"OpenAI Work",
		tunneldomain.ProviderOpenAI,
		tunneldomain.OpenAIBinding(),
		"tunnel_external_1",
		ref,
	)
	if err != nil {
		t.Fatal(err)
	}
	if err := store.Create(context.Background(), first); err != nil {
		t.Fatal(err)
	}

	loaded, err := store.Get(context.Background(), firstID)
	if err != nil {
		t.Fatal(err)
	}
	if loaded.ID() != first.ID() ||
		loaded.DisplayName() != first.DisplayName() ||
		loaded.Provider() != first.Provider() ||
		loaded.ExternalTunnelID() != first.ExternalTunnelID() ||
		loaded.RuntimeCredentialRef() != first.RuntimeCredentialRef() {
		t.Fatalf("loaded tunnel = %#v", loaded)
	}

	conflict, _ := tunneldomain.NewTunnel(
		secondID,
		"Duplicate OpenAI Work",
		tunneldomain.ProviderOpenAI,
		tunneldomain.OpenAIBinding(),
		first.ExternalTunnelID(),
		ref,
	)
	if err := store.Create(context.Background(), conflict); !errors.Is(
		err,
		tunnelapplication.ErrTunnelConflict,
	) {
		t.Fatalf("Create(conflict) error = %v", err)
	}

	items, err := store.List(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(items) != 1 || items[0].ID() != firstID {
		t.Fatalf("List() = %#v", items)
	}

	if err := store.Delete(context.Background(), firstID); err != nil {
		t.Fatal(err)
	}
	if _, err := store.Get(context.Background(), firstID); !errors.Is(
		err,
		tunnelapplication.ErrTunnelNotFound,
	) {
		t.Fatalf("Get(deleted) error = %v", err)
	}
}

func TestOpenRejectsFutureTunnelSchemaVersion(t *testing.T) {
	t.Parallel()

	path := filepath.Join(t.TempDir(), "mender.db")
	store, err := Open(context.Background(), path)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := store.db.Exec(
		"INSERT INTO tunnel_schema_migrations(version, applied_at) VALUES (99, 'future')",
	); err != nil {
		t.Fatal(err)
	}
	if err := store.Close(); err != nil {
		t.Fatal(err)
	}

	reopened, err := Open(context.Background(), path)
	if reopened != nil {
		_ = reopened.Close()
	}
	if err == nil {
		t.Fatal("Open() error = nil, want future schema rejection")
	}
}

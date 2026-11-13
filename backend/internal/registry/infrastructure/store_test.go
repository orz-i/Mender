package infrastructure

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/orz-i/Mender/backend/internal/registry/domain"
)

func TestIndexStoreSaveAndRetrieve(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	store, err := OpenIndexStore(ctx, ":memory:")
	if err != nil {
		t.Fatalf("OpenIndexStore() error = %v", err)
	}
	defer func() { _ = store.Close() }()

	// Initial sequence should be 0
	seq, err := store.GetLatestSequence(ctx, "official")
	if err != nil {
		t.Fatalf("GetLatestSequence() error = %v", err)
	}
	if seq != 0 {
		t.Fatalf("initial sequence = %d, want 0", seq)
	}

	now := time.Date(2026, 11, 12, 12, 0, 0, 0, time.UTC)
	envelope := domain.SignatureEnvelope{
		Payload: domain.CatalogIndex{
			SchemaVersion: 1,
			Sequence:      42,
			IssuedAt:      now,
			ExpiresAt:     now.Add(24 * time.Hour),
			Plugins: []domain.RegistryPluginEntry{
				{
					PluginID:    "dev.example.echo",
					Version:     "1.0.0",
					DisplayName: "Echo Plugin",
					Description: "Echo",
					Category:    "developer-tools",
					Targets: map[string]domain.ArtifactTarget{
						"linux/amd64": {
							URL:        "https://example.com/echo.tar.gz",
							SHA256:     strings.Repeat("a", 64),
							TreeDigest: "sha256:" + strings.Repeat("1", 64),
							Size:       1024,
						},
					},
				},
				{
					PluginID:    "dev.example.other",
					Version:     "2.1.0",
					DisplayName: "Other Plugin",
					Targets: map[string]domain.ArtifactTarget{
						"windows/amd64": {
							URL:        "https://example.com/other.tar.gz",
							SHA256:     strings.Repeat("b", 64),
							TreeDigest: "sha256:" + strings.Repeat("2", 64),
							Size:       2048,
						},
					},
				},
			},
		},
		Signatures: []domain.Signature{
			{KeyID: "official-root", Sig: strings.Repeat("c", 128)},
		},
	}

	if err := store.SaveEnvelope(ctx, "official", envelope); err != nil {
		t.Fatalf("SaveEnvelope() error = %v", err)
	}

	// Check latest sequence
	seq, err = store.GetLatestSequence(ctx, "official")
	if err != nil {
		t.Fatalf("GetLatestSequence() error = %v", err)
	}
	if seq != 42 {
		t.Fatalf("got sequence %d, want 42", seq)
	}

	// Retrieve envelope
	loaded, err := store.GetEnvelope(ctx, "official")
	if err != nil {
		t.Fatalf("GetEnvelope() error = %v", err)
	}
	if loaded.Payload.Sequence != 42 {
		t.Fatalf("loaded sequence = %d, want 42", loaded.Payload.Sequence)
	}
	if len(loaded.Payload.Plugins) != 2 {
		t.Fatalf("loaded %d plugins, want 2", len(loaded.Payload.Plugins))
	}

	// List plugins
	plugins, err := store.ListPlugins(ctx, "official")
	if err != nil {
		t.Fatalf("ListPlugins() error = %v", err)
	}
	if len(plugins) != 2 {
		t.Fatalf("ListPlugins() returned %d plugins, want 2", len(plugins))
	}

	// Get specific plugin
	plugin, err := store.GetPlugin(ctx, "official", "dev.example.echo")
	if err != nil {
		t.Fatalf("GetPlugin() error = %v", err)
	}
	if plugin.DisplayName != "Echo Plugin" {
		t.Fatalf("got display name %q, want 'Echo Plugin'", plugin.DisplayName)
	}

	// Get specific version
	versioned, err := store.GetPluginVersion(ctx, "official", "dev.example.echo", "1.0.0")
	if err != nil {
		t.Fatalf("GetPluginVersion() error = %v", err)
	}
	if versioned.Version != "1.0.0" {
		t.Fatalf("got version %q, want '1.0.0'", versioned.Version)
	}

	// Missing plugin
	if _, err := store.GetPlugin(ctx, "official", "nonexistent"); err == nil {
		t.Fatal("expected error for nonexistent plugin")
	}
}

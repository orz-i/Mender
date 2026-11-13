package domain

import (
	"errors"
	"strings"
	"testing"
	"time"
)

func sampleIndex() CatalogIndex {
	now := time.Date(2026, 11, 12, 12, 0, 0, 0, time.UTC)
	return CatalogIndex{
		SchemaVersion: 1,
		Sequence:      10,
		IssuedAt:      now.Add(-1 * time.Hour),
		ExpiresAt:     now.Add(24 * time.Hour),
		Plugins: []RegistryPluginEntry{
			{
				PluginID:    "dev.example.echo",
				Version:     "1.0.0",
				DisplayName: "Echo Plugin",
				Description: "Echoes messages",
				Targets: map[string]ArtifactTarget{
					"windows/amd64": {
						URL:        "https://registry.mender.io/plugins/echo-windows-amd64.tar.gz",
						SHA256:     "sha256:" + strings.Repeat("a", 64),
						TreeDigest: "sha256:" + strings.Repeat("1", 64),
						Size:       1024,
					},
					"linux/amd64": {
						URL:        "https://registry.mender.io/plugins/echo-linux-amd64.tar.gz",
						SHA256:     strings.Repeat("b", 64),
						TreeDigest: "sha256:" + strings.Repeat("2", 64),
						Size:       2048,
					},
				},
			},
		},
	}
}

func TestAntiFreezeValidation(t *testing.T) {
	t.Parallel()

	now := time.Date(2026, 11, 12, 12, 0, 0, 0, time.UTC)
	idx := sampleIndex()

	// Valid freshness: issuedAt <= now <= expiresAt
	if err := idx.ValidateFreshness(now); err != nil {
		t.Fatalf("ValidateFreshness(now) unexpected error: %v", err)
	}

	// Future issuedAt (now < issuedAt)
	future := idx
	future.IssuedAt = now.Add(1 * time.Hour)
	if err := future.ValidateFreshness(now); !errors.Is(err, ErrAntiFreeze) {
		t.Fatalf("expected ErrAntiFreeze for future index, got: %v", err)
	}

	// Expired (now > expiresAt)
	expired := idx
	expired.ExpiresAt = now.Add(-1 * time.Minute)
	if err := expired.ValidateFreshness(now); !errors.Is(err, ErrAntiFreeze) {
		t.Fatalf("expected ErrAntiFreeze for expired index, got: %v", err)
	}
}

func TestAntiRollbackValidation(t *testing.T) {
	t.Parallel()

	idx := sampleIndex()
	idx.Sequence = 15

	// New sequence > lastSequence -> ok
	if err := idx.ValidateSequence(10); err != nil {
		t.Fatalf("ValidateSequence(10) error = %v", err)
	}

	// Same sequence -> ok
	if err := idx.ValidateSequence(15); err != nil {
		t.Fatalf("ValidateSequence(15) error = %v", err)
	}

	// Rollback attempt (new sequence < lastSequence) -> error
	if err := idx.ValidateSequence(20); !errors.Is(err, ErrAntiRollback) {
		t.Fatalf("expected ErrAntiRollback for sequence 15 < 20, got: %v", err)
	}
}

func TestTargetPlatformLookup(t *testing.T) {
	t.Parallel()

	idx := sampleIndex()
	plugin, found := idx.FindPlugin("dev.example.echo")
	if !found {
		t.Fatal("FindPlugin() failed to find dev.example.echo")
	}

	winTarget, err := plugin.Target("windows", "amd64")
	if err != nil {
		t.Fatalf("Target(windows, amd64) error = %v", err)
	}
	if winTarget.SHA256Hex() != strings.Repeat("a", 64) {
		t.Fatalf("SHA256Hex() = %q, want %q", winTarget.SHA256Hex(), strings.Repeat("a", 64))
	}

	linuxTarget, err := plugin.Target("linux", "amd64")
	if err != nil {
		t.Fatalf("Target(linux, amd64) error = %v", err)
	}
	if linuxTarget.SHA256Hex() != strings.Repeat("b", 64) {
		t.Fatalf("SHA256Hex() = %q, want %q", linuxTarget.SHA256Hex(), strings.Repeat("b", 64))
	}

	// Unsupported platform
	if _, err := plugin.Target("darwin", "arm64"); !errors.Is(err, ErrTargetNotFound) {
		t.Fatalf("expected ErrTargetNotFound, got: %v", err)
	}
}

func TestIndexValidation(t *testing.T) {
	t.Parallel()

	idx := sampleIndex()
	if err := idx.Validate(); err != nil {
		t.Fatalf("valid index failed validation: %v", err)
	}

	// Negative sequence
	badSeq := idx
	badSeq.Sequence = -1
	if err := badSeq.Validate(); !errors.Is(err, ErrInvalidIndex) {
		t.Fatalf("expected ErrInvalidIndex for negative sequence, got: %v", err)
	}

	// Invalid semver
	badVer := idx
	badVer.Plugins[0].Version = "not-semver"
	if err := badVer.Validate(); !errors.Is(err, ErrInvalidIndex) {
		t.Fatalf("expected ErrInvalidIndex for bad semver, got: %v", err)
	}
}

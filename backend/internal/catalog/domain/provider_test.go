package domain

import (
	"testing"
	"time"
)

func TestProviderUpdateMetadataPreservesIdentity(t *testing.T) {
	t.Parallel()

	id, _ := ParseProviderID("prov_0123456789abcdef0123456789abcdef")
	namespace, _ := NewNamespace("github")
	created := time.Date(2026, 9, 25, 12, 0, 0, 0, time.UTC)

	provider, err := NewProvider(id, namespace, "GitHub", "Old", created)
	if err != nil {
		t.Fatal(err)
	}

	updatedAt := created.Add(time.Hour)
	updated, err := provider.UpdateMetadata("GitHub Work", "Updated", updatedAt)
	if err != nil {
		t.Fatalf("UpdateMetadata() error = %v", err)
	}

	if updated.ID() != id || updated.Namespace() != namespace {
		t.Fatalf("identity changed: %#v", updated)
	}
	if updated.CreatedAt() != created || updated.UpdatedAt() != updatedAt {
		t.Fatalf("timestamps = created %v updated %v", updated.CreatedAt(), updated.UpdatedAt())
	}
	if updated.DisplayName() != "GitHub Work" || updated.Description() != "Updated" {
		t.Fatalf("metadata = %q %q", updated.DisplayName(), updated.Description())
	}
}

func TestProviderIdentityIsIndependentFromDisplayName(t *testing.T) {
	t.Parallel()

	id, _ := ParseProviderID("prov_0123456789abcdef0123456789abcdef")
	namespace, _ := NewNamespace("github")
	now := time.Date(2026, 9, 25, 12, 0, 0, 0, time.UTC)

	provider, err := NewProvider(id, namespace, "GitHub Production", "Primary provider", now)
	if err != nil {
		t.Fatalf("NewProvider() error = %v", err)
	}

	if provider.ID() != id {
		t.Fatalf("Provider ID = %q, want %q", provider.ID(), id)
	}
	if provider.Namespace() != namespace {
		t.Fatalf("Provider namespace = %q, want %q", provider.Namespace(), namespace)
	}
	if provider.DisplayName() != "GitHub Production" {
		t.Fatalf("DisplayName() = %q", provider.DisplayName())
	}
}

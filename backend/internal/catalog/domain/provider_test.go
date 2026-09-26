package domain

import (
	"testing"
	"time"
)

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

package binding

import (
	"context"
	"path/filepath"
	"testing"
)

func TestService_RevealPath(t *testing.T) {
	service := &Service{ctx: context.Background()}

	// Empty path error
	if err := service.RevealPath(""); err == nil {
		t.Fatalf("expected error for empty path, got nil")
	}

	// Non-existent path error
	nonExistent := filepath.Join(t.TempDir(), "does-not-exist-dir")
	if err := service.RevealPath(nonExistent); err == nil {
		t.Fatalf("expected error for non-existent path, got nil")
	}
}

package appdata

import (
	"errors"
	"path/filepath"
	"testing"
)

func TestResolveDatabasePathUsesExplicitOverride(t *testing.T) {
	root := t.TempDir()
	explicit := filepath.Join(root, "custom", "mender.db")

	got, err := resolveDatabasePath(explicit, func() (string, error) {
		return "", errors.New("should not be called")
	})
	if err != nil {
		t.Fatalf("resolve explicit path: %v", err)
	}
	want, err := filepath.Abs(explicit)
	if err != nil {
		t.Fatalf("absolute path: %v", err)
	}
	if got != want {
		t.Fatalf("path = %q, want %q", got, want)
	}
}

func TestResolveDatabasePathUsesMenderConfigDirectory(t *testing.T) {
	base := t.TempDir()

	got, err := resolveDatabasePath("", func() (string, error) {
		return base, nil
	})
	if err != nil {
		t.Fatalf("resolve default path: %v", err)
	}
	want := filepath.Join(base, directoryName, "mender.db")
	if got != want {
		t.Fatalf("path = %q, want %q", got, want)
	}
}

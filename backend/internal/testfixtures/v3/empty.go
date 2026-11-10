// Package v3 provides a single, disposable, pinned Runtime v3 database fixture.
// It calls the same hash-checked schema initializer used by the production
// bootstrap. It never imports or creates historical Provider facades.
package v3

import (
	"path/filepath"
	"testing"

	schema "github.com/orz-i/Mender/backend/internal/schema/v3"
)

func EmptyDatabase(t testing.TB) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "v3.sqlite")
	Prepare(t, path)
	return path
}

func Prepare(t testing.TB, path string) {
	t.Helper()
	if err := schema.EnsureFreshDatabase(path); err != nil {
		t.Fatalf("prepare pinned v3 fixture: %v", err)
	}
}

package sqlite

import (
	"context"
	"testing"

	fixtures "github.com/orz-i/Mender/backend/internal/testfixtures/v3"
)

// openAgentV3 creates the real hash-pinned Runtime schema in a disposable
// test-owned file. It is NOT a legacy migration or an alternate production
// constructor; an existing file is never overwritten.
func openAgentV3(t *testing.T, ctx context.Context, path string) (*Store, error) {
	t.Helper()
	fixtures.Prepare(t, path)
	return OpenTyped(ctx, path)
}

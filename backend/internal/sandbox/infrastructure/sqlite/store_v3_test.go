package sqlite

import (
	"context"
	"testing"

	fixtures "github.com/orz-i/Mender/backend/internal/testfixtures/v3"
)

func openSandboxV3(t *testing.T, ctx context.Context, path string) (*Store, error) {
	t.Helper()
	fixtures.Prepare(t, path)
	return OpenTyped(ctx, path)
}

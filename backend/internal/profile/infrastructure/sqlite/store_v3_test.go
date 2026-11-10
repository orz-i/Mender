package sqlite

import (
	"context"
	"testing"

	v3fixture "github.com/orz-i/Mender/backend/internal/testfixtures/v3"
)

func openProfileV3(t *testing.T, ctx context.Context, path string) (*Store, error) {
	t.Helper()
	v3fixture.Prepare(t, path)
	return OpenTyped(ctx, path)
}

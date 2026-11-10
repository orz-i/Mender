package daytona

import (
	"context"
	"testing"

	sandboxsqlite "github.com/orz-i/Mender/backend/internal/sandbox/infrastructure/sqlite"
	v3fixture "github.com/orz-i/Mender/backend/internal/testfixtures/v3"
)

func openSandboxV3(t *testing.T, ctx context.Context, path string) (*sandboxsqlite.Store, error) {
	t.Helper()
	v3fixture.Prepare(t, path)
	return sandboxsqlite.OpenTyped(ctx, path)
}

package delegationprovider

import (
	"context"
	"testing"

	agentsqlite "github.com/orz-i/Mender/backend/internal/agent/infrastructure/sqlite"
	v3test "github.com/orz-i/Mender/backend/internal/testfixtures/v3"
)

func openAgentV3(t *testing.T, ctx context.Context, path string) (*agentsqlite.Store, error) {
	t.Helper()
	v3test.Prepare(t, path)
	return agentsqlite.OpenTyped(ctx, path)
}

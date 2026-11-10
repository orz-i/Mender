package application

import (
	"context"
	"testing"

	policysqlite "github.com/orz-i/Mender/backend/internal/policy/infrastructure/sqlite"
	v3test "github.com/orz-i/Mender/backend/internal/testfixtures/v3"
)

// Bare application fixtures must use real Policy v3 storage: positive ACP
// permissions must never acquire a hidden no-fence test fallback.
func requireApprovalFixturePolicy(t *testing.T, path string) {
	t.Helper()
	v3test.Prepare(t, path)
	p, err := policysqlite.OpenTyped(context.Background(), path)
	if err != nil {
		t.Fatalf("initialize fixture Policy v3: %v", err)
	}
	t.Cleanup(func() { _ = p.Close() })
}

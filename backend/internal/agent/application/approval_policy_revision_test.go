package application

import (
	"context"
	"testing"

	policysqlite "github.com/orz-i/Mender/backend/internal/policy/infrastructure/sqlite"
)

// Bare application fixtures must use real Policy v3 storage: positive ACP
// permissions must never acquire a hidden no-fence test fallback.
func requireApprovalFixturePolicy(t *testing.T, path string) {
	t.Helper()
	p, err := policysqlite.Open(context.Background(), path)
	if err != nil {
		t.Fatalf("initialize fixture Policy v3: %v", err)
	}
	t.Cleanup(func() { _ = p.Close() })
}

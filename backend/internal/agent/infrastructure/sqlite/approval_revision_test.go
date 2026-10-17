package sqlite

import (
	"context"
	"testing"

	policysqlite "github.com/orz-i/Mender/backend/internal/policy/infrastructure/sqlite"
)

func setupApprovalPolicyFenceFixture(t *testing.T, path string) {
	t.Helper()
	p, err := policysqlite.Open(context.Background(), path)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = p.Close() })
}

package sqlite

import (
	"context"
	"testing"

	policysqlite "github.com/orz-i/Mender/backend/internal/policy/infrastructure/sqlite"
	v3test "github.com/orz-i/Mender/backend/internal/testfixtures/v3"
)

func setupApprovalPolicyFenceFixture(t *testing.T, path string) {
	t.Helper()
	v3test.Prepare(t, path)
	p, err := policysqlite.OpenTyped(context.Background(), path)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = p.Close() })
}

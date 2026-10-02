package local

import (
	"context"
	"testing"

	governanceapplication "github.com/orz-i/Mender/backend/internal/governance/application"
)

func TestRejectApprovalFailsClosed(t *testing.T) {
	t.Parallel()

	approved, err := (RejectApproval{}).Approve(
		context.Background(),
		governanceapplication.ApprovalRequest{},
	)
	if err != nil {
		t.Fatal(err)
	}
	if approved {
		t.Fatal("RejectApproval approved unexpectedly")
	}
}

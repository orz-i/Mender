package local

import (
	"context"
	"testing"

	invocationapplication "github.com/orz-i/Mender/backend/internal/invocation/application"
)

func TestRejectApprovalFailsClosed(t *testing.T) {
	t.Parallel()

	approved, err := (RejectApproval{}).Approve(
		context.Background(),
		invocationapplication.ApprovalRequest{},
	)
	if err != nil {
		t.Fatal(err)
	}
	if approved {
		t.Fatal("RejectApproval approved unexpectedly")
	}
}

// Package local contains local-only Policy boundary adapters.
package local

import (
	"context"

	invocationapplication "github.com/orz-i/Mender/backend/internal/invocation/application"
)

// RejectApproval is a fail-closed gate when no interactive approval adapter is configured.
type RejectApproval struct{}

func (RejectApproval) Approve(
	context.Context,
	invocationapplication.ApprovalRequest,
) (bool, error) {
	return false, nil
}

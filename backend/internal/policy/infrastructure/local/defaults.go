// Package local contains local-only Policy boundary adapters.
package local

import (
	"context"

	governanceapplication "github.com/orz-i/Mender/backend/internal/governance/application"
)

// RejectApproval is a fail-closed gate when no interactive approval adapter is configured.
type RejectApproval struct{}

func (RejectApproval) Approve(
	context.Context,
	governanceapplication.ApprovalRequest,
) (bool, error) {
	return false, nil
}

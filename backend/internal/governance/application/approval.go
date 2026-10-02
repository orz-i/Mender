package application

import (
	"context"

	catalogdomain "github.com/orz-i/Mender/backend/internal/catalog/domain"
	governancedomain "github.com/orz-i/Mender/backend/internal/governance/domain"
	policydomain "github.com/orz-i/Mender/backend/internal/policy/domain"
)

// ApprovalRequest is the bounded approval context for one governed action.
type ApprovalRequest struct {
	OperationID string
	Action      governancedomain.Action
	Context     governancedomain.Context
	ProviderID  catalogdomain.ProviderID
	ToolID      *catalogdomain.ToolID
	Decision    policydomain.Decision
}

// ApprovalGate resolves Policy decisions that require explicit approval.
type ApprovalGate interface {
	Approve(context.Context, ApprovalRequest) (bool, error)
}

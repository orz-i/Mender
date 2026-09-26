// Package local contains safe local Policy defaults used before configurable
// policy rules are introduced.
package local

import (
	"context"

	invocationapplication "github.com/orz-i/Mender/backend/internal/invocation/application"
	policydomain "github.com/orz-i/Mender/backend/internal/policy/domain"
)

// AllowAll permits execution with a stable reason code.
//
// This is an explicit development/local default, not a substitute for future
// profile-aware policy configuration.
type AllowAll struct{}

func (AllowAll) Evaluate(
	context.Context,
	invocationapplication.PolicyRequest,
) (policydomain.Decision, error) {
	return policydomain.Allow("local.default", "local default policy")
}

// RejectApproval is a fail-closed gate when no interactive approval adapter is configured.
type RejectApproval struct{}

func (RejectApproval) Approve(
	context.Context,
	invocationapplication.ApprovalRequest,
) (bool, error) {
	return false, nil
}

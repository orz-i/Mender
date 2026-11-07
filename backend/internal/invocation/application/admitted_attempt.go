package application

import (
	"context"
	"errors"
	"slices"

	catalog "github.com/orz-i/Mender/backend/internal/catalog/domain"
	governance "github.com/orz-i/Mender/backend/internal/governance/domain"
	invocation "github.com/orz-i/Mender/backend/internal/invocation/domain"
)

// An admitted attempt is represented only inside the existing Invocation
// execution context. No extra approval store, task lifecycle or public receipt
// is created; external Tool arguments cannot construct this private key.
type admittedAttemptKey struct{}
type admittedAttempt struct {
	id     invocation.InvocationID
	caller governance.Context
	toolID catalog.ToolID
	facts  invocation.GovernanceFacts
}

var errInvocationNotAdmitted = errors.New("invocation admission proof unavailable")

// withAdmittedAttempt is called only after Tool Policy/Approval and Runtime
// Budget/Binding gates succeed in the canonical InvokeTool execution stage.
func withAdmittedAttempt(ctx context.Context, request ExecutionRequest) context.Context {
	return context.WithValue(ctx, admittedAttemptKey{}, admittedAttempt{
		id: request.InvocationID, caller: request.Context,
		toolID: request.Tool.ID(), facts: request.AdmittedFacts,
	})
}

// ApprovedAttemptVerifier consumes the private, attempt-scoped proof issued
// by the existing Invocation path. It does not mint an approval from a ToolID
// or any model- or plugin-supplied identifier.
type ApprovedAttemptVerifier struct{}

func (ApprovedAttemptVerifier) VerifyAdmittedAttempt(ctx context.Context,
	id invocation.InvocationID, caller governance.Context, toolID catalog.ToolID,
	facts invocation.GovernanceFacts,
) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	current, ok := ctx.Value(admittedAttemptKey{}).(admittedAttempt)
	if !ok || id != current.id || caller != current.caller || toolID != current.toolID ||
		!sameApprovedFacts(current.facts, facts) {
		return errInvocationNotAdmitted
	}
	return nil
}

func sameApprovedFacts(a, b invocation.GovernanceFacts) bool {
	if a.Validate() != nil || b.Validate() != nil ||
		a.Knowledge() != invocation.FactKnowledgeKnown ||
		b.Knowledge() != invocation.FactKnowledgeKnown {
		return false
	}
	return slices.Equal(a.Effects(), b.Effects()) &&
		slices.Equal(a.Resources(), b.Resources())
}

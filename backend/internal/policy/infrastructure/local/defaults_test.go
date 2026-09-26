package local

import (
	"context"
	"testing"

	invocationapplication "github.com/orz-i/Mender/backend/internal/invocation/application"
	policydomain "github.com/orz-i/Mender/backend/internal/policy/domain"
)

func TestDefaultsAreExplicitAndFailClosedForApproval(t *testing.T) {
	t.Parallel()

	decision, err := (AllowAll{}).Evaluate(
		context.Background(),
		invocationapplication.PolicyRequest{},
	)
	if err != nil {
		t.Fatal(err)
	}
	if decision.Kind() != policydomain.DecisionAllow || decision.ReasonCode() != "local.default" {
		t.Fatalf("decision = %#v", decision)
	}

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

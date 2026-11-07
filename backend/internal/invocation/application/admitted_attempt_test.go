package application

import (
	"context"
	"errors"
	"testing"

	catalog "github.com/orz-i/Mender/backend/internal/catalog/domain"
	invocation "github.com/orz-i/Mender/backend/internal/invocation/domain"
	policy "github.com/orz-i/Mender/backend/internal/policy/domain"
)

func TestCanonicalInvocationMintsExactExecutionOnlyAdmission(t *testing.T) {
	ports, useCase, request := tracedInvocation(t)
	resource, err := invocation.NewResourceFact("workspace_id", "wrk_0123456789abcdef0123456789abcdef")
	if err != nil {
		t.Fatal(err)
	}
	ports.facts, err = invocation.KnownGovernanceFacts([]invocation.EffectFact{"filesystem.read"}, []invocation.ResourceFact{resource})
	if err != nil {
		t.Fatal(err)
	}
	calls := 0
	ports.onExecuteCtx = func(ctx context.Context, execution ExecutionRequest) {
		calls++
		verifier := ApprovedAttemptVerifier{}
		if err := verifier.VerifyAdmittedAttempt(ctx, execution.InvocationID,
			execution.Context, execution.Tool.ID(), execution.AdmittedFacts); err != nil {
			t.Errorf("canonical execution did not carry its admission: %v", err)
		}
		if err := verifier.VerifyAdmittedAttempt(ctx, execution.InvocationID,
			execution.Context, catalog.ToolID("tool_11111111111111111111111111111111"),
			execution.AdmittedFacts); !errors.Is(err, errInvocationNotAdmitted) {
			t.Errorf("a different ToolID inherited admission: %v", err)
		}
		if err := verifier.VerifyAdmittedAttempt(ctx, invocation.InvocationID("inv_aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"),
			execution.Context, execution.Tool.ID(), execution.AdmittedFacts); !errors.Is(err, errInvocationNotAdmitted) {
			t.Errorf("another InvocationID inherited admission: %v", err)
		}
		if err := verifier.VerifyAdmittedAttempt(ctx, execution.InvocationID,
			execution.Context, execution.Tool.ID(), invocation.UnknownGovernanceFacts()); !errors.Is(err, errInvocationNotAdmitted) {
			t.Errorf("unknown policy facts reused admission: %v", err)
		}
	}
	result, err := useCase.Execute(context.Background(), request)
	if err != nil || result.Outcome.Kind() != invocation.OutcomeKindComplete || calls != 1 {
		t.Fatalf("canonical execution admission: result=%+v err=%v calls=%d", result, err, calls)
	}
}

func TestNoNativeAdmissionCanBeMintedBeforePolicyOrRuntimeCapacity(t *testing.T) {
	for _, tc := range []string{"policy", "budget", "binding"} {
		t.Run(tc, func(t *testing.T) {
			ports, useCase, request := tracedInvocation(t)
			if tc == "policy" {
				ports.decision, _ = policy.Deny("test", "blocked")
			} else {
				ports.failAt = tc
			}
			ports.onExecuteCtx = func(context.Context, ExecutionRequest) {
				t.Fatal("unadmitted executor received a private receipt")
			}
			_, _ = useCase.Execute(context.Background(), request)
			if ports.executionRequest.InvocationID != "" {
				t.Fatal("rejected admission reached executor")
			}
			if err := (ApprovedAttemptVerifier{}).VerifyAdmittedAttempt(context.Background(),
				"inv_aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa", request.Context, request.ToolID,
				invocation.UnknownGovernanceFacts()); !errors.Is(err, errInvocationNotAdmitted) {
				t.Fatalf("ordinary context forged private receipt: %v", err)
			}
		})
	}
}

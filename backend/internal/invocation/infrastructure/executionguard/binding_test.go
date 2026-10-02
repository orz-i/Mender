package executionguard

import (
	"context"
	"testing"
	"time"

	governancedomain "github.com/orz-i/Mender/backend/internal/governance/domain"
	invocationapplication "github.com/orz-i/Mender/backend/internal/invocation/application"
	invocationdomain "github.com/orz-i/Mender/backend/internal/invocation/domain"
)

func TestProfileBindingSerialBlocksSameBindingOnly(t *testing.T) {
	t.Parallel()

	guard := NewProfileBindingSerial()
	first := bindingGuardRequest(t, 1, "prof_00000000000000000000000000000001")
	same := bindingGuardRequest(t, 1, "prof_00000000000000000000000000000001")
	otherTool := bindingGuardRequest(t, 2, "prof_00000000000000000000000000000001")
	otherProfile := bindingGuardRequest(t, 1, "prof_00000000000000000000000000000002")

	releaseFirst, err := guard.Acquire(context.Background(), first)
	if err != nil {
		t.Fatal(err)
	}
	defer releaseFirst()

	blockedCtx, cancel := context.WithTimeout(context.Background(), 30*time.Millisecond)
	defer cancel()
	if _, err := guard.Acquire(blockedCtx, same); err == nil {
		t.Fatal("same binding acquisition unexpectedly succeeded")
	}

	releaseOtherTool, err := guard.Acquire(context.Background(), otherTool)
	if err != nil {
		t.Fatalf("other Tool binding acquisition error = %v", err)
	}
	releaseOtherTool()

	releaseOtherProfile, err := guard.Acquire(context.Background(), otherProfile)
	if err != nil {
		t.Fatalf("other Profile binding acquisition error = %v", err)
	}
	releaseOtherProfile()
}

func TestProfileBindingSerialInheritDoesNotSerialize(t *testing.T) {
	t.Parallel()

	guard := NewProfileBindingSerial()
	request := bindingGuardRequestWithConstraint(
		t,
		1,
		"prof_00000000000000000000000000000001",
		invocationdomain.BindingConcurrencyInherit,
	)

	releaseFirst, err := guard.Acquire(context.Background(), request)
	if err != nil {
		t.Fatal(err)
	}
	defer releaseFirst()

	releaseSecond, err := guard.Acquire(context.Background(), request)
	if err != nil {
		t.Fatalf("inherit acquisition blocked: %v", err)
	}
	releaseSecond()
}

func TestProfileBindingSerialRequiresProfileScopeForSerial(t *testing.T) {
	t.Parallel()

	request := bindingGuardRequest(t, 1, "prof_00000000000000000000000000000001")
	principal, err := governancedomain.NewPrincipal(
		governancedomain.PrincipalService,
		"test:binding-guard",
	)
	if err != nil {
		t.Fatal(err)
	}
	catalogContext, err := governancedomain.NewContext(
		governancedomain.OriginInternal,
		principal,
		governancedomain.CatalogScope(),
	)
	if err != nil {
		t.Fatal(err)
	}
	request.Context = catalogContext

	if _, err := NewProfileBindingSerial().Acquire(context.Background(), request); err == nil {
		t.Fatal("serial binding guard accepted Catalog scope")
	}
}

func bindingGuardRequest(
	t *testing.T,
	toolNumber int,
	profileReference string,
) invocationapplication.BindingExecutionGuardRequest {
	t.Helper()
	return bindingGuardRequestWithConstraint(
		t,
		toolNumber,
		profileReference,
		invocationdomain.BindingConcurrencySerial,
	)
}

func bindingGuardRequestWithConstraint(
	t *testing.T,
	toolNumber int,
	profileReference string,
	constraint invocationdomain.BindingConcurrencyConstraint,
) invocationapplication.BindingExecutionGuardRequest {
	t.Helper()

	request := guardRequest(t, toolNumber, 1)
	scope, err := governancedomain.ProfileScope(profileReference)
	if err != nil {
		t.Fatal(err)
	}
	principal, err := governancedomain.NewPrincipal(
		governancedomain.PrincipalService,
		"test:binding-guard",
	)
	if err != nil {
		t.Fatal(err)
	}
	executionContext, err := governancedomain.NewContext(
		governancedomain.OriginMCP,
		principal,
		scope,
	)
	if err != nil {
		t.Fatal(err)
	}
	governance, err := invocationdomain.NewScopedBindingGovernance(
		invocationdomain.BindingPolicyInherit,
		constraint,
	)
	if err != nil {
		t.Fatal(err)
	}

	return invocationapplication.BindingExecutionGuardRequest{
		InvocationID: "",
		Tool:         request.Tool,
		Context:      executionContext,
		Governance:   governance,
	}
}

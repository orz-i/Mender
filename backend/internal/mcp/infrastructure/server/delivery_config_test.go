package server

import (
	"context"
	"errors"
	"testing"

	supply "github.com/orz-i/Mender/backend/internal/profile/infrastructure/typedsupply"
)

func TestDeliveryConfigurationCannotExpandProfileSelection(t *testing.T) {
	f := newTypedMCPFixture(t)
	// Direct tool missing from verified profile projection is rejected
	f.snapshot.Direct = []supply.Export{{ToolID: "tool_missing", Name: "missing", SourceID: "native:workspace"}}
	if _, err := NewTyped(context.Background(), typedProfileID, TypedDependencies{Profiles: f, DescribeTool: f.describe, InvokeTool: f.invoke}); !errors.Is(err, ErrToolOutsideProfile) {
		t.Fatalf("out-of-scope direct selection accepted: %v", err)
	}

	// Invalid Profile ID format is rejected
	if _, err := NewTyped(context.Background(), "invalid-profile-id", TypedDependencies{Profiles: f, DescribeTool: f.describe, InvokeTool: f.invoke}); err == nil {
		t.Fatalf("invalid profile ID accepted")
	}
}

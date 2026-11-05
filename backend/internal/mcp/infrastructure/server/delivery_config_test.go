package server

import (
	"context"
	"strings"
	"testing"

	catalogdomain "github.com/orz-i/Mender/backend/internal/catalog/domain"
	invocationapp "github.com/orz-i/Mender/backend/internal/invocation/application"
	mcpdomain "github.com/orz-i/Mender/backend/internal/mcp/domain"
	profiledomain "github.com/orz-i/Mender/backend/internal/profile/domain"
)

func TestDeliveryConfigurationCannotExpandProfileSelection(t *testing.T) {
	fixture := newServerFixture(t)
	profile := fixture.profile(t, mcpdomain.ModeHybrid, []profiledomain.ToolBindingSpec{{ToolID: fixture.tools[0].ID()}}, fixture.tools[2].ID())
	deps := Dependencies{
		DescribeTool: func(_ context.Context, id catalogdomain.ToolID) (catalogdomain.Tool, error) {
			return fixture.tools[0], nil
		},
		GetProvider: func(context.Context, catalogdomain.ProviderID) (catalogdomain.Provider, error) {
			return fixture.provider, nil
		},
		InvokeTool: func(context.Context, invocationapp.InvokeToolRequest) (invocationapp.InvokeToolResponse, error) {
			t.Fatal("configuration check invoked Tool")
			return invocationapp.InvokeToolResponse{}, nil
		},
	}
	if _, err := New(context.Background(), profile.Profile, profile.Config, deps); err == nil || !strings.Contains(err.Error(), "missing from profile projection") {
		t.Fatalf("out-of-scope direct selection accepted: %v", err)
	}
	otherID, _ := profiledomain.ParseProfileID("prof_00000000000000000000000000000002")
	otherConfig, err := mcpdomain.NewConfig(otherID, mcpdomain.ModeDynamic, "", nil)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := New(context.Background(), profile.Profile, otherConfig, deps); err == nil || !strings.Contains(err.Error(), "identity mismatch") {
		t.Fatalf("another Profile's delivery accepted: %v", err)
	}
}

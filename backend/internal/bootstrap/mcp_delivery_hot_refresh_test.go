package bootstrap

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	catalogdomain "github.com/orz-i/Mender/backend/internal/catalog/domain"
	mcpdelivery "github.com/orz-i/Mender/backend/internal/mcp/application/delivery"
	mcpdomain "github.com/orz-i/Mender/backend/internal/mcp/domain"
	profileapp "github.com/orz-i/Mender/backend/internal/profile/application"
)

func TestManagedMCPHTTPDeliveryChangesKeepSelectionListenerAndTunnel(t *testing.T) {
	f := newLiveMCPFixture(t, "hybrid", true)
	ctx := context.Background()
	if _, err := addMCPProfileToolFixture(t, f.runtime, ctx, profileapp.AddToolRequest{
		ProfileSlug: f.profile.Slug(), ToolID: f.ids["echo_beta"],
	}, false); err != nil {
		t.Fatal(err)
	}
	if names := f.toolNames(t); !containsName(names, "upstream__echo_alpha") || containsName(names, "upstream__echo_beta") {
		t.Fatalf("hybrid membership changed explicit direct selection: %v", names)
	}
	before, err := f.runtime.GetProfileMCPDelivery(ctx, f.profile.Slug())
	if err != nil {
		t.Fatal(err)
	}
	tunnelID, _ := before.Config.TunnelID()
	revision := before.Config.Revision()
	updated, err := f.runtime.ConfigureProfileMCPDelivery(ctx, f.profile.Slug(), mcpdelivery.ConfigureRequest{
		Mode: mcpdomain.ModeHybrid, TunnelID: tunnelID, DirectToolIDs: []catalogdomain.ToolID{f.ids["echo_beta"]},
		ExpectedProfileRevision: before.Profile.Revision(), ExpectedRevision: &revision,
	})
	if err != nil {
		t.Fatal(err)
	}
	if updated.Profile.Revision() != before.Profile.Revision() || updated.Config.Revision() <= revision {
		t.Fatal("delivery configuration mutated Profile selection or failed to advance its own revision")
	}
	if names := f.toolNames(t); containsName(names, "upstream__echo_alpha") || !containsName(names, "upstream__echo_beta") || !containsName(names, "mender_call_tool") {
		t.Fatalf("delivery-only direct change was not refreshed: %v", names)
	}
	previousAlpha := f.callCounts[0].Load()
	result, callErr := f.session.CallTool(ctx, &mcp.CallToolParams{Name: "upstream__echo_alpha", Arguments: json.RawMessage(`{}`)})
	if callErr == nil && (result == nil || !result.IsError) {
		t.Fatal("removed direct export remained callable")
	}
	if f.callCounts[0].Load() != previousAlpha {
		t.Fatal("removed direct export reached upstream")
	}
	// Configuring visibility cannot revoke or create Profile membership.
	// The still-selected alpha remains available through hybrid dynamic calls.
	result, callErr = f.session.CallTool(ctx, &mcp.CallToolParams{Name: "mender_call_tool", Arguments: json.RawMessage(
		`{"toolId":"` + f.ids["echo_alpha"].String() + `","arguments":{}}`)})
	if callErr != nil || result == nil || result.IsError || f.callCounts[0].Load() != previousAlpha+1 {
		t.Fatalf("direct visibility configuration changed selected Tool authority: %+v %v", result, callErr)
	}
	result, callErr = f.session.CallTool(ctx, &mcp.CallToolParams{Name: "upstream__echo_beta", Arguments: json.RawMessage(`{}`)})
	if callErr != nil || result == nil || result.IsError || f.callCounts[1].Load() != 1 {
		t.Fatalf("new direct export did not invoke canonical Tool: %+v %v", result, callErr)
	}
	revision = updated.Config.Revision()
	if _, err := f.runtime.ConfigureProfileMCPDelivery(ctx, f.profile.Slug(), mcpdelivery.ConfigureRequest{
		Mode: mcpdomain.ModeDynamic, TunnelID: tunnelID,
		ExpectedProfileRevision: updated.Profile.Revision(), ExpectedRevision: &revision,
	}); err != nil {
		t.Fatal(err)
	}
	if names := f.toolNames(t); len(names) != 3 || !containsName(names, "mender_search_tools") {
		t.Fatalf("delivery-only mode change did not refresh SDK projection: %v", names)
	}
	if names := f.searchNames(t); len(names) != 2 || !containsName(names, "echo_alpha") || !containsName(names, "echo_beta") {
		t.Fatalf("mode change altered Profile membership: %v", names)
	}
	f.assertStableListener(t)
	original := f.tunnelRunner.processAt(t, 0)
	f.tunnelRunner.mu.Lock()
	processCount := len(f.tunnelRunner.processes)
	f.tunnelRunner.mu.Unlock()
	original.mu.Lock()
	stopped := original.stopped
	original.mu.Unlock()
	if processCount != 1 || stopped {
		t.Fatalf("delivery update restarted Tunnel: count=%d stopped=%t", processCount, stopped)
	}
}

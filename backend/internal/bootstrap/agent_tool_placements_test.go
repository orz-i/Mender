package bootstrap

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	agentdomain "github.com/orz-i/Mender/backend/internal/agent/domain"
	sandboxdomain "github.com/orz-i/Mender/backend/internal/sandbox/domain"
)

func TestConfiguredAgentTaskReceivesOptionalGovernedToolSupplyAndCanDisableIt(t *testing.T) {
	f := setupGovernedTaskToolFixture(t)
	ctx := context.Background()
	// The first Task was admitted without a configured placement. Existing
	// Host/CLI semantics never create an implicit Sandbox or MCP capability.
	select {
	case first := <-f.executor.selected:
		if !first.IsZero() {
			t.Fatalf("default Agent was silently given a capability: %+v", first)
		}
	case <-time.After(4 * time.Second):
		t.Fatal("missing default Runner configuration")
	}
	placement, err := f.runtime.ConfigureAgentToolPlacement(ctx, f.agent.ID(), "", f.sandbox.ID().String(),
		[]string{f.ids["workspace.read"].String()}, f.owner)
	if err != nil {
		t.Fatal(err)
	}
	if placement.WorkspaceRoot != f.root || placement.SandboxID != f.sandbox.ID().String() || len(placement.ToolIDs) != 1 {
		t.Fatalf("wrong saved owner config: %+v", placement)
	}
	listed, err := f.runtime.ListAgentToolPlacements(ctx, f.agent.ID(), f.owner)
	if err != nil || len(listed) != 1 {
		t.Fatalf("owner configuration not visible: %+v %v", listed, err)
	}
	if _, err := f.runtime.ListAgentToolPlacements(ctx, f.agent.ID(), taskServicePrincipal(t, f.profile.ID().String())); !errors.Is(err, agentdomain.ErrForbidden) {
		t.Fatalf("Profile queried owner-only placement: %v", err)
	}
	next, err := f.runtime.Agents.Submit(ctx, f.agent.ID(), "bounded second Task", taskServicePrincipal(t, f.profile.ID().String()))
	if err != nil {
		t.Fatal(err)
	}
	select {
	case <-f.executor.started:
	case <-time.After(4 * time.Second):
		t.Fatal("configured Runner never started")
	}
	var selected string
	var bearer string
	select {
	case supply := <-f.executor.selected:
		if supply.IsZero() || supply.Validate() != nil {
			t.Fatalf("configured Task missing valid ToolSupply %+v", supply)
		}
		selected = supply.Endpoint
		bearer = supply.BearerToken
	case <-time.After(4 * time.Second):
		t.Fatal("configured ToolSupply was not delivered")
	}
	session, err := connectTaskScopedMCP(t, selected, bearer)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = session.Close() }()
	tools, err := session.ListTools(ctx, nil)
	if err != nil || len(tools.Tools) != 1 || tools.Tools[0].Name != "mender_read" {
		t.Fatalf("wrong canonical Tools exposed: %+v %v", tools, err)
	}
	if err := f.runtime.DisableAgentToolPlacement(ctx, f.agent.ID(), "", f.owner); err != nil {
		t.Fatal(err)
	}
	if _, err := f.runtime.agentStore.GetToolPlacement(ctx, f.agent.ID(), ""); !errors.Is(err, agentdomain.ErrNotFound) {
		t.Fatalf("owner disable failed: %v", err)
	}
	// Both already-connected clients and new clients lose the old capability.
	res, err := session.CallTool(ctx, &mcp.CallToolParams{Name: "mender_read", Arguments: []byte(`{"path":"review.txt"}`)})
	if err == nil && res != nil && !res.IsError {
		t.Fatal("disabled tool placement retained read authority")
	}
	if _, err := connectTaskScopedMCP(t, selected, bearer); err == nil {
		t.Fatal("disabled governed placement still accepted its old bearer")
	}
	if err := f.runtime.Agents.Cancel(ctx, next.ID, f.owner); err != nil {
		t.Fatal(err)
	}
}
func TestConfiguredPlacementRejectsWrongWorkspaceOrUnpreparedSandbox(t *testing.T) {
	f := setupGovernedTaskToolFixture(t)
	ctx := context.Background()
	if _, err := f.runtime.ConfigureAgentToolPlacement(ctx, f.agent.ID(), "missing", f.sandbox.ID().String(),
		[]string{f.ids["workspace.read"].String()}, f.owner); err == nil {
		t.Fatal("owner configured an unregistered workspace alias")
	}
	unknown, _ := sandboxdomain.ParseID("sbx_ffffffffffffffffffffffffffffffff")
	if _, err := f.runtime.ConfigureAgentToolPlacement(ctx, f.agent.ID(), "", unknown.String(),
		[]string{f.ids["workspace.read"].String()}, f.owner); err == nil {
		t.Fatal("owner configured a nonexistent Sandbox")
	}
	if _, err := f.runtime.ConfigureAgentToolPlacement(ctx, f.agent.ID(), "", f.sandbox.ID().String(),
		[]string{f.ids["workspace.shell"].String()}, f.owner); !errors.Is(err, agentdomain.ErrForbidden) {
		t.Fatalf("shell Tool was authorized by selection: %v", err)
	}
	if _, err := f.runtime.ConfigureAgentToolPlacement(ctx, f.agent.ID(), "", f.sandbox.ID().String(),
		[]string{f.ids["workspace.read"].String(), f.ids["workspace.read"].String()}, f.owner); !errors.Is(err, agentdomain.ErrInvalid) {
		t.Fatalf("duplicate ToolID accepted: %v", err)
	}
	wrongSandbox, err := f.runtime.Sandboxes.CreateLocalSandbox(ctx, CreateLocalSandboxRequest{
		Name: "Unrelated restricted Sandbox", Root: t.TempDir(), Isolation: sandboxdomain.IsolationRestrictedHost,
	}, f.owner)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := f.runtime.EnsureSandboxWorkspaceTools(ctx, wrongSandbox.ID(), f.owner); err != nil {
		t.Fatal(err)
	}
	if _, err := f.runtime.ConfigureAgentToolPlacement(ctx, f.agent.ID(), "", wrongSandbox.ID().String(),
		[]string{f.ids["workspace.read"].String()}, f.owner); !errors.Is(err, agentdomain.ErrForbidden) {
		t.Fatalf("Sandbox root substitution accepted: %v", err)
	}
}

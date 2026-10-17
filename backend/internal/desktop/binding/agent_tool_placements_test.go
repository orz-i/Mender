package binding

import (
	"context"
	"path/filepath"
	"testing"

	"github.com/orz-i/Mender/backend/internal/bootstrap"
	gov "github.com/orz-i/Mender/backend/internal/governance/domain"
	sandboxdomain "github.com/orz-i/Mender/backend/internal/sandbox/domain"
)

func TestDesktopOwnerCanConfigureAndDisableExactAgentToolPlacement(t *testing.T) {
	ctx := context.Background()
	runtime, err := bootstrap.NewRuntime(ctx, bootstrap.RuntimeOptions{DatabasePath: filepath.Join(t.TempDir(), "wails-governed.db")})
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = runtime.Close() }()
	desktop := NewService(ctx, runtime)
	owner, err := gov.NewContext(gov.OriginDesktop, runtime.OwnerPrincipal(), gov.CatalogScope())
	if err != nil {
		t.Fatal(err)
	}
	root := t.TempDir()
	sandbox, err := runtime.Sandboxes.CreateLocalSandbox(ctx, bootstrap.CreateLocalSandboxRequest{
		Name: "Owner approved restricted Sandbox", Root: root, Isolation: sandboxdomain.IsolationRestrictedHost,
	}, owner)
	if err != nil {
		t.Fatal(err)
	}
	suite, err := runtime.EnsureSandboxWorkspaceTools(ctx, sandbox.ID(), owner)
	if err != nil {
		t.Fatal(err)
	}
	var readID string
	for _, tool := range suite.Tools {
		if tool.SourceKey == "workspace.read" {
			readID = tool.ToolID
		}
	}
	if readID == "" {
		t.Fatal("preexisting Workspace read ToolID unavailable")
	}
	a, err := desktop.CreateAgent(CreateAgentInput{Name: "Owner governed", Backend: "opencode", WorkspaceRoot: root, Isolation: "restricted_host"})
	if err != nil {
		t.Fatal(err)
	}
	listed, err := desktop.ListAgentToolPlacements(a.AgentID)
	if err != nil || len(listed.Placements) != 0 {
		t.Fatalf("placement auto-created: %+v %v", listed, err)
	}
	saved, err := desktop.ConfigureAgentToolPlacement(a.AgentID, "", sandbox.ID().String(), []string{readID})
	if err != nil || saved.AgentID != a.AgentID || saved.WorkspaceKey != "" || saved.SandboxID != sandbox.ID().String() ||
		len(saved.ToolIDs) != 1 || saved.ToolIDs[0] != readID {
		t.Fatalf("invalid owner configuration %+v %v", saved, err)
	}
	listed, err = desktop.ListAgentToolPlacements(a.AgentID)
	if err != nil || len(listed.Placements) != 1 {
		t.Fatalf("owner cannot list configuration %+v %v", listed, err)
	}
	if err := desktop.DisableAgentToolPlacement(a.AgentID, ""); err != nil {
		t.Fatal(err)
	}
	listed, err = desktop.ListAgentToolPlacements(a.AgentID)
	if err != nil || len(listed.Placements) != 0 {
		t.Fatalf("disable left a config %+v %v", listed, err)
	}
	after, err := runtime.SandboxWorkspaceTools(ctx, sandbox.ID(), owner)
	if err != nil || after.ProviderID != suite.ProviderID || len(after.Tools) != len(suite.Tools) {
		t.Fatalf("owner configuration unexpectedly modified Tool inventory: %+v %v", after, err)
	}
	host, err := desktop.CreateAgent(CreateAgentInput{Name: "Existing Host", Backend: "opencode", WorkspaceRoot: t.TempDir(), Isolation: "host"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := desktop.ConfigureAgentToolPlacement(host.AgentID, "", sandbox.ID().String(), []string{readID}); err == nil {
		t.Fatal("Wails owner enabled governed mode on an existing Host Agent")
	}
}

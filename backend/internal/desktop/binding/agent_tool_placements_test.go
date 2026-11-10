package binding

import (
	"context"
	"errors"
	"path/filepath"
	"testing"

	agentdomain "github.com/orz-i/Mender/backend/internal/agent/domain"
	"github.com/orz-i/Mender/backend/internal/bootstrap"
	gov "github.com/orz-i/Mender/backend/internal/governance/domain"
	processisolation "github.com/orz-i/Mender/backend/internal/runtimegovernance/infrastructure/processisolation"
	sandboxdomain "github.com/orz-i/Mender/backend/internal/sandbox/domain"
)

func TestDesktopOwnerCanConfigureAndDisableExactAgentToolPlacement(t *testing.T) {
	if !processisolation.New().Info().RestrictedHostAvailable {
		t.Skip("governed placement requires a verified restricted_host backend; no host fallback")
	}

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
	suite, err := runtime.SandboxWorkspaceTools(ctx, sandbox.ID(), owner)
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
	if err != nil || len(after.Tools) != len(suite.Tools) {
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

// This refusal coverage runs even where restricted_host is unavailable. A
// prepared Host Sandbox cannot substitute for the governed execution boundary.
func TestDesktopAgentToolPlacementRejectsUnisolatedHost(t *testing.T) {
	ctx := context.Background()
	runtime, err := bootstrap.NewRuntime(ctx, bootstrap.RuntimeOptions{DatabasePath: filepath.Join(t.TempDir(), "wails-host-refusal.db")})
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
		Name: "Explicit unisolated Host", Root: root, Isolation: sandboxdomain.IsolationHost,
	}, owner)
	if err != nil {
		t.Fatal(err)
	}
	suite, err := runtime.SandboxWorkspaceTools(ctx, sandbox.ID(), owner)
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
		t.Fatal("Host fixture has no canonical Workspace read tool")
	}
	for _, tc := range []struct {
		isolation string
		wantError error
	}{
		{"host", agentdomain.ErrUnavailable},
		{"restricted_host", agentdomain.ErrForbidden},
	} {
		t.Run(tc.isolation, func(t *testing.T) {
			agent, err := desktop.CreateAgent(CreateAgentInput{
				Name: "Unisolated target refusal", Backend: "opencode", WorkspaceRoot: root, Isolation: tc.isolation,
			})
			if err != nil {
				t.Fatal(err)
			}
			if _, err := desktop.ConfigureAgentToolPlacement(agent.AgentID, "", sandbox.ID().String(), []string{readID}); !errors.Is(err, tc.wantError) {
				t.Fatalf("Host target accepted for %s Agent: %v", tc.isolation, err)
			}
			listed, err := desktop.ListAgentToolPlacements(agent.AgentID)
			if err != nil || len(listed.Placements) != 0 {
				t.Fatalf("refused target left an execution placement: %+v %v", listed, err)
			}
		})
	}
}

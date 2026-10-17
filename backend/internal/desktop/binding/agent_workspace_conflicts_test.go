package binding

import (
	"context"
	"errors"
	"path/filepath"
	"strings"
	"testing"

	agentdomain "github.com/orz-i/Mender/backend/internal/agent/domain"
	"github.com/orz-i/Mender/backend/internal/bootstrap"
)

func TestDesktopAgentWorkspaceDuplicatePreservesTypedConflictForWails(t *testing.T) {
	ctx := context.Background()
	runtime, err := bootstrap.NewRuntime(ctx, bootstrap.RuntimeOptions{
		DatabasePath: filepath.Join(t.TempDir(), "binding-duplicate.db"),
	})
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = runtime.Close() }()
	service := NewService(ctx, runtime)
	root := t.TempDir()
	created, err := service.CreateAgent(CreateAgentInput{Name: "Wails Workspace", Backend: "opencode",
		WorkspaceRoot: root, Isolation: "host"})
	if err != nil {
		t.Fatal(err)
	}
	if err := service.BindAgentWorkspace(created.AgentID, "mender", root); !errors.Is(err, agentdomain.ErrWorkspaceDefaultDirectory) ||
		!strings.Contains(err.Error(), "agent_workspace_default_directory") {
		t.Fatalf("Wails lost default-root conflict identity: %v", err)
	}
	second := t.TempDir()
	canonicalSecond, err := filepath.EvalSymlinks(second)
	if err != nil {
		t.Fatal(err)
	}
	if err := service.BindAgentWorkspace(created.AgentID, "gaoge", second); err != nil {
		t.Fatal(err)
	}
	if err := service.BindAgentWorkspace(created.AgentID, "gaoge", t.TempDir()); !errors.Is(err, agentdomain.ErrWorkspaceAliasExists) ||
		!strings.Contains(err.Error(), "agent_workspace_alias_exists") {
		t.Fatalf("Wails lost duplicate-alias conflict identity: %v", err)
	}
	if err := service.BindAgentWorkspace(created.AgentID, "another", second); !errors.Is(err, agentdomain.ErrWorkspaceDirectoryExists) ||
		!strings.Contains(err.Error(), "agent_workspace_directory_exists") {
		t.Fatalf("Wails lost duplicate-directory conflict identity: %v", err)
	}
	workspaces, err := service.ListAgentWorkspaces(created.AgentID)
	if err != nil || len(workspaces.Workspaces) != 1 || workspaces.Workspaces[0].Key != "gaoge" ||
		workspaces.Workspaces[0].WorkspaceRoot != canonicalSecond || len(workspaces.Workspaces[0].ProfileIDs) != 0 {
		t.Fatalf("workspace inventory changed on rejected conflicts: %+v %v", workspaces, err)
	}
}

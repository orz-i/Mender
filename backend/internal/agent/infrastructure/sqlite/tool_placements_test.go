package sqlite

import (
	"context"
	"errors"
	"path/filepath"
	"testing"
	"time"

	agentdomain "github.com/orz-i/Mender/backend/internal/agent/domain"
)

func TestOptionalToolPlacementV11ExactRootAndDisable(t *testing.T) {
	ctx := context.Background()
	s, err := Open(ctx, filepath.Join(t.TempDir(), "placement-v11.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = s.Close() }()
	id, err := agentdomain.NewID()
	if err != nil {
		t.Fatal(err)
	}
	root := t.TempDir()
	another := t.TempDir()
	a, err := agentdomain.NewAgent(agentdomain.Spec{ID: id, Name: "Placement v11", Backend: agentdomain.BackendGrok,
		PluginID: agentdomain.BackendGrok.PluginID(), ExtensionID: "main", WorkspaceRoot: root,
		Isolation: "restricted_host", OwnerKind: "local_user", OwnerRef: "os:501", CreatedAt: time.Now().UTC()})
	if err != nil {
		t.Fatal(err)
	}
	if err := s.CreateAgent(ctx, a); err != nil {
		t.Fatal(err)
	}
	toolA := "tool_0123456789abcdef0123456789abcdef"
	toolB := "tool_1123456789abcdef0123456789abcdef"
	sandID := "sbx_0123456789abcdef0123456789abcdef"
	defaultPlacement := agentdomain.ToolPlacement{AgentID: id, WorkspaceRoot: root, SandboxID: sandID,
		ToolIDs: []string{toolA, toolB}, UpdatedAt: time.Now().UTC()}
	if err := s.PutToolPlacement(ctx, defaultPlacement); err != nil {
		t.Fatal(err)
	}
	got, err := s.GetToolPlacement(ctx, id, "")
	if err != nil || !agentdomain.SameToolPlacement(got, defaultPlacement) {
		t.Fatalf("default placement not round-tripped %+v %v", got, err)
	}
	if err := s.BindWorkspace(ctx, id, "project", another); err != nil {
		t.Fatal(err)
	}
	alias := defaultPlacement
	alias.WorkspaceKey = "project"
	alias.WorkspaceRoot = another
	alias.ToolIDs = []string{toolA}
	alias.UpdatedAt = time.Now().UTC().Add(time.Second)
	if err := s.PutToolPlacement(ctx, alias); err != nil {
		t.Fatal(err)
	}
	values, err := s.ListToolPlacements(ctx, id)
	if err != nil || len(values) != 2 {
		t.Fatalf("registered placement count=%d err=%v", len(values), err)
	}
	stale := alias
	stale.WorkspaceRoot = t.TempDir()
	if err := s.PutToolPlacement(ctx, stale); !errors.Is(err, agentdomain.ErrConflict) {
		t.Fatalf("stale workspace path accepted: %v", err)
	}
	if err := s.UnbindWorkspace(ctx, id, "project"); !errors.Is(err, agentdomain.ErrConflict) || !errors.Is(err, agentdomain.ErrWorkspaceGovernedToolsEnabled) {
		t.Fatalf("workspace with active placement was not rejected with typed reason: %v", err)
	}
	if err := s.DeleteToolPlacement(ctx, id, "project"); err != nil {
		t.Fatal(err)
	}
	if err := s.UnbindWorkspace(ctx, id, "project"); err != nil {
		t.Fatal(err)
	}
	if err := s.PutToolPlacement(ctx, alias); !errors.Is(err, agentdomain.ErrConflict) {
		t.Fatalf("removed workspace resurrected placement: %v", err)
	}
	if err := s.DeleteToolPlacement(ctx, id, ""); err != nil {
		t.Fatal(err)
	}
	if _, err := s.GetToolPlacement(ctx, id, ""); !errors.Is(err, agentdomain.ErrNotFound) {
		t.Fatalf("default not disabled: %v", err)
	}
	if got, err := s.ListToolPlacements(ctx, id); err != nil || len(got) != 0 {
		t.Fatalf("placement inventory still present %+v %v", got, err)
	}
	var version int
	if err := s.db.QueryRowContext(ctx, "SELECT MAX(version) FROM agent_schema_migrations").Scan(&version); err != nil || version != latestSchemaVersion {
		t.Fatalf("expected current Agent v13: %d %v", version, err)
	}
}
func TestToolPlacementRejectsInvalidToolIDsAndAliases(t *testing.T) {
	id, err := agentdomain.NewID()
	if err != nil {
		t.Fatal(err)
	}
	fixture := agentdomain.ToolPlacement{AgentID: id, WorkspaceRoot: "/tmp/placement", SandboxID: "sbx_0123456789abcdef0123456789abcdef",
		ToolIDs: []string{"tool_0123456789abcdef0123456789abcdef"}, UpdatedAt: time.Now().UTC()}
	if err := fixture.Validate(); err != nil {
		t.Fatal(err)
	}
	for _, bad := range []agentdomain.ToolPlacement{
		func() agentdomain.ToolPlacement { v := fixture; v.WorkspaceKey = "bad/alias"; return v }(),
		func() agentdomain.ToolPlacement { v := fixture; v.SandboxID = "wrong"; return v }(),
		func() agentdomain.ToolPlacement { v := fixture; v.WorkspaceRoot = "relative"; return v }(),
		func() agentdomain.ToolPlacement { v := fixture; v.ToolIDs = []string{}; return v }(),
		func() agentdomain.ToolPlacement {
			v := fixture
			v.ToolIDs = []string{fixture.ToolIDs[0], fixture.ToolIDs[0]}
			return v
		}(),
		func() agentdomain.ToolPlacement {
			v := fixture
			v.ToolIDs = []string{"tool_aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa", "tool_0123456789abcdef0123456789abcdef"}
			return v
		}(),
	} {
		if err := bad.Validate(); err == nil {
			t.Fatalf("invalid placement accepted: %+v", bad)
		}
	}
}

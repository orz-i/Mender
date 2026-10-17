package sqlite

import (
	"context"
	"errors"
	"path/filepath"
	"strings"
	"testing"
	"time"

	agentdomain "github.com/orz-i/Mender/backend/internal/agent/domain"
)

func TestWorkspaceCannotBeUnboundWithActiveOwnerApprovalMode(t *testing.T) {
	ctx := context.Background()
	store, err := Open(ctx, filepath.Join(t.TempDir(), "workspace-owner-approval.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = store.Close() }()
	id, err := agentdomain.NewID()
	if err != nil {
		t.Fatal(err)
	}
	root, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	extra, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	agent, err := agentdomain.NewAgent(agentdomain.Spec{
		ID: id, Name: "Additional workspace consent", Backend: agentdomain.BackendGrok,
		PluginID: agentdomain.BackendGrok.PluginID(), ExtensionID: "main",
		WorkspaceRoot: root, Isolation: "host",
		OwnerKind: "local_user", OwnerRef: "os:501", CreatedAt: time.Now().UTC(),
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := store.CreateAgent(ctx, agent); err != nil {
		t.Fatal(err)
	}
	if err := store.BindWorkspace(ctx, id, "secondary", extra); err != nil {
		t.Fatal(err)
	}
	p := agentdomain.ApprovalPolicy{AgentID: id, WorkspaceKey: "secondary", WorkspaceRoot: extra,
		Mode: agentdomain.ApprovalWorkspaceRead, UpdatedAt: time.Now().UTC()}
	if err := store.SetApprovalPolicy(ctx, p); err != nil {
		t.Fatal(err)
	}
	if err := store.UnbindWorkspace(ctx, id, "secondary"); err == nil || !strings.Contains(err.Error(), "disable Agent workspace approval mode before unbinding") {
		t.Fatalf("workspace was removed while owner policy still active: %v", err)
	}
	kept, err := store.Workspace(ctx, id, "secondary")
	if err != nil || kept != extra {
		t.Fatalf("failed unbind corrupted registered root: %q %v", kept, err)
	}
	if err := store.ClearApprovalPolicy(ctx, id, "secondary"); err != nil {
		t.Fatal(err)
	}
	if err := store.UnbindWorkspace(ctx, id, "secondary"); err != nil {
		t.Fatal(err)
	}
	if _, err := store.Workspace(ctx, id, "secondary"); !errors.Is(err, agentdomain.ErrNotFound) {
		t.Fatalf("revoked workspace binding unexpectedly remains: %v", err)
	}
	var changes int
	if err := store.db.QueryRowContext(ctx, "SELECT COUNT(*) FROM agent_approval_policy_events WHERE agent_id=? AND workspace_key='secondary'", id).Scan(&changes); err != nil || changes != 2 {
		t.Fatalf("owner set/revoke audit missing: %d %v", changes, err)
	}
}

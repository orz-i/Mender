package sqlite

import (
	"context"
	"path/filepath"
	"testing"
	"time"

	agentdomain "github.com/orz-i/Mender/backend/internal/agent/domain"
)

func TestOwnerApprovalModeChangesLeaveImmutableAuditEvents(t *testing.T) {
	ctx := context.Background()
	db, err := Open(ctx, filepath.Join(t.TempDir(), "policy-events.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = db.Close() }()
	id, err := agentdomain.NewID()
	if err != nil {
		t.Fatal(err)
	}
	root, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	agent, err := agentdomain.NewAgent(agentdomain.Spec{
		ID: id, Name: "Policy audit fixture", Backend: agentdomain.BackendOpenCode,
		PluginID: agentdomain.BackendOpenCode.PluginID(), ExtensionID: "main",
		WorkspaceRoot: root, Isolation: "host", OwnerKind: "local_user",
		OwnerRef: "os:501", CreatedAt: time.Now().UTC(),
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := db.CreateAgent(ctx, agent); err != nil {
		t.Fatal(err)
	}
	now := time.Now().UTC()
	read := agentdomain.ApprovalPolicy{AgentID: id, WorkspaceRoot: root, Mode: agentdomain.ApprovalWorkspaceRead, UpdatedAt: now}
	if err := db.SetApprovalPolicy(ctx, read); err != nil {
		t.Fatal(err)
	}
	trusted := read
	trusted.Mode = agentdomain.ApprovalTrusted
	trusted.ExpiresAt = now.Add(23 * time.Hour)
	trusted.UpdatedAt = now.Add(time.Second)
	if err := db.SetApprovalPolicy(ctx, trusted); err != nil {
		t.Fatal(err)
	}
	if err := db.ClearApprovalPolicy(ctx, id, ""); err != nil {
		t.Fatal(err)
	}
	rows, err := db.db.QueryContext(ctx, `SELECT agent_id,workspace_key,workspace_root,action,mode,expires_unix_ms
	 FROM agent_approval_policy_events WHERE agent_id=? ORDER BY event_id`, id)
	if err != nil {
		t.Fatal(err)
	}
	type entry struct {
		agentID, key, root, action, mode string
		expires                          int64
	}
	var events []entry
	for rows.Next() {
		var e entry
		if err := rows.Scan(&e.agentID, &e.key, &e.root, &e.action, &e.mode, &e.expires); err != nil {
			_ = rows.Close()
			t.Fatal(err)
		}
		events = append(events, e)
	}
	if err := rows.Close(); err != nil {
		t.Fatal(err)
	}
	if len(events) != 3 {
		t.Fatalf("every grant/revocation must leave an event; got %d", len(events))
	}
	for _, e := range events {
		if e.agentID != string(id) || e.key != "" || e.root != root {
			t.Fatal("authorization change was recorded against the wrong Agent/workspace")
		}
	}
	if events[0].action != "set" || events[0].mode != "workspace_read" || events[0].expires != 0 ||
		events[1].action != "set" || events[1].mode != "trusted" || events[1].expires == 0 ||
		events[2].action != "revoke" || events[2].mode != "trusted" || events[2].expires != events[1].expires {
		t.Fatalf("incorrect owner permission audit history: %+v", events)
	}
	for _, query := range []string{
		"DELETE FROM agent_approval_policy_events",
		"UPDATE agent_approval_policy_events SET action='set' WHERE event_id=1",
	} {
		if _, err := db.db.ExecContext(ctx, query); err == nil {
			t.Fatal("historical owner permission change could be deleted or modified")
		}
	}
	var still int
	if err := db.db.QueryRowContext(ctx, "SELECT COUNT(*) FROM agent_approval_policy_events").Scan(&still); err != nil || still != 3 {
		t.Fatal("attempted audit tampering destroyed owner event history", err)
	}
}

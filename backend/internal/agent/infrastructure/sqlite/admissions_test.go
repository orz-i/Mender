package sqlite

import (
	"context"
	"encoding/json"
	"path/filepath"
	"strings"
	"testing"

	agentdomain "github.com/orz-i/Mender/backend/internal/agent/domain"
)

func TestAgentAdmissionAuditIsImmutableAndDataMinimized(t *testing.T) {
	ctx := context.Background()
	path := filepath.Join(t.TempDir(), "agent-audit.db")
	store, err := Open(ctx, path)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = store.Close() }()
	id, err := agentdomain.NewID()
	if err != nil {
		t.Fatal(err)
	}
	e := AdmissionEvent{
		AgentID: id, Action: "agent.task.submit",
		PrincipalKind: "service", PrincipalRef: "profile:private-subject",
		Origin: "mcp", ScopeKind: "profile", ScopeRef: "private-scope",
		PolicyKind: "deny", PolicyReason: "policy_default_deny", Status: "denied",
	}
	if err := store.RecordAdmission(ctx, e); err != nil {
		t.Fatal(err)
	}
	if err := store.RecordAdmission(ctx, AdmissionEvent{AgentID: id, Action: "bad.action"}); err == nil {
		t.Fatal("invalid Agent governance action audited")
	}
	events, err := store.ListAdmissions(ctx, id, 10)
	if err != nil || len(events) != 1 || events[0].Status != "denied" {
		t.Fatalf("admission audit=%+v / %v", events, err)
	}
	projected, err := json.Marshal(events[0])
	if err != nil {
		t.Fatal(err)
	}
	for _, private := range []string{"private-subject", "private-scope", "prompt", "credential"} {
		if strings.Contains(strings.ToLower(string(projected)), private) {
			t.Fatalf("audit leaked %s: %s", private, projected)
		}
	}
	if _, err := store.db.ExecContext(ctx, "UPDATE agent_admissions SET status='admitted' WHERE agent_id=?", string(id)); err == nil {
		t.Fatal("admission audit was overwritten")
	}
	if _, err := store.db.ExecContext(ctx, "DELETE FROM agent_admissions WHERE agent_id=?", string(id)); err == nil {
		t.Fatal("admission audit was deleted")
	}
	if err := store.Close(); err != nil {
		t.Fatal(err)
	}
	reopened, err := Open(ctx, path)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = reopened.Close() }()
	events, err = reopened.ListAdmissions(ctx, id, 10)
	if err != nil || len(events) != 1 {
		t.Fatalf("audit did not survive restart: %+v / %v", events, err)
	}
}

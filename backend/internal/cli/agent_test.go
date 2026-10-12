package cli

import (
	"bytes"
	"context"
	"encoding/json"
	"path/filepath"
	"testing"

	"github.com/orz-i/Mender/backend/internal/bootstrap"
	profileapplication "github.com/orz-i/Mender/backend/internal/profile/application"
	profiledomain "github.com/orz-i/Mender/backend/internal/profile/domain"
)

func runAgentJSON(t *testing.T, db string, args ...string) map[string]any {
	t.Helper()
	var stdout, stderr bytes.Buffer
	cmd := NewRootCommand(Options{Stdout: &stdout, Stderr: &stderr})
	cmd.SetArgs(append([]string{"--db", db, "--output", "json"}, args...))
	if err := cmd.ExecuteContext(context.Background()); err != nil {
		t.Fatalf("mender %v failed: %v, stderr=%s", args, err, stderr.String())
	}
	var output map[string]any
	if err := json.Unmarshal(stdout.Bytes(), &output); err != nil {
		t.Fatalf("mender %v returned invalid JSON: %v,stdout=%s", args, err, stdout.String())
	}
	return output
}

func TestAgentCLIResourceAndProfileGrantLifecycle(t *testing.T) {
	ctx := context.Background()
	db := filepath.Join(t.TempDir(), "agent-cli.sqlite")
	root := t.TempDir()
	canonicalRoot, err := filepath.EvalSymlinks(root)
	if err != nil {
		t.Fatal(err)
	}
	created := runAgentJSON(t, db, "agent", "add", "--name", "Review Agent", "--backend", "grok",
		"--root", root, "--isolation", "restricted_host")
	item, ok := created["agent"].(map[string]any)
	if !ok || item["backend"] != "grok" || item["workspaceRoot"] != canonicalRoot {
		t.Fatalf("created Agent=%v", created)
	}
	id, ok := item["agentId"].(string)
	if !ok || id == "" {
		t.Fatal("Agent ID missing")
	}
	inventory := runAgentJSON(t, db, "agent", "list")
	list, ok := inventory["agents"].([]any)
	if !ok || len(list) != 1 {
		t.Fatalf("list=%v", inventory)
	}
	inspected := runAgentJSON(t, db, "agent", "show", id)
	if inspected["agent"].(map[string]any)["agentId"] != id {
		t.Fatal("show Agent identity mismatch")
	}
	toolState := runAgentJSON(t, db, "agent", "tools", id)["suite"].(map[string]any)
	if toolState["status"] != "ready" || len(toolState["tools"].([]any)) != 4 {
		t.Fatalf("CLI Agent tools missing: %v", toolState)
	}
	prepared := runAgentJSON(t, db, "agent", "tools", id, "--prepare")["suite"].(map[string]any)
	if prepared["providerId"] != toolState["providerId"] {
		t.Fatalf("CLI repair replaced stable ProviderID: %v", prepared)
	}
	admissions := runAgentJSON(t, db, "agent", "admissions", id)
	if len(admissions["events"].([]any)) != 0 {
		t.Fatalf("unexpected admission events: %v", admissions)
	}
	pending := runAgentJSON(t, db, "agent", "tasks", id)
	if len(pending["tasks"].([]any)) != 0 {
		t.Fatalf("unexpected tasks=%v", pending)
	}

	r, err := bootstrap.NewRuntime(ctx, bootstrap.RuntimeOptions{DatabasePath: db})
	if err != nil {
		t.Fatal(err)
	}
	profile, err := r.Profiles.Create.Execute(ctx, profileapplication.CreateProfileRequest{
		Slug: "agent-fixture", DisplayName: "Agent Fixture", Mode: profiledomain.ExposureDynamic,
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := r.Close(); err != nil {
		t.Fatal(err)
	}
	profileID := profile.ID().String()
	runAgentJSON(t, db, "agent", "grant", id, profileID)
	granted := runAgentJSON(t, db, "agent", "grants", id)
	if len(granted["profileIds"].([]any)) != 1 {
		t.Fatalf("Agent grant not saved: %v", granted)
	}
	runAgentJSON(t, db, "agent", "revoke", id, profileID)
	revoked := runAgentJSON(t, db, "agent", "grants", id)
	if len(revoked["profileIds"].([]any)) != 0 {
		t.Fatalf("Agent grant not revoked: %v", revoked)
	}
	runAgentJSON(t, db, "agent", "remove", id, "--confirm")
	remaining := runAgentJSON(t, db, "agent", "list")
	if len(remaining["agents"].([]any)) != 0 {
		t.Fatalf("Agent not removed: %v", remaining)
	}
}

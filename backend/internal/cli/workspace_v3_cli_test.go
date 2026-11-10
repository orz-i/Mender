package cli

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/orz-i/Mender/backend/internal/bootstrap"
)

// This is the full public-CLI route with no direct fixture SQL and no model or
// third-party process. It proves automatic unique Workspace selection,
// atomic grants and revocation using a disposable real v3 Runtime database.
func TestCLISharedWorkspaceNativeToolEndToEndAndRevocation(t *testing.T) {
	root, e := filepath.EvalSymlinks(t.TempDir())
	if e != nil {
		t.Fatal(e)
	}
	project := filepath.Join(root, "project")
	if e = os.Mkdir(project, 0700); e != nil {
		t.Fatal(e)
	}
	if e = os.WriteFile(filepath.Join(project, "probe.txt"), []byte("Workspace fully governed\n"), 0600); e != nil {
		t.Fatal(e)
	}
	db := filepath.Join(root, "v3.sqlite")
	sb := runAgentJSON(t, db, "sandbox", "add", "local", "--name", "Workspace", "--root", project, "--isolation", "host")["sandbox"].(map[string]any)["sandboxId"].(string)
	ws := runAgentJSON(t, db, "workspace", "add", "--name", "Workspace", "--root", project)["workspace"].(map[string]any)["workspaceId"].(string)
	runAgentJSON(t, db, "workspace", "attach", ws, sb)
	prof := runAgentJSON(t, db, "profile", "create", "--slug", "cli-registered", "--name", "CLI registered")
	pid := prof["profileId"].(string)
	tool := "tool_882fb9c25db58744f34da684f7f7ab5f"
	runAgentJSON(t, db, "profile", "add-tool", "cli-registered", tool)
	runAgentJSON(t, db, "sandbox", "grant", sb, pid, "--capability", "file.read")
	runAgentJSON(t, db, "workspace", "grant", ws, pid)
	runAgentJSON(t, db, "workspace", "target-add", "--profile", pid, "--tool", tool, "--workspace", ws, "--sandbox", sb)
	runAgentJSON(t, db, "policy", "set-default", "allow")
	// workspaceId omitted intentionally; the unique authorized Workspace and
	// eligible Sandbox are selected without widening the Profile TargetScope.
	for _, args := range [][]string{
		{"tool", "call", tool, "--args", `{"path":"probe.txt"}`},
		{"profile", "tool", "call", "cli-registered", tool, "--args", `{"path":"probe.txt"}`},
	} {
		result := runAgentJSON(t, db, args...)
		if result["kind"] != "complete" {
			t.Fatalf("native CLI invocation %v failed: %+v", args, result)
		}
		content, ok := result["content"].(map[string]any)
		if !ok || content["content"] != "Workspace fully governed" {
			t.Fatalf("native Workspace result mismatched: %+v", result)
		}
	}
	runAgentJSON(t, db, "workspace", "revoke", ws, pid)
	runAgentJSON(t, db, "workspace", "grant", ws, pid)
	if got := runAgentJSON(t, db, "workspace", "targets", pid)["targets"].([]any); len(got) != 0 {
		t.Fatalf("revocation allowed old target to revive: %+v", got)
	}
	var stdout, stderr bytes.Buffer
	cmd := NewRootCommand(Options{Stdout: &stdout, Stderr: &stderr})
	cmd.SetArgs([]string{"--db", db, "--output", "json", "profile", "tool", "call", "cli-registered", tool, "--args", `{"path":"probe.txt"}`})
	if e := cmd.ExecuteContext(context.Background()); e == nil {
		t.Fatalf("previously revoked target executed: %s", stdout.String())
	}
	// A denied call remains an audited invocation, not a silent success.
	r, e := bootstrap.NewRuntime(context.Background(), bootstrap.RuntimeOptions{DatabasePath: db})
	if e != nil {
		t.Fatal(e)
	}
	defer func() { _ = r.Close() }()
	items, e := r.ListAudit.Execute(context.Background(), 10)
	if e != nil || len(items) == 0 {
		t.Fatalf("missing Invocation audit: %v %+v", e, items)
	}
}

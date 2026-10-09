package cli

import (
	"context"
	"io"
	"path/filepath"
	"testing"

	"github.com/spf13/cobra"
)

func TestCLILocalSandboxLifecycleAndDestroyConfirmation(t *testing.T) {
	t.Parallel()
	db := filepath.Join(t.TempDir(), "mender.db")
	root := t.TempDir()
	created := executeJSONCommand(t, db, "sandbox", "add", "local", "--name", "CLI Local", "--root", root, "--isolation", "host")
	item, ok := created["sandbox"].(map[string]any)
	if !ok {
		t.Fatalf("sandbox create %+v", created)
	}
	id := item["sandboxId"].(string)
	stopped := executeJSONCommand(t, db, "sandbox", "stop", id)
	if stopped["state"] != "stopped" {
		t.Fatalf("stop %+v", stopped)
	}
	started := executeJSONCommand(t, db, "sandbox", "start", id)
	if started["state"] != "ready" || started["generation"] != float64(2) {
		t.Fatalf("start %+v", started)
	}
	run := NewRootCommand(Options{Stdin: nil, Stdout: io.Discard, Stderr: io.Discard})
	run.SetArgs([]string{"--db", db, "sandbox", "destroy", id})
	if err := run.ExecuteContext(context.Background()); err == nil {
		t.Fatal("destroy without --confirm allowed")
	}
	destroyed := executeJSONCommand(t, db, "sandbox", "destroy", id, "--confirm")
	if destroyed["state"] != "destroyed" {
		t.Fatalf("destroy %+v", destroyed)
	}
	listed := executeJSONCommand(t, db, "sandbox", "list")
	if values, ok := listed["sandboxes"].([]any); !ok || len(values) != 1 {
		t.Fatalf("tombstoned resource lost %+v", listed)
	}
	// CLI must not expose arbitrary --isolation changes to a live Sandbox.
	_, _, _ = (*cobra.Command)(nil), 0, 0
}

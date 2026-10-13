package cli

import (
	"path/filepath"
	"testing"
)

func TestSandboxToolSuiteCLIManagementWithoutProfileExposure(t *testing.T) {
	db := filepath.Join(t.TempDir(), "suite.sqlite")
	created := runAgentJSON(t, db, "sandbox", "add", "local", "--name", "CLI Tool Suite", "--root", t.TempDir(), "--isolation", "host")
	sandbox := created["sandbox"].(map[string]any)
	id := sandbox["sandboxId"].(string)
	definitions := runAgentJSON(t, db, "sandbox", "suites")
	suites := definitions["suites"].([]any)
	if len(suites) != 4 {
		t.Fatal("Tool suite catalogue unavailable from CLI")
	}
	jobs := runAgentJSON(t, db, "sandbox", "job-tools", id)["suite"].(map[string]any)
	if jobs["suiteId"] != "sandbox.jobs" || jobs["available"] != false || jobs["providerId"] != nil {
		t.Fatalf("CLI local Sandbox exposed Jobs %+v", jobs)
	}
	before := runAgentJSON(t, db, "sandbox", "tools", id)["suite"].(map[string]any)
	if before["status"] != "missing" {
		t.Fatalf("GET suite unexpectedly mutating: %+v", before)
	}
	prepared := runAgentJSON(t, db, "sandbox", "tools", id, "--prepare")["suite"].(map[string]any)
	if prepared["status"] != "ready" || len(prepared["tools"].([]any)) != 4 {
		t.Fatalf("CLI suite not ready: %+v", prepared)
	}
	repeated := runAgentJSON(t, db, "sandbox", "tools", id, "--prepare")["suite"].(map[string]any)
	if repeated["providerId"] != prepared["providerId"] {
		t.Fatal("CLI tool suite identity changed")
	}
	removed := runAgentJSON(t, db, "sandbox", "tools", id, "--remove", "--confirm")
	if removed["removed"] != true {
		t.Fatalf("CLI could not retire unused suite: %+v", removed)
	}
}

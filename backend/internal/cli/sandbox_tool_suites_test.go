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
	native := runAgentJSON(t, db, "sandbox", "tools", id)["suite"].(map[string]any)
	if native["status"] != "ready" || len(native["tools"].([]any)) != 4 || native["providerId"] != nil {
		t.Fatalf("CLI native tool projection invalid: %+v", native)
	}
	providers := runAgentJSON(t, db, "provider", "list")
	if len(providers["providers"].([]any)) != 0 {
		t.Fatal("Sandbox Tool projection created Provider")
	}
}

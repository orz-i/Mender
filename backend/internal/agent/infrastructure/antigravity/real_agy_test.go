package antigravity_test

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	agentapp "github.com/orz-i/Mender/backend/internal/agent/application"
	agentdomain "github.com/orz-i/Mender/backend/internal/agent/domain"
	agentacp "github.com/orz-i/Mender/backend/internal/agent/infrastructure/acp"
	agentagy "github.com/orz-i/Mender/backend/internal/agent/infrastructure/antigravity"
	isolation "github.com/orz-i/Mender/backend/internal/runtimegovernance/infrastructure/processisolation"
)

// Opt-in only: an actual authenticated CLI run may consume model usage.
// No production database, code, credentials, native tool or project path is touched.
func TestRealAntigravityNoToolMenderNativeLauncherAndRunner(t *testing.T) {
	if os.Getenv("MENDER_REAL_AGY_NO_TOOL") != "1" {
		t.Skip("set MENDER_REAL_AGY_NO_TOOL=1 for an opt-in authenticated smoke")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 80*time.Second)
	defer cancel()
	launcher, err := agentacp.NewLocalFactory(isolation.New())
	if err != nil {
		t.Fatal(err)
	}
	if _, err = launcher.Availability(agentdomain.BackendAntigravity); err != nil {
		t.Fatal(err)
	}
	root, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	id, err := agentdomain.NewID()
	if err != nil {
		t.Fatal(err)
	}
	a, err := agentdomain.NewAgent(agentdomain.Spec{ID: id, Name: "Ephemeral AGY smoke",
		Backend: agentdomain.BackendAntigravity, PluginID: agentdomain.BackendAntigravity.PluginID(), ExtensionID: "main",
		WorkspaceRoot: root, Isolation: "host", OwnerKind: "local_user", OwnerRef: "smoke", CreatedAt: time.Now()})
	if err != nil {
		t.Fatal(err)
	}
	runner := agentagy.Runner{NewCommand: launcher.AntigravityCommand}
	choices, err := runner.InspectModels(ctx, a)
	if err != nil || len(choices.Models) == 0 {
		t.Fatalf("CLI model inspection: count=%d err=%v", len(choices.Models), err)
	}
	result, err := runner.ExecuteConfigured(ctx, a, "No tools, file access or shell. Reply with exactly MENDER_ANTIGRAVITY_INTEGRATION_OK", agentapp.RunConfig{WorkspaceRoot: root})
	if err != nil || strings.TrimSpace(result) != "MENDER_ANTIGRAVITY_INTEGRATION_OK" {
		t.Fatalf("real native headless completion failed: response_matches=%v err=%v", strings.TrimSpace(result) == "MENDER_ANTIGRAVITY_INTEGRATION_OK", err)
	}
	t.Log("real Mender native launcher + model inspection + headless no-tool task passed")
}

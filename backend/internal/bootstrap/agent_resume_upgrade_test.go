package bootstrap

import (
	"context"
	"database/sql"
	"path/filepath"
	"testing"

	agentapp "github.com/orz-i/Mender/backend/internal/agent/application"
	agentdomain "github.com/orz-i/Mender/backend/internal/agent/domain"
	catalogdomain "github.com/orz-i/Mender/backend/internal/catalog/domain"
	profileapp "github.com/orz-i/Mender/backend/internal/profile/application"
	profiledomain "github.com/orz-i/Mender/backend/internal/profile/domain"
	_ "modernc.org/sqlite"
)

// A pre-v15 provisioned Agent facade has only four canonical Tools. Adding
// resume is an explicit resource-owner sync. It must preserve the four old
// ToolIDs and Profile membership; new resume never auto-binds to any Profile.
func TestAgentResumeUpgradesFourToolFacadeWithoutChangingProfileGrants(t *testing.T) {
	ctx := context.Background()
	path := filepath.Join(t.TempDir(), "old-agent-facade.db")
	runtime, err := NewRuntime(ctx, RuntimeOptions{DatabasePath: path, AgentExecutor: fixtureAgentExecutor{}})
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = runtime.Close() }()
	owner := agentOwnerContext(t, runtime)
	agent, err := runtime.CreateAgentWithTools(ctx, agentapp.CreateRequest{
		Name: "Old Agent facade", Backend: agentdomain.BackendGrok, WorkspaceRoot: t.TempDir(),
	}, owner)
	if err != nil {
		t.Fatal(err)
	}
	initial, err := runtime.AgentTools(ctx, agent.ID(), owner)
	if err != nil || initial.Status != "ready" || len(initial.Tools) != 5 {
		t.Fatalf("initial %+v %v", initial, err)
	}
	oldIDs := map[string]string{}
	resumeID := ""
	for _, tool := range initial.Tools {
		if tool.SourceKey == "agent.resume" {
			resumeID = tool.ToolID
		} else {
			oldIDs[tool.SourceKey] = tool.ToolID
		}
	}
	if len(oldIDs) != 4 || resumeID == "" {
		t.Fatal("missing original or resume Tool")
	}
	profile, err := runtime.Profiles.Create.Execute(ctx, profileapp.CreateProfileRequest{
		Slug: "four-tool-profile", DisplayName: "Existing Profile", Mode: profiledomain.ExposureDirect,
	})
	if err != nil {
		t.Fatal(err)
	}
	_, err = runtime.Profiles.AddTool.Execute(ctx, profileapp.AddToolRequest{
		ProfileSlug: profile.Slug(), ToolID: catalogdomain.ToolID(oldIDs["agent.delegate"]), Direct: true,
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := runtime.Agents.Grant(ctx, agent.ID(), profile.ID().String(), owner); err != nil {
		t.Fatal(err)
	}
	raw, err := sql.Open("sqlite", path)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = raw.Close() }()
	if _, err := raw.ExecContext(ctx, "DELETE FROM tools WHERE id=?", resumeID); err != nil {
		t.Fatal(err)
	}
	stale, err := runtime.AgentTools(ctx, agent.ID(), owner)
	if err != nil || stale.Status != "incomplete" || len(stale.Tools) != 4 {
		t.Fatalf("four-tool fixture %+v %v", stale, err)
	}
	restored, err := runtime.EnsureAgentTools(ctx, agent.ID(), owner)
	if err != nil || restored.Status != "ready" || len(restored.Tools) != 5 {
		t.Fatalf("repair %+v %v", restored, err)
	}
	for _, tool := range restored.Tools {
		if tool.SourceKey == "agent.resume" {
			continue
		}
		if oldIDs[tool.SourceKey] != tool.ToolID {
			t.Fatalf("old Tool identity rotated %s", tool.SourceKey)
		}
	}
	var bindingCount int
	if err := raw.QueryRowContext(ctx,
		"SELECT COUNT(*) FROM profile_tools WHERE tool_id IN (?,?)", oldIDs["agent.delegate"], resumeID).Scan(&bindingCount); err != nil || bindingCount != 1 {
		t.Fatalf("Profile implicitly inherited fifth Tool n=%d err=%v", bindingCount, err)
	}
	if err := raw.QueryRowContext(ctx, "SELECT COUNT(*) FROM profile_tools WHERE tool_id=?",
		resumeID).Scan(&bindingCount); err != nil || bindingCount != 0 {
		t.Fatalf("new resume Tool was automatically granted n=%d %v", bindingCount, err)
	}
}

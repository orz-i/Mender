package bootstrap

import (
	"bytes"
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

func TestRefreshReadyAgentToolsUpdatesSchemaWithoutIdentityChange(t *testing.T) {
	ctx := context.Background()
	path := filepath.Join(t.TempDir(), "ready-facade.db")
	r, err := NewRuntime(ctx, RuntimeOptions{DatabasePath: path, AgentExecutor: fixtureAgentExecutor{}})
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = r.Close() }()
	owner := agentOwnerContext(t, r)
	agent, err := r.CreateAgentWithTools(ctx, agentapp.CreateRequest{
		Name: "Existing Grok", Backend: agentdomain.BackendGrok, WorkspaceRoot: t.TempDir(),
	}, owner)
	if err != nil {
		t.Fatal(err)
	}
	before, err := r.AgentTools(ctx, agent.ID(), owner)
	if err != nil || before.Status != "ready" {
		t.Fatalf("not ready %+v %v", before, err)
	}
	var delegateID string
	for _, tool := range before.Tools {
		if tool.SourceKey == "agent.delegate" {
			delegateID = tool.ToolID
		}
	}
	if delegateID == "" {
		t.Fatal("delegate tool missing")
	}
	profile, err := r.Profiles.Create.Execute(ctx, profileapp.CreateProfileRequest{Slug: "existing-agent", DisplayName: "Existing Profile", Mode: profiledomain.ExposureDirect})
	if err != nil {
		t.Fatal(err)
	}
	_, err = r.Profiles.AddTool.Execute(ctx, profileapp.AddToolRequest{ProfileSlug: profile.Slug(), ToolID: catalogdomain.ToolID(delegateID), Direct: true})
	if err != nil {
		t.Fatal(err)
	}
	if err := r.Agents.Grant(ctx, agent.ID(), profile.ID().String(), owner); err != nil {
		t.Fatal(err)
	}
	raw, err := sql.Open("sqlite", path)
	if err != nil {
		t.Fatal(err)
	}
	// Simulate the former persisted model-only Tool schema on a Ready facade.
	legacy := []byte(`{"type":"object","required":["prompt"],"properties":{"prompt":{"type":"string"},"model":{"type":"string"}}}`)
	if _, err := raw.ExecContext(ctx, "UPDATE tools SET input_schema=? WHERE id=?", legacy, delegateID); err != nil {
		t.Fatal(err)
	}
	if err := raw.Close(); err != nil {
		t.Fatal(err)
	}
	fresh, err := r.EnsureAgentTools(ctx, agent.ID(), owner)
	if err != nil {
		t.Fatal(err)
	}
	if fresh.ProviderID != before.ProviderID || len(fresh.Tools) != len(before.Tools) {
		t.Fatalf("refresh cloned Provider/Tools %+v", fresh)
	}
	for i, item := range fresh.Tools {
		if item.ToolID != before.Tools[i].ToolID {
			t.Fatal("refresh rotated canonical Tool ID")
		}
	}
	pid, err := catalogdomain.ParseProviderID(fresh.ProviderID)
	if err != nil {
		t.Fatal(err)
	}
	tools, err := r.Catalog.store.ListProviderTools(ctx, pid)
	if err != nil {
		t.Fatal(err)
	}
	var updated bool
	for _, tool := range tools {
		if tool.SourceKey() == "agent.delegate" {
			updated = bytes.Contains(tool.InputSchema().Bytes(), []byte("reasoningEffort"))
		}
	}
	if !updated {
		t.Fatal("Ready facade retained stale delegation input schema")
	}
	grants, err := r.Agents.Grants(ctx, agent.ID(), owner)
	if err != nil || len(grants) != 1 || grants[0] != profile.ID().String() {
		t.Fatalf("refresh changed Profile grants %+v %v", grants, err)
	}
}

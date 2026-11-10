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
	_ "modernc.org/sqlite"
)

func TestRefreshAgentOwnedToolsPreservesIdentityAndProfileGrants(t *testing.T) {
	ctx := context.Background()
	path := filepath.Join(t.TempDir(), "v3-agent.db")
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
	profile, err := createMCPProfileFixture(t, r, ctx, profileapp.CreateProfileRequest{Slug: "existing-agent", DisplayName: "Existing Profile"}, "direct")
	if err != nil {
		t.Fatal(err)
	}
	_, err = addMCPProfileToolFixture(t, r, ctx, profileapp.AddToolRequest{ProfileSlug: profile.Slug(), ToolID: catalogdomain.ToolID(delegateID)}, true)
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
	// Simulate a stale schema in a canonical Agent-owned Tool row.
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
	if len(fresh.Tools) != len(before.Tools) {
		t.Fatalf("refresh cloned Provider/Tools %+v", fresh)
	}
	for i, item := range fresh.Tools {
		if item.ToolID != before.Tools[i].ToolID {
			t.Fatal("refresh rotated canonical Tool ID")
		}
	}
	source, err := catalogdomain.AgentSource(string(agent.ID()))
	if err != nil {
		t.Fatal(err)
	}
	providerless, ok := r.Catalog.store.(interface {
		ListSourceTools(context.Context, catalogdomain.SourceRef) ([]catalogdomain.Tool, error)
	})
	if !ok {
		t.Fatal("typed Catalog source lookup unavailable")
	}
	tools, err := providerless.ListSourceTools(ctx, source)
	if err != nil {
		t.Fatal(err)
	}
	var updated bool
	for _, tool := range tools {
		if tool.SourceKey() == "agent.delegate" {
			updated = bytes.Contains(tool.InputSchema().Bytes(), []byte("reasoningEffort")) &&
				bytes.Contains(tool.InputSchema().Bytes(), []byte("timeoutMinutes"))
		}
	}
	if !updated {
		t.Fatal("Agent source retained stale delegation input schema")
	}
	grants, err := r.Agents.Grants(ctx, agent.ID(), owner)
	if err != nil || len(grants) != 1 || grants[0] != profile.ID().String() {
		t.Fatalf("refresh changed Profile grants %+v %v", grants, err)
	}
}

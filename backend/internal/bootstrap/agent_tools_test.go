package bootstrap

import (
	"context"
	"errors"
	"path/filepath"
	"testing"

	agentapp "github.com/orz-i/Mender/backend/internal/agent/application"
	agentdomain "github.com/orz-i/Mender/backend/internal/agent/domain"
)

// Every Agent as Tool is a directly owned Catalog source. No resource Provider
// is synthesized, even when two Agents use the same Backend and tool names.
func TestAgentOwnedToolsAreDistinctIdempotentAndRetireAtomically(t *testing.T) {
	ctx := context.Background()
	r, err := NewRuntime(ctx, RuntimeOptions{DatabasePath: filepath.Join(t.TempDir(), "v3.db"), AgentExecutor: fixtureAgentExecutor{}})
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = r.Close() }()
	owner := agentOwnerContext(t, r)
	identities := map[string]bool{}
	for _, name := range []string{"Grok Frontend", "Grok Review"} {
		agent, err := r.CreateAgentWithTools(ctx, agentapp.CreateRequest{Name: name, Backend: agentdomain.BackendGrok, WorkspaceRoot: t.TempDir()}, owner)
		if err != nil {
			t.Fatal(err)
		}
		state, err := r.AgentTools(ctx, agent.ID(), owner)
		if err != nil || state.Status != "ready" || len(state.Tools) != 5 {
			t.Fatalf("Agent-owned source: %+v %v", state, err)
		}
		for _, tool := range state.Tools {
			if identities[tool.ToolID] {
				t.Fatalf("Tool ID shared between Agents: %s", tool.ToolID)
			}
			identities[tool.ToolID] = true
		}
		again, err := r.EnsureAgentTools(ctx, agent.ID(), owner)
		if err != nil || len(again.Tools) != 5 {
			t.Fatalf("idempotent source refresh: %+v %v", again, err)
		}
		for i, tool := range state.Tools {
			if tool.ToolID != again.Tools[i].ToolID {
				t.Fatal("ToolID rotated on refresh")
			}
		}
		grants, err := r.Agents.Grants(ctx, agent.ID(), owner)
		if err != nil || len(grants) != 0 {
			t.Fatalf("source creation granted authority: %v %v", grants, err)
		}
		if err = r.DeleteAgentWithTools(ctx, agent.ID(), owner); err != nil {
			t.Fatalf("unreferenced Agent not retired: %v", err)
		}
		if _, err = r.AgentTools(ctx, agent.ID(), owner); !errors.Is(err, agentdomain.ErrNotFound) {
			t.Fatalf("retired Agent remained visible: %v", err)
		}
	}
}
